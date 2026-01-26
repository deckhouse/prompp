package block

import (
	"errors"
	"time"

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
)

// LsIdBatchSize is the batch size for label set ID.
var LsIdBatchSize uint32 = 100000

// Shard the minimum required head [Shard] implementation.
type Shard interface {
	LSS() *shard.LSS

	DataStorage() *shard.DataStorage

	UnloadedDataStorage() *shard.UnloadedDataStorage
}

// Writer represents a block writer. It is used to write blocks to disk from a shard.
type Writer[TShard Shard] struct {
	dataDir                  string
	longtermDataDir          string
	maxBlockChunkSegmentSize int64
	blockDurationMs          int64
	longtermIntervalMs       int64
	blockWriteDuration       *prometheus.GaugeVec
}

// NewWriter creates a new [Writer].
func NewWriter[TShard Shard](
	dataDir, longtermDataDir string,
	maxBlockChunkSegmentSize, longtermIntervalMs int64,
	blockDuration time.Duration,
	registerer prometheus.Registerer,
) *Writer[TShard] {
	factory := util.NewUnconflictRegisterer(registerer)
	return &Writer[TShard]{
		dataDir:                  dataDir,
		longtermDataDir:          longtermDataDir,
		maxBlockChunkSegmentSize: maxBlockChunkSegmentSize,
		blockDurationMs:          blockDuration.Milliseconds(),
		longtermIntervalMs:       longtermIntervalMs,
		blockWriteDuration: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prompp_block_write_duration",
			Help: "Block write duration in milliseconds.",
		}, []string{"block_id"}),
	}
}

// Write writes blocks to disk from a shard.
func (w *Writer[TShard]) Write(sd TShard) (writtenBlocks []WrittenBlock, err error) {
	_ = sd.LSS().WithRLock(func(_, _ *cppbridge.LabelSetStorage) error {
		var writers blockWriters
		writers, err = w.createWriters(sd)
		if err != nil {
			return err
		}
		defer func() {
			if err := writers.Close(); err != nil {
				logger.Warnf("Failed to close block writers: %v", err)
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
func (w *Writer[TShard]) createWriters(sd TShard) (blockWriters, error) {
	var writers blockWriters

	timeInterval := sd.DataStorage().TimeInterval(false)

	lss := sd.LSS().Target()
	quantStart := (timeInterval.MinT / w.blockDurationMs) * w.blockDurationMs
	for ; quantStart <= timeInterval.MaxT; quantStart += w.blockDurationMs {
		minT, maxT := quantStart, quantStart+w.blockDurationMs-1
		if minT < timeInterval.MinT {
			minT = timeInterval.MinT
		}
		if maxT > timeInterval.MaxT {
			maxT = timeInterval.MaxT
		}

		writer, err := w.createWriter(w.dataDir, sd, lss, minT, maxT, cppbridge.NoDownsampling)
		if err != nil {
			return blockWriters{}, errors.Join(err, writers.Close())
		}

		writers.append(writer)

		if w.longtermDataDir == "" {
			continue
		}

		longtermWriter, err := w.createWriter(w.longtermDataDir, sd, lss, minT, maxT, w.longtermIntervalMs)
		if err != nil {
			writers.close()
			return blockWriters{}, err
		}

		writers.append(longtermWriter)
	}

	return writers, nil
}

func (w *Writer[TShard]) createWriter(
	dataDir string,
	sd TShard,
	lss *cppbridge.LabelSetStorage,
	minT, maxT, downsamplingMs int64,
) (blockWriter, error) {
	var chunkIterator ChunkIterator
	_ = sd.DataStorage().WithRLock(func(ds *cppbridge.DataStorage) error {
		chunkIterator = NewChunkIterator(lss, LsIdBatchSize, ds, minT, maxT, downsamplingMs)
		return nil
	})

	return newBlockWriter(
		dataDir,
		w.maxBlockChunkSegmentSize,
		NewIndexWriter(lss),
		chunkIterator,
	)
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
