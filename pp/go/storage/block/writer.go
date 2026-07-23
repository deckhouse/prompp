package block

import (
	"errors"
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
	// LsIdBatchSize is the batch size for label set ID.
	LsIdBatchSize uint32 = 100000

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
	downsamplingMs           int64
	shardLabelsCtor          func(numberOfShards, shardID uint16) map[string]string
	retentionPeriod          time.Duration
	clock                    clockwork.Clock
	blockWriteDuration       *prometheus.GaugeVec
}

// NewWriter creates a new [Writer].
//
// retentionPeriod, when greater than zero, prevents creating blocks whose whole
// time range is already older than the retention period: such blocks would be
// deleted on the very next retention pass, so writing them is pointless work.
func NewWriter[TShard Shard](
	dataDir string,
	maxBlockChunkSegmentSize, downsamplingMs int64,
	blockDuration time.Duration,
	retentionPeriod time.Duration,
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
		downsamplingMs:           downsamplingMs,
		retentionPeriod:          retentionPeriod,
		shardLabelsCtor:          shardLabelsCtor,
		clock:                    clock,
		blockWriteDuration: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prompp_block_write_duration",
			Help: "Block write duration in milliseconds.",
		}, []string{"block_id"}),
	}
}

// Write writes blocks to disk from a shard.
func (w *Writer[TShard]) Write(sd TShard, numberOfShards uint16) (writtenBlocks []WrittenBlock, err error) {
	_ = sd.LSS().WithRLock(func(_, _ *cppbridge.LabelSetStorage) error {
		var writers blockWriters
		writers, err = w.createWriters(sd, numberOfShards)
		if err != nil {
			return err
		}
		defer func() {
			if errClose := writers.Close(); errClose != nil {
				logger.Warnf("Failed to close block writers: %v", errClose)
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
func (w *Writer[TShard]) createWriters(sd TShard, numberOfShards uint16) (blockWriters, error) {
	var writers blockWriters

	timeInterval := sd.DataStorage().TimeInterval(false)
	retentionCutoffMs, applyRetention := w.retentionCutoffMs()
	lss := sd.LSS().Target()
	tLabels := w.shardLabelsCtor(numberOfShards, sd.ShardID())
	quantStart := (timeInterval.MinT / w.blockDurationMs) * w.blockDurationMs
	for ; quantStart <= timeInterval.MaxT; quantStart += w.blockDurationMs {
		minT, maxT := quantStart, quantStart+w.blockDurationMs-1
		if minT < timeInterval.MinT {
			minT = timeInterval.MinT
		}
		if maxT > timeInterval.MaxT {
			maxT = timeInterval.MaxT
		}

		// Skip blocks whose whole time range is already beyond the retention
		// period: they would be deleted on the next retention pass anyway.
		if applyRetention && maxT <= retentionCutoffMs {
			continue
		}

		writer, err := w.createWriter(w.dataDir, sd, lss, minT, maxT, cppbridge.NoDownsampling, tLabels)
		if err != nil {
			return blockWriters{}, errors.Join(err, writers.Close())
		}

		writers.append(writer)

		if w.downsamplingMs == cppbridge.NoDownsampling {
			continue
		}

		downsamplingWriter, err := w.createWriter(w.dataDir, sd, lss, minT, maxT, w.downsamplingMs, tLabels)
		if err != nil {
			return blockWriters{}, errors.Join(err, writers.Close())
		}

		writers.append(downsamplingWriter)
	}

	return writers, nil
}

func (w *Writer[TShard]) createWriter(
	dataDir string,
	sd TShard,
	lss *cppbridge.LabelSetStorage,
	minT, maxT, downsamplingMs int64,
	tLabels map[string]string,
) (blockWriter, error) {
	var chunkIterator ChunkIterator
	_ = sd.DataStorage().WithRLock(func(ds *cppbridge.DataStorage) error {
		chunkIterator = NewChunkIterator(lss, LsIdBatchSize, ds, minT, maxT, downsamplingMs)
		return nil
	})

	return newBlockWriter(
		dataDir,
		w.maxBlockChunkSegmentSize,
		downsamplingMs,
		NewIndexWriter(lss),
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

// recodeAndWriteChunks recodes and writes chunks for the shard.
func (*Writer[TShard]) recodeAndWriteChunks(sd TShard, writers blockWriters) error {
	var loader *cppbridge.UnloadedDataRevertableLoader
	_ = sd.DataStorage().WithRLock(func(*cppbridge.DataStorage) error {
		loader = sd.DataStorage().CreateRevertableLoader(sd.LSS().Target(), LsIdBatchSize)
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
