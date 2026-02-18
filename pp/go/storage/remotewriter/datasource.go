package remotewriter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
)

//
// CorruptMarker
//

// CorruptMarker mark head as corrupted by ID.
type CorruptMarker interface {
	// MarkCorrupted mark head as corrupted by ID.
	MarkCorrupted(headID string) error
}

//
// SegmentReadyChecker
//

// SegmentReadyChecker is a segment ready checker.
type SegmentReadyChecker interface {
	// SegmentIsReady checks if the segment is ready.
	SegmentIsReady(segmentID uint32) (shards []uint16, ready, outOfRange bool)
}

//
// segmentReadyChecker
//

type segmentReadyChecker struct {
	headRecord *catalog.Record
	shards     []uint16
}

func newSegmentReadyChecker(headRecord *catalog.Record) *segmentReadyChecker {
	return &segmentReadyChecker{
		headRecord: headRecord,
		shards:     make([]uint16, 0, headRecord.NumberOfShards()),
	}
}

func (src *segmentReadyChecker) SegmentIsReady(segmentID uint32) (shards []uint16, ready, outOfRange bool) {
	sourceShard := src.headRecord.GetShardBySegmentID(segmentID)

	readyV1 := src.headRecord.LastAppendedSegmentID() != nil && *src.headRecord.LastAppendedSegmentID() >= segmentID
	readyV2 := sourceShard != math.MaxUint16
	ready = readyV1 || readyV2

	outOfRange = (src.headRecord.Status() != catalog.StatusNew &&
		src.headRecord.Status() != catalog.StatusActive) &&
		!ready

	if !ready {
		return nil, ready, outOfRange
	}

	if readyV1 {
		// on v1 fill once and reuse
		if len(src.shards) == 0 {
			for i := uint16(0); i < src.headRecord.NumberOfShards(); i++ {
				src.shards = append(src.shards, i)
			}
		}

		return src.shards, ready, outOfRange
	}

	src.shards = src.shards[:0]
	src.shards = append(src.shards, sourceShard)

	fmt.Println(
		" ===== SegmentIsReady",
		"segmentID:", segmentID,
		"sourceShard:", sourceShard,
		"ready:", ready,
		"outOfRange:", outOfRange,
		"readyV1:", readyV1,
		"readyV2:", readyV2,
		"src.shards:", src.shards,
	)

	return src.shards, ready, outOfRange
}

//
// shardCache
//

type shardCache struct {
	shardID uint16
	cache   *bytes.Buffer
	written bool
	writer  io.Writer
}

//
// dataSource
//

type dataSource struct {
	ID                  string
	shards              []*shard
	segmentReadyChecker SegmentReadyChecker
	corruptMarker       CorruptMarker
	closed              bool
	completed           bool
	corrupted           bool
	headReleaseFunc     func()

	lssSlice []*cppbridge.LabelSetStorage

	cacheMtx             sync.Mutex
	caches               []*shardCache
	cacheWriteSignal     chan struct{}
	cacheWriteLoopClosed chan struct{}

	unexpectedEOFCount prometheus.Counter
	segmentSize        prometheus.Histogram
}

// newDataSource creates a new [dataSource].
func newDataSource(dataDir string,
	numberOfShards uint16,
	config DestinationConfig, //nolint:gocritic // hugeParam // config
	discardCache bool,
	segmentReadyChecker SegmentReadyChecker,
	corruptMarker CorruptMarker,
	headRecord *catalog.Record,
	unexpectedEOFCount prometheus.Counter,
	segmentSize prometheus.Histogram,
) (*dataSource, error) {
	var err error
	var convertedRelabelConfigs []*cppbridge.RelabelConfig
	convertedRelabelConfigs, err = convertRelabelConfigs(config.WriteRelabelConfigs...)
	if err != nil {
		return nil, fmt.Errorf("failed to convert relabel configs: %w", err)
	}

	b := &dataSource{
		corruptMarker:        corruptMarker,
		segmentReadyChecker:  segmentReadyChecker,
		headReleaseFunc:      headRecord.Acquire(),
		unexpectedEOFCount:   unexpectedEOFCount,
		segmentSize:          segmentSize,
		cacheWriteSignal:     make(chan struct{}),
		cacheWriteLoopClosed: make(chan struct{}),
	}

	go b.cacheWriteLoop()

	for shardID := range numberOfShards {
		shardFileName := filepath.Join(dataDir, fmt.Sprintf("shard_%d.wal", shardID))
		decoderStateFileName := filepath.Join(dataDir, fmt.Sprintf("%s_shard_%d.state", config.Name, shardID))
		var s *shard
		s, err = createShard(
			headRecord.ID(),
			shardID,
			shardFileName,
			decoderStateFileName,
			discardCache,
			config.ExternalLabels,
			convertedRelabelConfigs,
			b.unexpectedEOFCount,
			b.segmentSize,
		)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("failed to create shard: %w", err), b.Close())
		}
		b.shards = append(b.shards, s)
		b.lssSlice = append(b.lssSlice, s.decoder.lss)
		b.caches = append(b.caches, &shardCache{
			shardID: shardID,
			cache:   bytes.NewBuffer(nil),
			written: true,
			writer:  s.decoderStateFile,
		})
	}

	return b, nil
}

func createShard(
	headID string,
	shardID uint16,
	shardFileName, decoderStateFileName string,
	resetDecoderState bool,
	externalLabels labels.Labels,
	relabelConfigs []*cppbridge.RelabelConfig,
	unexpectedEOFCount prometheus.Counter,
	segmentSize prometheus.Histogram,
) (*shard, error) {
	s, err := newShard(
		headID,
		shardID,
		shardFileName,
		decoderStateFileName,
		resetDecoderState,
		externalLabels,
		relabelConfigs,
		unexpectedEOFCount,
		segmentSize,
	)
	if err != nil {
		logger.Errorf("failed to create shard: %v", err)
		return newShard(
			headID,
			shardID,
			shardFileName,
			decoderStateFileName,
			true,
			externalLabels,
			relabelConfigs,
			unexpectedEOFCount,
			segmentSize,
		)
	}
	return s, nil
}

func convertRelabelConfigs(relabelConfigs ...*relabel.Config) ([]*cppbridge.RelabelConfig, error) {
	convertedConfigs := make([]*cppbridge.RelabelConfig, 0, len(relabelConfigs))
	for _, relabelConfig := range relabelConfigs {
		var sourceLabels []string
		for _, label := range relabelConfig.SourceLabels {
			sourceLabels = append(sourceLabels, string(label))
		}

		convertedConfig := &cppbridge.RelabelConfig{
			SourceLabels: sourceLabels,
			Separator:    relabelConfig.Separator,
			Regex:        relabelConfig.Regex.String(),
			Modulus:      relabelConfig.Modulus,
			TargetLabel:  relabelConfig.TargetLabel,
			Replacement:  relabelConfig.Replacement,
			Action:       cppbridge.ActionNameToValueMap[string(relabelConfig.Action)],
		}

		if err := convertedConfig.Validate(); err != nil {
			return nil, fmt.Errorf("failed to validate config: %w", err)
		}

		convertedConfigs = append(convertedConfigs, convertedConfig)
	}

	return convertedConfigs, nil
}

func (ds *dataSource) Close() error {
	if ds.closed {
		return nil
	}
	ds.closed = true
	var err error
	// stop cache writing first
	close(ds.cacheWriteSignal)
	<-ds.cacheWriteLoopClosed

	for _, s := range ds.shards {
		err = errors.Join(err, s.Close())
	}
	ds.headReleaseFunc()
	return err
}

func (ds *dataSource) IsCompleted() bool {
	return ds.completed
}

type readShardResult struct {
	segment *DecodedSegment
	err     error
}

func (ds *dataSource) Read(
	ctx context.Context,
	segmentID uint32,
	minTimestamp int64,
	segmentSamplesStorages *cppbridge.SegmentSamplesStorageList,
) ([]*DecodedSegment, error) {
	if ds.completed {
		return nil, ErrEndOfBlock
	}

	shards, segmentIsReady, segmentIsOutOfRange := ds.segmentReadyChecker.SegmentIsReady(segmentID)
	if !segmentIsReady {
		if segmentIsOutOfRange {
			return nil, ErrEndOfBlock
		}

		return nil, ErrEmptyReadResult
	}

	wg := sync.WaitGroup{}
	readShardResults := make([]readShardResult, len(shards))
	for i, shardID := range shards {
		if ds.shards[shardID].corrupted {
			readShardResults[i] = readShardResult{
				segment: nil,
				err:     NewShardError(shardID, false, ErrShardIsCorrupted),
			}
			continue
		}
		wg.Add(1)
		go func(id int, shardID uint16) {
			defer wg.Done()
			segment, err := ds.shards[shardID].Read(
				ctx,
				segmentID,
				minTimestamp,
				segmentSamplesStorages.Get(uint64(shardID)), // #nosec G115 // no overflow
			)
			if err != nil {
				err = NewShardError(shardID, true, err)
			}
			readShardResults[id] = readShardResult{segment: segment, err: err}
		}(i, shardID)
	}
	wg.Wait()

	segments := make([]*DecodedSegment, 0, len(ds.shards))
	errs := make([]error, 0, len(ds.shards))
	for _, result := range readShardResults {
		if result.segment != nil {
			segments = append(segments, result.segment)
		}
		if result.err != nil {
			errs = append(errs, result.err)
		}
	}

	return segments, ds.handleReadErrors(errs)
}

func (ds *dataSource) handleReadErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}

	if len(errs) == len(ds.shards) {
		ds.corrupted = true
		if ds.corruptMarker != nil {
			if err := ds.corruptMarker.MarkCorrupted(ds.ID); err != nil {
				return fmt.Errorf("failed to mark head corrupted: %w", err)
			}
			ds.corruptMarker = nil
		}

		return ErrEndOfBlock
	}

	ds.corrupted = true
	if ds.corruptMarker != nil {
		if err := ds.corruptMarker.MarkCorrupted(ds.ID); err != nil {
			return fmt.Errorf("failed to mark head corrupted: %w", err)
		}
		ds.corruptMarker = nil
	}

	for _, err := range errs {
		var shardErr ShardError
		if errors.As(err, &shardErr) {
			if shardErr.processable {
				logger.Errorf("shard %s/%d is corrupted", ds.ID, shardErr.ShardID())
			}
		}
	}

	return nil
}

func (ds *dataSource) LSSes() []*cppbridge.LabelSetStorage {
	return ds.lssSlice
}

func (ds *dataSource) NumberOfLSSes() int {
	return len(ds.lssSlice)
}

// WriteCaches writes caches to the buffer and sends the signal to write the caches.
func (ds *dataSource) WriteCaches() {
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
func (ds *dataSource) cacheWriteLoop() {
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

// writeBufferedCaches writes cached data to the disk and resets the cache.
func (ds *dataSource) writeBufferedCaches() {
	ds.cacheMtx.Lock()
	caches := make([]*shardCache, 0, len(ds.caches))
	for _, sc := range ds.caches {
		if sc.written {
			continue
		}
		sc := sc
		caches = append(caches, sc)
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
