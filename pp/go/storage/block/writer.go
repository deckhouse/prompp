package block

import (
	"strconv"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/util"
)

const (
	// DefaultChunkSegmentSize is the default chunks segment size.
	DefaultChunkSegmentSize = 512 * 1024 * 1024
	// DefaultBlockDuration is the default block duration.
	DefaultBlockDuration = 2 * time.Hour

	// shardIDLabel is the Thanos meta label holding the source shard ID. It is
	// written to block meta.json only (it does not become a series label, so
	// queries are unaffected) and is used as the compaction group key, so blocks
	// from different shards are never compacted together.
	shardIDLabel = "shard_id"

	// numberOfShardsLabel is the Thanos meta label holding the number of shards.
	// Adding number_of_shards keeps each (shard_id, number_of_shards) generation
	// isolated so blocks are never merged across a shard-count change.
	numberOfShardsLabel = "number_of_shards"
)

var (
	// LsIDBatchSize is the batch size for label set ID.
	LsIDBatchSize uint32 = 100000

	// EnableBlockShardLabels is a flag to enable block shard labels.
	EnableBlockShardLabels = false
)

// Shard the minimum required head [Shard] implementation.
type Shard interface {
	DataStorage() *shard.DataStorage

	LSS() *shard.LSS

	// ShardID returns the shard ID.
	ShardID() uint16

	UnloadedDataStorage() *shard.UnloadedDataStorage
}

// Writer represents a block writer. It is used to write blocks to disk from a shard.
type Writer[TShard Shard] struct {
	dataDir                  string
	maxBlockChunkSegmentSize int64
	blockDurationMs          int64
	shardLabelsCtor          func(numberOfShards, shardID uint16) map[string]string
	retentionPeriod          time.Duration
	futurePeriod             time.Duration
	clock                    clockwork.Clock
	blockWriteDuration       *prometheus.GaugeVec
	droppedFutureBlocks      *prometheus.CounterVec
}

// NewWriter creates a new [Writer].
//
// retentionPeriod, when greater than zero, prevents creating blocks whose whole
// time range is already older than the retention period: such blocks would be
// deleted on the very next retention pass, so writing them is pointless work.
//
// futurePeriod, when greater than zero, drops the samples of quants that start
// later than now plus the future period, rounded down to a quant boundary: such
// samples come from a clock jump on the source side. The filter works on quants,
// not on the block MinTime, so samples up to futurePeriod plus one block duration
// ahead of now are kept.
func NewWriter[TShard Shard](
	dataDir string,
	maxBlockChunkSegmentSize int64,
	blockDuration time.Duration,
	retentionPeriod time.Duration,
	futurePeriod time.Duration,
	clock clockwork.Clock,
	registerer prometheus.Registerer,
) *Writer[TShard] {
	if clock == nil {
		clock = clockwork.NewRealClock()
	}

	shardLabelsCtor := noopShardLabelsCtorFunc
	if EnableBlockShardLabels {
		shardLabelsCtor = shardLabelsCtorFunc
	}

	factory := util.NewUnconflictRegisterer(registerer)
	return &Writer[TShard]{
		dataDir:                  dataDir,
		maxBlockChunkSegmentSize: maxBlockChunkSegmentSize,
		blockDurationMs:          blockDuration.Milliseconds(),
		retentionPeriod:          retentionPeriod,
		futurePeriod:             futurePeriod,
		shardLabelsCtor:          shardLabelsCtor,
		clock:                    clock,
		blockWriteDuration: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prompp_block_write_duration",
			Help: "Block write duration in milliseconds.",
		}, []string{"block_id"}),
		droppedFutureBlocks: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "prompp_block_writer_dropped_future_blocks_total",
			Help: "Number of blocks not written because they start later than now plus the future period, " +
				"rounded down to a block boundary.",
		}, []string{"shard_id"}),
	}
}

// Write writes blocks to disk from a shard.
func (w *Writer[TShard]) Write(sd TShard, numberOfShards uint16) (writtenBlocks []WrittenBlock, err error) {
	_ = sd.LSS().WithRLock(func(_, _ *cppbridge.LabelSetStorage) error {
		writers := w.createWriters(sd, numberOfShards)
		defer func() {
			if closeErr := writers.Close(); closeErr != nil {
				logger.Warnf("Failed to close block writers: %v", closeErr)
			}
		}()

		if err = w.recodeAndWriteChunks(sd, writers); err != nil {
			return err
		}

		writtenBlocks, err = writers.writeIndexCloseAndMoveTmpDirToDir()

		return nil
	})

	return writtenBlocks, err
}

// createWriters creates writers for the shard.
func (w *Writer[TShard]) createWriters(sd TShard, numberOfShards uint16) blockWriters {
	var writers blockWriters

	retentionCutoffMs, applyRetention := w.retentionCutoffMs()
	futureCutoffMs, applyFuture := w.futureCutoffMs()
	tLabels := w.shardLabelsCtor(numberOfShards, sd.ShardID())

	// The last quant is clamped to the last sample, so that a quant with old samples only is skipped too.
	var retentionMaxT int64
	if applyRetention {
		retentionMaxT = sd.DataStorage().TimeInterval(true).MaxT
	}

	// Only quants with samples get a writer: a far-future or far-past sample widens the shard time
	// interval to thousands of quants, and every writer recodes every batch of the shard.
	var quantStarts []int64
	_ = sd.DataStorage().WithRLock(func(ds *cppbridge.DataStorage) error {
		quantStarts = ds.NonEmptyQuants(w.blockDurationMs)
		return nil
	})

	var skippedFutureQuants []int64
	for _, quantStart := range quantStarts {
		minT, maxT := quantStart, quantStart+w.blockDurationMs-1

		if applyFuture && minT > futureCutoffMs {
			skippedFutureQuants = append(skippedFutureQuants, minT)
			continue
		}

		// Skip blocks whose whole time range is already beyond the retention
		// period: they would be deleted on the next retention pass anyway.
		if applyRetention && min(maxT, retentionMaxT) <= retentionCutoffMs {
			continue
		}

		writers.append(w.createWriter(w.dataDir, sd, sd.LSS().Target(), minT, maxT, cppbridge.NoDownsampling, tLabels))
	}

	if len(skippedFutureQuants) > 0 {
		w.droppedFutureBlocks.WithLabelValues(
			//revive:disable-next-line:add-constant // it's base 10
			strconv.FormatUint(uint64(sd.ShardID()), 10),
		).Add(float64(len(skippedFutureQuants)))
		logger.Warnf(
			"[BlockWriter]: shard %d: dropped samples of %d blocks later than %s, the first block starts at %s",
			sd.ShardID(),
			len(skippedFutureQuants),
			time.UnixMilli(futureCutoffMs).UTC().Format(time.RFC3339),
			time.UnixMilli(skippedFutureQuants[0]).UTC().Format(time.RFC3339),
		)
	}

	return writers
}

func (w *Writer[TShard]) createWriter(
	dataDir string,
	sd TShard,
	lss *cppbridge.LabelSetStorage,
	minT, maxT, downsamplingMs int64,
	tLabels map[string]string,
) blockWriter {
	var chunkIterator ChunkIterator
	_ = sd.DataStorage().WithRLock(func(ds *cppbridge.DataStorage) error {
		chunkIterator = NewChunkIterator(lss, LsIDBatchSize, ds, minT, maxT, downsamplingMs)
		return nil
	})

	return newBlockWriter(
		dataDir,
		w.maxBlockChunkSegmentSize,
		lss,
		chunkIterator,
		tLabels,
	)
}

// retentionCutoffMs returns the max time (in unix milliseconds) below which a
// block is already beyond the retention period, and whether retention filtering
// is enabled at all. A block whose MaxTime is at or before the cutoff would be
// deleted on the next retention pass, so it should not be created.
func (w *Writer[TShard]) retentionCutoffMs() (cutoffMs int64, apply bool) {
	if w.retentionPeriod <= 0 {
		return 0, false
	}

	return w.clock.Now().Add(-w.retentionPeriod).UnixMilli(), true
}

// futureCutoffMs returns the start (in unix milliseconds) of the last quant that
// is not in the future yet, and whether future filtering is enabled at all.
func (w *Writer[TShard]) futureCutoffMs() (cutoffMs int64, apply bool) {
	if w.futurePeriod <= 0 {
		return 0, false
	}

	return quantStartMs(w.clock.Now().Add(w.futurePeriod).UnixMilli(), w.blockDurationMs), true
}

// quantStartMs returns the start of the quantDurationMs-aligned quant that holds timestampMs.
func quantStartMs(timestampMs, quantDurationMs int64) int64 {
	start := timestampMs - timestampMs%quantDurationMs
	if start > timestampMs {
		start -= quantDurationMs
	}

	return start
}

// recodeAndWriteChunks recodes and writes chunks for the shard.
func (*Writer[TShard]) recodeAndWriteChunks(sd TShard, writers blockWriters) error {
	var loader *cppbridge.UnloadedDataRevertableLoader
	_ = sd.DataStorage().WithRLock(func(*cppbridge.DataStorage) error {
		loader = sd.DataStorage().CreateRevertableLoader(sd.LSS().Target(), LsIDBatchSize)
		return nil
	})

	isFirstBatch := true

	loadData := func() (bool, error) {
		if isFirstBatch {
			isFirstBatch = false
		} else if !loader.NextBatch() {
			return false, nil
		}

		if sd.UnloadedDataStorage() == nil {
			return true, nil
		}

		return true, sd.UnloadedDataStorage().ForEachSnapshot(loader.Load)
	}

	for {
		var hasMoreData bool
		var err error
		_ = sd.DataStorage().WithLock(func(*cppbridge.DataStorage) error {
			hasMoreData, err = loadData()
			return nil
		})

		if !hasMoreData {
			break
		}

		if err != nil {
			return err
		}

		if err = sd.DataStorage().WithRLock(func(*cppbridge.DataStorage) error {
			return writers.recodeAndWriteChunksBatch()
		}); err != nil {
			return err
		}
	}

	return writers.writeRestOfRecodedChunks()
}

// shardLabelsCtorFunc constructs the shard labels.
func shardLabelsCtorFunc(numberOfShards, shardID uint16) map[string]string {
	return map[string]string{
		//revive:disable-next-line:add-constant // it's base 10
		shardIDLabel: strconv.FormatUint(uint64(shardID), 10),
		//revive:disable-next-line:add-constant // it's base 10
		numberOfShardsLabel: strconv.FormatUint(uint64(numberOfShards), 10),
	}
}

// noopShardLabelsCtorFunc constructs the shard labels.
func noopShardLabelsCtorFunc(_, _ uint16) map[string]string {
	return nil
}
