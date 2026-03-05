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
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
)

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

	caches *caches[*shard]
}

// newDataSourceActive creates a new [dataSourceActive].
func newDataSourceActive(
	dataDir string,
	config DestinationConfig, //nolint:gocritic // hugeParam // config
	numberOfShards uint16,
	discardCache bool,
	clock clockwork.Clock,
	segmentReadyChecker SegmentReadyChecker,
	corruptMarker CorruptMarker,
	headRecord *catalog.Record,
	segmentSize prometheus.Histogram,
) (*dataSourceActive, error) {
	convertedRelabelConfigs, err := convertRelabelConfigs(config.WriteRelabelConfigs...)
	if err != nil {
		return nil, fmt.Errorf("failed to convert relabel configs: %w", err)
	}

	ds := &dataSourceActive{
		headID:              headRecord.ID(),
		clock:               clock,
		segmentReadyChecker: segmentReadyChecker,
		corruptMarker:       corruptMarker,
		headReleaseFunc:     headRecord.Acquire(),
		shards:              make([]*shard, 0, numberOfShards),
		lsses:               make([]*cppbridge.LabelSetStorage, 0, numberOfShards),
		caches:              newCaches[*shard](),
	}

	go ds.caches.cacheWriteLoop()

	for shardID := range numberOfShards {
		s, err := createShard(
			ds.headID,
			shardID,
			filepath.Join(dataDir, fmt.Sprintf("shard_%d.wal", shardID)),
			filepath.Join(dataDir, fmt.Sprintf("%s_shard_%d.state", config.Name, shardID)),
			discardCache,
			config.ExternalLabels,
			convertedRelabelConfigs,
			segmentSize,
		)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("failed to create shard: %w", err), ds.Close())
		}

		ds.caches.append(s.decoderStateFile, shardID)
		ds.shards = append(ds.shards, s)
		ds.lsses = append(ds.lsses, s.decoder.lss)
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
	ds.caches.close()

	var err error
	for _, s := range ds.shards {
		err = errors.Join(err, s.Close())
	}
	ds.headReleaseFunc()

	return err
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
	for ds.nextSegmentID < targetSegmentID {
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

// LSSes returns the label set storages of the shards.
func (ds *dataSourceActive) LSSes() []*cppbridge.LabelSetStorage {
	return ds.lsses
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
			ds.nextSegmentID = max(ds.nextSegmentID, result.segment.ID+1)
		}
		if result.err != nil && !errors.Is(result.err, context.Canceled) {
			errs = append(errs, result.err)
		}
	}

	return segments, handleReadErrors(errs, ds.markCorrupted, ds.checkFullCorrupted, len(ds.shards))
}

// NumberOfLSSes returns the number of label set storages.
func (ds *dataSourceActive) NumberOfLSSes() int {
	return len(ds.lsses)
}

// WriteCaches writes caches to the buffer and sends the signal to write the caches.
func (ds *dataSourceActive) WriteCaches() {
	ds.caches.writeCaches(ds.shards)
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

// markCorrupted marks the head as corrupted.
func (ds *dataSourceActive) markCorrupted() {
	if ds.corruptMarker == nil {
		return
	}

	if err := ds.corruptMarker.MarkCorrupted(ds.headID); err != nil {
		logger.Errorf(
			"datasource: %s failed to mark head corrupted: %v",
			ds.headID,
			err,
		)

		return
	}

	ds.corruptMarker = nil
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
				err:     NewShardError(ds.headID, shardIDs[0], false, ErrShardIsCorrupted),
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
			err = NewShardError(ds.headID, shardIDs[0], true, err)
		}
		readShardResults[0] = readShardResult{segment: segment, err: err}

		return readShardResults
	}

	wg := sync.WaitGroup{}
	for i, shardID := range shardIDs {
		if ds.shards[shardID].corrupted {
			readShardResults[i] = readShardResult{
				segment: nil,
				err:     NewShardError(ds.headID, shardID, false, ErrShardIsCorrupted),
			}
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
				err = NewShardError(ds.headID, shardID, true, err)
			}
			readShardResults[id] = readShardResult{segment: segment, err: err}
		}(i, shardID)
	}
	wg.Wait()

	return readShardResults
}

//
// dataSourceRotated
//

// dataSourceRotated a data source of the rotated head shards for sending data through the RemoteWriter.
type dataSourceRotated struct {
	headID          string
	clock           clockwork.Clock
	corruptMarker   CorruptMarker
	headReleaseFunc func()
	shards          []*shardRotated
	lsses           []*cppbridge.LabelSetStorage
	closed          bool

	caches *caches[*shardRotated]
}

// newDataSourceRotated creates a new [dataSourceRotated].
func newDataSourceRotated(
	dataDir string,
	config DestinationConfig, //nolint:gocritic // hugeParam // config
	numberOfShards uint16,
	discardCache bool,
	clock clockwork.Clock,
	corruptMarker CorruptMarker,
	headRecord *catalog.Record,
	segmentSize prometheus.Histogram,
) (*dataSourceRotated, error) {
	convertedRelabelConfigs, err := convertRelabelConfigs(config.WriteRelabelConfigs...)
	if err != nil {
		return nil, fmt.Errorf("failed to convert relabel configs: %w", err)
	}

	ds := &dataSourceRotated{
		headID:          headRecord.ID(),
		clock:           clock,
		corruptMarker:   corruptMarker,
		headReleaseFunc: headRecord.Acquire(),
		shards:          make([]*shardRotated, 0, numberOfShards),
		lsses:           make([]*cppbridge.LabelSetStorage, 0, numberOfShards),
		caches:          newCaches[*shardRotated](),
	}

	go ds.caches.cacheWriteLoop()

	for shardID := range numberOfShards {
		s, err := createShardRotated(
			ds.headID,
			shardID,
			filepath.Join(dataDir, fmt.Sprintf("shard_%d.wal", shardID)),
			filepath.Join(dataDir, fmt.Sprintf("%s_shard_%d.state", config.Name, shardID)),
			discardCache,
			config.ExternalLabels,
			convertedRelabelConfigs,
			segmentSize,
		)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("failed to create shard: %w", err), ds.Close())
		}

		ds.shards = append(ds.shards, s)
		ds.lsses = append(ds.lsses, s.decoder.lss)
		ds.caches.append(s.decoderStateFile, shardID)
	}

	return ds, nil
}

// Close write caches and closes the data source and releases the resources.
func (ds *dataSourceRotated) Close() error {
	if ds.closed {
		return nil
	}
	ds.closed = true

	// stop cache writing first
	ds.caches.close()

	var err error
	for _, s := range ds.shards {
		err = errors.Join(err, s.Close())
	}
	ds.headReleaseFunc()

	return err
}

// Init it initializes the data source by reading segments from shards until the required number is reached.
func (ds *dataSourceRotated) Init(ctx context.Context, targetSegmentID uint32) error {
	if targetSegmentID == 0 {
		return nil
	}

	now := ds.clock.Now().UnixMilli()
	segmentSampleStorages := cppbridge.NewSegmentSamplesStorage(
		uint64(len(ds.shards)), // #nosec G115 // no overflow
	)

	wg := sync.WaitGroup{}
	for i := range ds.shards {
		if ds.shards[i].corrupted {
			continue
		}

		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			ds.restoreShard(ctx, now, segmentSampleStorages, id, targetSegmentID)
		}(i)
	}
	wg.Wait()

	return nil
}

// LSSes returns the label set storages of the shards.
func (ds *dataSourceRotated) LSSes() []*cppbridge.LabelSetStorage {
	return ds.lsses
}

// Next checks the segmentID for readiness and reads the [DecodedSegment] from the shards.
func (ds *dataSourceRotated) Next(
	_ context.Context,
	minTimestamp int64,
	segmentSamplesStorages *cppbridge.SegmentSamplesStorageList,
) ([]*DecodedSegment, error) {
	// shardIDs are needed for V2 to read only recorded segments,
	// otherwise there will be an attempt to read the sync data
	shardIDs := ds.selectNextSegment()
	if len(shardIDs) == 0 {
		return nil, ds.finalize()
	}

	readShardResults := ds.readFromShards(shardIDs, minTimestamp, segmentSamplesStorages)
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

	return segments, handleReadErrors(errs, ds.markCorrupted, ds.checkFullCorrupted, len(ds.shards))
}

// NumberOfLSSes returns the number of label set storages.
func (ds *dataSourceRotated) NumberOfLSSes() int {
	return len(ds.lsses)
}

// WriteCaches writes caches to the buffer and sends the signal to write the caches.
func (ds *dataSourceRotated) WriteCaches() {
	ds.caches.writeCaches(ds.shards)
}

// checkFullCorrupted checks if all the shards are corrupted, if all the shards are corrupted,
// there is no point in continuing to read the shards, we return an error.
func (ds *dataSourceRotated) checkFullCorrupted() error {
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

// finalize finalizes the data source by marking the head as corrupted if one of the shards are corrupted.
func (ds *dataSourceRotated) finalize() error {
	for _, s := range ds.shards {
		if !s.corrupted {
			continue
		}

		if ds.corruptMarker == nil {
			return ErrEndOfBlock
		}

		ds.markCorrupted()
	}

	return ErrEndOfBlock
}

// markCorrupted marks the head as corrupted.
func (ds *dataSourceRotated) markCorrupted() {
	if ds.corruptMarker == nil {
		return
	}

	if err := ds.corruptMarker.MarkCorrupted(ds.headID); err != nil {
		logger.Errorf(
			"datasource: %s failed to mark head corrupted: %v",
			ds.headID,
			err,
		)

		return
	}

	ds.corruptMarker = nil
}

// readFromShards parallel reading of [DecodedSegment] from shards.
func (ds *dataSourceRotated) readFromShards(
	shardIDs []uint16,
	minTimestamp int64,
	segmentSamplesStorages *cppbridge.SegmentSamplesStorageList,
) []readShardResult {
	readShardResults := make([]readShardResult, len(shardIDs))
	if len(shardIDs) == 1 {
		if ds.shards[shardIDs[0]].corrupted {
			readShardResults[0] = readShardResult{
				segment: nil,
				err:     NewShardError(ds.headID, shardIDs[0], false, ErrShardIsCorrupted),
			}
			return readShardResults
		}

		segment, err := ds.shards[shardIDs[0]].ReadSegment(
			minTimestamp,
			segmentSamplesStorages.Get(uint64(shardIDs[0])),
		)
		if err != nil {
			err = NewShardError(ds.headID, shardIDs[0], true, err)
		}
		readShardResults[0] = readShardResult{segment: segment, err: err}

		return readShardResults
	}

	wg := sync.WaitGroup{}
	for i, shardID := range shardIDs {
		if ds.shards[shardID].corrupted {
			readShardResults[i] = readShardResult{
				segment: nil,
				err:     NewShardError(ds.headID, shardID, false, ErrShardIsCorrupted),
			}
			continue
		}
		wg.Add(1)
		go func(id int, shardID uint16) {
			defer wg.Done()
			segment, err := ds.shards[shardID].ReadSegment(
				minTimestamp,
				segmentSamplesStorages.Get(uint64(shardID)),
			)
			if err != nil {
				err = NewShardError(ds.headID, shardID, true, err)
			}
			readShardResults[id] = readShardResult{segment: segment, err: err}
		}(i, shardID)
	}
	wg.Wait()

	return readShardResults
}

// restoreShard restores the shard by reading the shard from the disk.
func (ds *dataSourceRotated) restoreShard(
	ctx context.Context,
	minTimestamp int64,
	segmentSampleStorages *cppbridge.SegmentSamplesStorageList,
	id int,
	targetSegmentID uint32,
) {
	s := ds.shards[id]

	for {
		if ctx.Err() != nil {
			return
		}

		segmentID, err := s.SegmentID()
		if err != nil {
			if errors.Is(err, ErrShardIsCorrupted) {
				ds.markCorrupted()
			}

			return
		}

		if segmentID >= targetSegmentID {
			return
		}

		err = s.SkipSegment(minTimestamp, segmentSampleStorages.Get(uint64(s.shardID)))
		if err != nil {
			if errors.Is(err, ErrShardIsCorrupted) {
				ds.markCorrupted()
			}

			return
		}
	}
}

// selectNextSegment selects the next segment to read from the shards.
func (ds *dataSourceRotated) selectNextSegment() []uint16 {
	nextSegments := make([]segmentByShard, 0, len(ds.shards))
	minSegmentID := uint32(math.MaxUint32)

	for _, shard := range ds.shards {
		if shard.corrupted || shard.completed {
			continue
		}

		segmentID, err := shard.SegmentID()
		if err != nil {
			if errors.Is(err, ErrShardIsCorrupted) {
				ds.markCorrupted()
			}

			continue
		}
		if segmentID < minSegmentID {
			minSegmentID = segmentID
		}

		nextSegments = append(nextSegments, segmentByShard{segmentID: segmentID, shardID: shard.shardID})
	}

	return ds.selectShards(nextSegments, minSegmentID)
}

// selectShards selects the shards to read the next segment from shards with the minimum segment ID.
func (*dataSourceRotated) selectShards(nextSegments []segmentByShard, minSegmentID uint32) []uint16 {
	if len(nextSegments) == 0 {
		return nil
	}

	shardIDs := make([]uint16, 0, len(nextSegments))
	for _, ns := range nextSegments {
		if ns.segmentID > minSegmentID {
			continue
		}

		shardIDs = append(shardIDs, ns.shardID)
	}

	return shardIDs
}

//
// segmentByShard
//

// segmentByShard a segment by shard.
type segmentByShard struct {
	segmentID uint32
	shardID   uint16
}

//
// caches
//

// caches a cache for writing data from shards to the disk.
type caches[TWT io.WriterTo] struct {
	cacheMtx             sync.Mutex
	caches               []*shardCache
	cacheWriteSignal     chan struct{}
	cacheWriteLoopClosed chan struct{}
}

// newCaches creates a new [caches].
func newCaches[TWT io.WriterTo]() *caches[TWT] {
	return &caches[TWT]{
		cacheWriteSignal:     make(chan struct{}),
		cacheWriteLoopClosed: make(chan struct{}),
	}
}

// append appends a new cache to the caches.
func (c *caches[TWT]) append(writer io.Writer, shardID uint16) {
	c.caches = append(c.caches, &shardCache{
		shardID: shardID,
		cache:   bytes.NewBuffer(nil),
		written: true,
		writer:  writer,
	})
}

// cacheWriteLoop loop that writes caches to the buffer and sends the signal to write the caches.
func (c *caches[TWT]) cacheWriteLoop() {
	defer close(c.cacheWriteLoopClosed)
	var closed bool
	var writeRequested bool
	var writeResultc chan struct{}

	for {
		if writeRequested && !closed && writeResultc == nil {
			writeResultc = make(chan struct{})
			go func() {
				defer close(writeResultc)
				c.writeBufferedCaches()
			}()
			writeRequested = false
		}

		if closed && writeResultc == nil {
			return
		}

		select {
		case _, ok := <-c.cacheWriteSignal:
			if !ok {
				return
			}
			writeRequested = true
		case <-writeResultc:
			writeResultc = nil
		}
	}
}

// close stops the cache writing loop.
func (c *caches[TWT]) close() {
	// stop cache writing first
	close(c.cacheWriteSignal)
	<-c.cacheWriteLoopClosed
}

// writeBufferedCaches writes cached data to the disk and resets the cache.
func (c *caches[TWT]) writeBufferedCaches() {
	c.cacheMtx.Lock()
	caches := make([]*shardCache, 0, len(c.caches))
	for i := range c.caches {
		if c.caches[i].written {
			continue
		}

		caches = append(caches, c.caches[i])
	}
	c.cacheMtx.Unlock()

	writtenCacheShardIDs := make([]uint16, 0, len(caches))
	for _, sc := range caches {
		if _, err := sc.cache.WriteTo(sc.writer); err != nil {
			logger.Errorf("failed to write cache: %v", err)
			continue
		}

		writtenCacheShardIDs = append(writtenCacheShardIDs, sc.shardID)
	}

	if len(writtenCacheShardIDs) > 0 {
		c.cacheMtx.Lock()
		for _, shardID := range writtenCacheShardIDs {
			c.caches[shardID].written = true
		}
		c.cacheMtx.Unlock()
	}
}

// writeCaches writes caches to the buffer and sends the signal to write the caches.
func (c *caches[TWT]) writeCaches(wts []TWT) {
	c.cacheMtx.Lock()
	for shardID, sc := range c.caches {
		if !sc.written {
			continue
		}

		sc.cache.Reset()
		if _, err := wts[shardID].WriteTo(sc.cache); err != nil {
			logger.Errorf("failed to get output decoder cache: %v", err)
			continue
		}

		sc.written = false
	}
	c.cacheMtx.Unlock()

	select {
	case c.cacheWriteSignal <- struct{}{}:
	default:
	}
}

//
// functions
//

// handleReadErrors handles the errors from the shards and returns an [ErrEndOfBlock] if the data source is corrupted.
func handleReadErrors(
	errs []error,
	markCorrupted func(),
	checkFullCorrupted func() error,
	numberOfShards int,
) error {
	if len(errs) == 0 {
		return nil
	}

	if len(errs) == numberOfShards {
		markCorrupted()

		return ErrEndOfBlock
	}

	markCorrupted()

	printErrorIfNeed(errs)

	return checkFullCorrupted()
}

// printErrorIfNeed logs errors if necessary.
func printErrorIfNeed(errs []error) {
	for _, err := range errs {
		if errors.Is(err, context.Canceled) {
			continue
		}

		var shardErr ShardError
		if errors.As(err, &shardErr) && shardErr.processable {
			logger.Errorf("shard %s/%d is corrupted: %s", shardErr.headID, shardErr.ShardID(), shardErr.Error())
		}
	}
}
