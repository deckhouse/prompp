package block

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/util"
)

const (
	// DefaultChunkSegmentSize is the default chunks segment size.
	DefaultChunkSegmentSize = 512 * 1024 * 1024
	// DefaultBlockDuration is the default block duration.
	DefaultBlockDuration = 2 * time.Hour
)

var LsIdBatchSize uint32 = 100000

// Shard the minimum required head [Shard] implementation.
type Shard interface {
	LSS() *shard.LSS

	DataStorage() *shard.DataStorage

	UnloadedDataStorage() *shard.UnloadedDataStorage
}

type Writer[TShard Shard] struct {
	dataDir                  string
	maxBlockChunkSegmentSize int64
	blockDurationMs          int64
	blockWriteDuration       *prometheus.GaugeVec
}

func NewWriter[TShard Shard](
	dataDir string,
	maxBlockChunkSegmentSize int64,
	blockDuration time.Duration,
	registerer prometheus.Registerer,
) *Writer[TShard] {
	factory := util.NewUnconflictRegisterer(registerer)
	return &Writer[TShard]{
		dataDir:                  dataDir,
		maxBlockChunkSegmentSize: maxBlockChunkSegmentSize,
		blockDurationMs:          blockDuration.Milliseconds(),
		blockWriteDuration: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prompp_block_write_duration",
			Help: "Block write duration in milliseconds.",
		}, []string{"block_id"}),
	}
}

func (w *Writer[TShard]) Write(shard TShard) (writtenBlocks []WrittenBlock, err error) {
	_ = shard.LSS().WithRLock(func(_, _ *cppbridge.LabelSetStorage) error {
		var writers blockWriters
		writers, err = w.createWriters(shard)
		if err != nil {
			return err
		}

		defer func() {
			writers.close()
		}()

		if err = w.recodeAndWriteChunks(shard, writers); err != nil {
			return err
		}

		writtenBlocks, err = writers.writeIndexAndMoveTmpDirToDir()
		return nil
	})
	return
}

func (w *Writer[TShard]) createWriters(shard TShard) (blockWriters, error) {
	var writers blockWriters

	timeInterval := shard.DataStorage().TimeInterval(false)

	quantStart := (timeInterval.MinT / w.blockDurationMs) * w.blockDurationMs
	for ; quantStart <= timeInterval.MaxT; quantStart += w.blockDurationMs {
		minT, maxT := quantStart, quantStart+w.blockDurationMs-1
		if minT < timeInterval.MinT {
			minT = timeInterval.MinT
		}
		if maxT > timeInterval.MaxT {
			maxT = timeInterval.MaxT
		}

		var chunkIterator ChunkIterator
		_ = shard.DataStorage().WithRLock(func(ds *cppbridge.HeadDataStorage) error {
			chunkIterator = NewChunkIterator(shard.LSS().Target(), LsIdBatchSize, shard.DataStorage().Raw(), minT, maxT)
			return nil
		})

		if writer, err := newBlockWriter(w.dataDir, w.maxBlockChunkSegmentSize, NewIndexWriter(shard.LSS().Target()), chunkIterator); err == nil {
			writers.append(writer)
		} else {
			writers.close()
			return blockWriters{}, err
		}
	}

	return writers, nil
}

func (w *Writer[TShard]) recodeAndWriteChunks(shard TShard, writers blockWriters) error {
	var loader *cppbridge.UnloadedDataRevertableLoader
	_ = shard.DataStorage().WithRLock(func(ds *cppbridge.HeadDataStorage) error {
		loader = shard.DataStorage().CreateRevertableLoader(shard.LSS().Target(), LsIdBatchSize)
		return nil
	})

	isFirstBatch := true

	loadData := func() (bool, error) {
		if isFirstBatch {
			isFirstBatch = false
		} else {
			if !loader.NextBatch() {
				return false, nil
			}
		}

		if shard.UnloadedDataStorage() == nil {
			return true, nil
		}

		return true, shard.UnloadedDataStorage().ForEachSnapshot(loader.Load)
	}

	for {
		var hasMoreData bool
		var err error
		_ = shard.DataStorage().WithLock(func(ds *cppbridge.HeadDataStorage) error {
			hasMoreData, err = loadData()
			return nil
		})

		if !hasMoreData {
			break
		}

		if err != nil {
			return err
		}

		if err = shard.DataStorage().WithRLock(func(ds *cppbridge.HeadDataStorage) error {
			return writers.recodeAndWriteChunksBatch()
		}); err != nil {
			return err
		}
	}

	return writers.writeRestOfRecodedChunks()
}
