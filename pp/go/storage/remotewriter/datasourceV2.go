package remotewriter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
)

//
// DataSourceV2
//

// DataSourceV2 a data source of the head shards for sending data through the RemoteWriter..
type DataSourceV2 interface {
	// Close write caches and closes the data source and releases the resources.
	Close() error

	Init(skipUntil uint32)
	Next(
		ctx context.Context,
		minTimestamp int64,
		segmentSamplesStorages *cppbridge.SegmentSamplesStorageList,
	) ([]*DecodedSegment, error)

	// LSSes returns the label set storages of the shards.
	LSSes() []*cppbridge.LabelSetStorage

	// NumberOfLSSes returns the number of label set storages.
	NumberOfLSSes() int

	// WriteCaches writes caches to the buffer and sends the signal to write the caches.
	WriteCaches()
}

//
// dataSourceActive
//

// dataSourceActive a data source of the active head shards for sending data through the RemoteWriter.
type dataSourceActive struct {
	headID              string
	clock               clockwork.Clock
	segmentReadyChecker SegmentReadyChecker
	corruptMarker       CorruptMarker
	headReleaseFunc     func()
	shards              []*shard
	lsses               []*cppbridge.LabelSetStorage
	nextSegmentID       uint32
	closed              bool

	cacheMtx             sync.Mutex
	caches               []*shardCache
	cacheWriteSignal     chan struct{}
	cacheWriteLoopClosed chan struct{}

	unexpectedEOFCount prometheus.Counter
	segmentSize        prometheus.Histogram
}

func newDataSourceActive(
	dataDir string,
	config DestinationConfig, //nolint:gocritic // hugeParam // config
	numberOfShards uint16,
	discardCache bool,
	clock clockwork.Clock,
	segmentReadyChecker SegmentReadyChecker,
	corruptMarker CorruptMarker,
	headRecord *catalog.Record,
	unexpectedEOFCount prometheus.Counter,
	segmentSize prometheus.Histogram,
) (*dataSourceActive, error) {
	convertedRelabelConfigs, err := convertRelabelConfigs(config.WriteRelabelConfigs...)
	if err != nil {
		return nil, fmt.Errorf("failed to convert relabel configs: %w", err)
	}

	ds := &dataSourceActive{
		headID:               headRecord.ID(),
		clock:                clock,
		segmentReadyChecker:  segmentReadyChecker,
		corruptMarker:        corruptMarker,
		headReleaseFunc:      headRecord.Acquire(),
		shards:               make([]*shard, 0, numberOfShards),
		lsses:                make([]*cppbridge.LabelSetStorage, 0, numberOfShards),
		unexpectedEOFCount:   unexpectedEOFCount,
		segmentSize:          segmentSize,
		cacheWriteSignal:     make(chan struct{}),
		cacheWriteLoopClosed: make(chan struct{}),
	}

	go ds.cacheWriteLoop()

	for shardID := range numberOfShards {
		var s *shard
		s, err = createShard(
			ds.headID,
			shardID,
			filepath.Join(dataDir, fmt.Sprintf("shard_%d.wal", shardID)),
			filepath.Join(dataDir, fmt.Sprintf("%s_shard_%d.state", config.Name, shardID)),
			discardCache,
			config.ExternalLabels,
			convertedRelabelConfigs,
			ds.unexpectedEOFCount,
			ds.segmentSize,
		)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("failed to create shard: %w", err), ds.Close())
		}

		ds.shards = append(ds.shards, s)
		ds.lsses = append(ds.lsses, s.decoder.lss)
		ds.caches = append(ds.caches, &shardCache{
			shardID: shardID,
			cache:   bytes.NewBuffer(nil),
			written: true,
			writer:  s.decoderStateFile,
		})
	}

	return ds, nil
}

// Close write caches and closes the data source and releases the resources.
func (ds *dataSourceActive) Close() error {
	if ds.closed {
		return nil
	}
	ds.closed = true

	// stop cache writing first
	close(ds.cacheWriteSignal)
	<-ds.cacheWriteLoopClosed

	var err error
	for _, s := range ds.shards {
		err = errors.Join(err, s.Close())
	}
	ds.headReleaseFunc()

	return err
}

// LSSes returns the label set storages of the shards.
func (ds *dataSourceActive) LSSes() []*cppbridge.LabelSetStorage {
	return ds.lsses
}

// NumberOfLSSes returns the number of label set storages.
func (ds *dataSourceActive) NumberOfLSSes() int {
	return len(ds.lsses)
}

// Init it initializes the data source by reading segments from shards until the required number is reached.
func (ds *dataSourceActive) Init(ctx context.Context, targetSegmentID uint32) error {
	if targetSegmentID == 0 {
		return nil
	}

	mow := ds.clock.Now().UnixMilli()
	segmentSampleStorages := cppbridge.NewSegmentSamplesStorage(
		uint64(len(ds.shards)), // #nosec G115 // no overflow
	)
	var delay time.Duration
	for ; ds.nextSegmentID < targetSegmentID; ds.nextSegmentID++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ds.clock.After(delay):
		}

		if _, err := ds.Next(ctx, mow, segmentSampleStorages); err != nil {
			if errors.Is(err, ErrEndOfBlock) {
				return nil
			}

			if errors.Is(err, ErrEmptyReadResult) {
				delay = defaultDelay
				continue
			}

			return err
		}
	}

	return nil
}

// Next checks the segmentID for readiness and reads the [DecodedSegment] from the shards.
func (ds *dataSourceActive) Next(
	ctx context.Context,
	minTimestamp int64,
	segmentSamplesStorages *cppbridge.SegmentSamplesStorageList,
) ([]*DecodedSegment, error) {
	// shardIDs are needed for V2 to read only recorded segments,
	// otherwise there will be an attempt to read the sync data
	shardIDs, segmentIsReady, segmentIsOutOfRange := ds.segmentReadyChecker.SegmentIsReady(ds.nextSegmentID)
	if !segmentIsReady {
		if segmentIsOutOfRange {
			return nil, ErrEndOfBlock
		}

		return nil, ErrEmptyReadResult
	}

	readShardResults := ds.readFromShards(ctx, shardIDs, minTimestamp, segmentSamplesStorages, ds.nextSegmentID)
	segments := make([]*DecodedSegment, 0, len(shardIDs))
	errs := make([]error, 0, len(shardIDs))
	for _, result := range readShardResults {
		if result.segment != nil {
			segments = append(segments, result.segment)
		}
		if result.err != nil && !errors.Is(result.err, context.Canceled) {
			errs = append(errs, result.err)
		}
	}

	return segments, ds.handleReadErrors(errs)
}

// WriteCaches writes caches to the buffer and sends the signal to write the caches.
func (ds *dataSourceActive) WriteCaches() {
	ds.cacheMtx.Lock()
	for shardID, sc := range ds.caches {
		if !sc.written {
			continue
		}
		sc.cache.Reset()
		if _, err := ds.shards[shardID].decoder.WriteTo(sc.cache); err != nil {
			logger.Errorf("failed to get output decoder cache: %v", err)
			continue
		}
		sc.written = false
	}
	ds.cacheMtx.Unlock()

	select {
	case ds.cacheWriteSignal <- struct{}{}:
	default:
	}
}

// cacheWriteLoop loop that writes caches to the buffer and sends the signal to write the caches.
func (ds *dataSourceActive) cacheWriteLoop() {
	defer close(ds.cacheWriteLoopClosed)
	var closed bool
	var writeRequested bool
	var writeResultc chan struct{}

	for {
		if writeRequested && !closed && writeResultc == nil {
			writeResultc = make(chan struct{})
			go func() {
				defer close(writeResultc)
				ds.writeBufferedCaches()
			}()
			writeRequested = false
		}

		if closed && writeResultc == nil {
			return
		}

		select {
		case _, ok := <-ds.cacheWriteSignal:
			if !ok {
				return
			}
			writeRequested = true
		case <-writeResultc:
			writeResultc = nil
		}
	}
}

// checkFullCorrupted checks if all the shards are corrupted, if all the shards are corrupted,
// there is no point in continuing to read the shards, we return an error.
func (ds *dataSourceActive) checkFullCorrupted() error {
	corruptedShards := 0
	for _, s := range ds.shards {
		if s.corrupted {
			corruptedShards++
		}
	}

	if corruptedShards == len(ds.shards) {
		return ErrEndOfBlock
	}

	return nil
}

// handleReadErrors handles the errors from the shards and returns an [ErrEndOfBlock] if the data source is corrupted.
func (ds *dataSourceActive) handleReadErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}

	if len(errs) == len(ds.shards) {
		if ds.corruptMarker != nil {
			if err := ds.corruptMarker.MarkCorrupted(ds.headID); err != nil {
				return fmt.Errorf("failed to mark head corrupted: %w", err)
			}
			ds.corruptMarker = nil
		}

		return ErrEndOfBlock
	}

	if ds.corruptMarker != nil {
		if err := ds.corruptMarker.MarkCorrupted(ds.headID); err != nil {
			return fmt.Errorf("failed to mark head corrupted: %w", err)
		}
		ds.corruptMarker = nil
	}

	ds.printErrorIfNeed(errs)

	return ds.checkFullCorrupted()
}

// printErrorIfNeed logs errors if necessary.
func (ds *dataSourceActive) printErrorIfNeed(errs []error) {
	for _, err := range errs {
		if errors.Is(err, context.Canceled) {
			continue
		}

		var shardErr ShardError
		if errors.As(err, &shardErr) && shardErr.processable {
			logger.Errorf("shard %s/%d is corrupted: %s", ds.headID, shardErr.ShardID(), shardErr.Error())
		}
	}
}

// readFromShards parallel reading of [DecodedSegment] from shards.
func (ds *dataSourceActive) readFromShards(
	ctx context.Context,
	shardIDs []uint16,
	minTimestamp int64,
	segmentSamplesStorages *cppbridge.SegmentSamplesStorageList,
	targetSegmentID uint32,
) []readShardResult {
	readShardResults := make([]readShardResult, len(shardIDs))
	if len(shardIDs) == 1 {
		if ds.shards[shardIDs[0]].corrupted {
			readShardResults[0] = readShardResult{
				segment: nil,
				err:     NewShardError(shardIDs[0], false, ErrShardIsCorrupted),
			}
			return readShardResults
		}

		segment, err := ds.shards[shardIDs[0]].Read(
			ctx,
			targetSegmentID,
			minTimestamp,
			segmentSamplesStorages.Get(uint64(shardIDs[0])),
		)
		if err != nil {
			err = NewShardError(shardIDs[0], true, err)
		}
		readShardResults[0] = readShardResult{segment: segment, err: err}

		return readShardResults
	}

	wg := sync.WaitGroup{}
	for i, shardID := range shardIDs {
		if ds.shards[shardID].corrupted {
			readShardResults[i] = readShardResult{segment: nil, err: NewShardError(shardID, false, ErrShardIsCorrupted)}
			continue
		}
		wg.Add(1)
		go func(id int, shardID uint16) {
			defer wg.Done()
			segment, err := ds.shards[shardID].Read(
				ctx,
				targetSegmentID,
				minTimestamp,
				segmentSamplesStorages.Get(uint64(shardID)),
			)
			if err != nil {
				err = NewShardError(shardID, true, err)
			}
			readShardResults[id] = readShardResult{segment: segment, err: err}
		}(i, shardID)
	}
	wg.Wait()

	return readShardResults
}

// writeBufferedCaches writes cached data to the disk and resets the cache.
func (ds *dataSourceActive) writeBufferedCaches() {
	ds.cacheMtx.Lock()
	caches := make([]*shardCache, 0, len(ds.caches))
	for i := range ds.caches {
		if ds.caches[i].written {
			continue
		}

		caches = append(caches, ds.caches[i])
	}
	ds.cacheMtx.Unlock()

	writtenCacheShardIDs := make([]uint16, 0, len(caches))
	for _, sc := range caches {
		if _, err := sc.cache.WriteTo(sc.writer); err != nil {
			logger.Errorf("failed to write cache: %v", err)
			continue
		}

		writtenCacheShardIDs = append(writtenCacheShardIDs, sc.shardID)
	}

	if len(writtenCacheShardIDs) > 0 {
		ds.cacheMtx.Lock()
		for _, shardID := range writtenCacheShardIDs {
			ds.caches[shardID].written = true
		}
		ds.cacheMtx.Unlock()
	}
}
