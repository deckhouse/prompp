package block

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/fileutil"
)

const (
	// DefaultChunkSegmentSize is the default chunks segment size.
	DefaultChunkSegmentSize = 512 * 1024 * 1024
	// DefaultBlockDuration is the default block duration.
	DefaultBlockDuration         = 2 * time.Hour
	tmpForCreationBlockDirSuffix = ".tmp-for-creation"
	indexFilename                = "index"
	metaFilename                 = "meta.json"
	metaVersion1                 = 1
)

type chunkRecoder struct {
	chunkIterator    ChunkIterator
	chunksMetadata   []ChunkMetadata
	previousSeriesID uint32
}

func (recoder *chunkRecoder) Recode(
	chunkWriter *ChunkWriter,
	blockMeta *tsdb.BlockMeta,
	writeSeries func(seriesID uint32, chunksMetadata []ChunkMetadata) error,
) (err error) {
	for recoder.chunkIterator.Next() {
		chunk := recoder.chunkIterator.At()

		var chunkMetadata ChunkMetadata
		if chunkMetadata, err = chunkWriter.Write(chunk); err != nil {
			return fmt.Errorf("failed to write chunk: %w", err)
		}

		adjustBlockMetaTimeRange(blockMeta, chunk.MinT(), chunk.MaxT())
		blockMeta.Stats.NumChunks++
		blockMeta.Stats.NumSamples += uint64(chunk.SampleCount())
		seriesID := chunk.SeriesID()

		if recoder.previousSeriesID == seriesID {
			recoder.chunksMetadata = append(recoder.chunksMetadata, chunkMetadata)
		} else {
			if err = writeSeries(recoder.previousSeriesID, recoder.chunksMetadata); err != nil {
				return err
			}
			blockMeta.Stats.NumSeries++
			recoder.chunksMetadata = append(recoder.chunksMetadata[:0], chunkMetadata)
			recoder.previousSeriesID = seriesID
		}
	}

	recoder.chunkIterator.NextBatch()
	return nil
}

func newChunkRecoder(chunkIterator ChunkIterator) chunkRecoder {
	return chunkRecoder{
		chunkIterator:    chunkIterator,
		previousSeriesID: math.MaxUint32,
	}
}

type WrittenBlock struct {
	Dir  string
	Meta tsdb.BlockMeta
}

func (block *WrittenBlock) ChunkDir() string {
	return filepath.Join(block.Dir, "chunks")
}

func (block *WrittenBlock) IndexFilename() string {
	return filepath.Join(block.Dir, indexFilename)
}

func (block *WrittenBlock) MetaFilename() string {
	return filepath.Join(block.Dir, metaFilename)
}

type blockWriter struct {
	WrittenBlock

	chunkWriter     *ChunkWriter
	indexFileWriter *FileWriter
	indexWriter     IndexWriter

	chunkRecoder chunkRecoder
}

func newBlockWriter(dir string, maxBlockChunkSegmentSize int64, indexWriter IndexWriter, chunkIterator ChunkIterator) (writer blockWriter, err error) {
	uid := ulid.MustNew(ulid.Now(), rand.Reader)
	writer.Dir = filepath.Join(dir, uid.String()) + tmpForCreationBlockDirSuffix

	if err = createTmpDir(writer.Dir); err != nil {
		return
	}

	if err = writer.createWriters(maxBlockChunkSegmentSize); err != nil {
		return
	}

	writer.Meta = tsdb.BlockMeta{
		ULID:    uid,
		MinTime: math.MaxInt64,
		MaxTime: math.MinInt64,
		Version: metaVersion1,
		Compaction: tsdb.BlockMetaCompaction{
			Level:   1,
			Sources: []ulid.ULID{uid},
		},
	}

	writer.indexWriter = indexWriter
	writer.chunkRecoder = newChunkRecoder(chunkIterator)
	return
}

func (writer *blockWriter) createWriters(maxBlockChunkSegmentSize int64) error {
	chunkWriter, err := NewChunkWriter(writer.ChunkDir(), maxBlockChunkSegmentSize)
	if err != nil {
		return fmt.Errorf("failed to create chunk writer: %w", err)
	}

	indexFileWriter, err := NewFileWriter(writer.IndexFilename())
	if err != nil {
		_ = chunkWriter.Close()
		return fmt.Errorf("failed to create index file writer: %w", err)
	}

	writer.chunkWriter = chunkWriter
	writer.indexFileWriter = indexFileWriter
	return nil
}

func (writer *blockWriter) Close() error {
	return closeAll(writer.chunkWriter, writer.indexFileWriter)
}

func (writer *blockWriter) RecodeAndWriteChunksBatch() error {
	return writer.chunkRecoder.Recode(writer.chunkWriter, &writer.Meta, writer.writeSeries)
}

func (writer *blockWriter) WriteRestOfRecodedChunks() error {
	return writer.writeSeries(writer.chunkRecoder.previousSeriesID, writer.chunkRecoder.chunksMetadata)
}

func (writer *blockWriter) writeSeries(seriesID uint32, chunksMetadata []ChunkMetadata) error {
	if len(chunksMetadata) > 0 {
		if _, err := writer.indexWriter.WriteSeriesTo(seriesID, chunksMetadata, writer.indexFileWriter); err != nil {
			return fmt.Errorf("failed to write series %d: %w", seriesID, err)
		}
	}

	return nil
}

func (writer *blockWriter) writeIndex() error {
	if _, err := writer.indexWriter.WriteRestTo(writer.indexFileWriter); err != nil {
		return fmt.Errorf("failed to write index: %w", err)
	}

	writer.Meta.MaxTime += 1
	if _, err := writeBlockMetaFile(writer.MetaFilename(), &writer.Meta); err != nil {
		return fmt.Errorf("failed to write block meta file: %w", err)
	}

	return nil
}

func (writer *blockWriter) MoveTmpDirToDir() error {
	if err := syncDir(writer.Dir); err != nil {
		return fmt.Errorf("failed to sync temporary block dir: %w", err)
	}

	dir := writer.Dir[:len(writer.Dir)-len(tmpForCreationBlockDirSuffix)]

	if err := fileutil.Replace(writer.Dir, dir); err != nil {
		return fmt.Errorf("failed to move temporary block dir {%s} to {%s}: %w", writer.Dir, dir, err)
	}

	writer.Dir = dir
	return nil
}

type BlockWriter struct {
	dataDir                  string
	maxBlockChunkSegmentSize int64
	blockDurationMs          int64
	blockWriteDuration       *prometheus.GaugeVec
}

func NewBlockWriter(
	dataDir string,
	maxBlockChunkSegmentSize int64,
	blockDuration time.Duration,
	registerer prometheus.Registerer,
) *BlockWriter {
	factory := util.NewUnconflictRegisterer(registerer)
	return &BlockWriter{
		dataDir:                  dataDir,
		maxBlockChunkSegmentSize: maxBlockChunkSegmentSize,
		blockDurationMs:          blockDuration.Milliseconds(),
		blockWriteDuration: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prompp_block_write_duration",
			Help: "Block write duration in milliseconds.",
		}, []string{"block_id"}),
	}
}

func (w *BlockWriter) Write(
	dataStorage *cppbridge.HeadDataStorage,
	unloadedDataStorage relabeler.UnloadedDataStorage,
	lss *cppbridge.LabelSetStorage,
	lsIdBatchSize uint32,
) ([]WrittenBlock, error) {
	writers, err := w.createWriters(dataStorage, lss, lsIdBatchSize)
	if err != nil {
		return nil, err
	}

	defer func() {
		for _, w := range writers {
			_ = w.Close()
		}
	}()

	if err = w.recodeAndWriteChunks(unloadedDataStorage, dataStorage.CreateRevertableLoader(lss, lsIdBatchSize), writers); err != nil {
		return nil, err
	}

	writtenBlocks := make([]WrittenBlock, 0, len(writers))
	for i := range writers {
		if err = writers[i].writeIndex(); err != nil {
			return nil, err
		}

		if err = writers[i].MoveTmpDirToDir(); err != nil {
			return nil, err
		}

		writtenBlocks = append(writtenBlocks, writers[i].WrittenBlock)
	}

	return writtenBlocks, nil
}

func (w *BlockWriter) createWriters(dataStorage *cppbridge.HeadDataStorage, lss *cppbridge.LabelSetStorage, lsIdBatchSize uint32) ([]blockWriter, error) {
	var writers []blockWriter

	timeInterval := dataStorage.TimeInterval()
	quantStart := (timeInterval.MinT / w.blockDurationMs) * w.blockDurationMs
	for ; quantStart <= timeInterval.MaxT; quantStart += w.blockDurationMs {
		minT, maxT := quantStart, quantStart+w.blockDurationMs-1
		if minT < timeInterval.MinT {
			minT = timeInterval.MinT
		}
		if maxT > timeInterval.MaxT {
			maxT = timeInterval.MaxT
		}

		chunkIterator := NewChunkIterator(lss, lsIdBatchSize, dataStorage, minT, maxT)
		if writer, err := newBlockWriter(w.dataDir, w.maxBlockChunkSegmentSize, NewIndexWriter(lss), chunkIterator); err == nil {
			writers = append(writers, writer)
		} else {
			for _, wr := range writers {
				_ = wr.Close()
			}
			return nil, err
		}
	}

	return writers, nil
}

func (w *BlockWriter) recodeAndWriteChunks(
	unloadedDataStorage relabeler.UnloadedDataStorage,
	loader *cppbridge.UnloadedDataRevertableLoader,
	writers []blockWriter,
) error {
	for {
		if err := unloadedDataStorage.ForEachSnapshot(loader.Load); err != nil {
			return err
		}

		for i := range writers {
			if err := writers[i].RecodeAndWriteChunksBatch(); err != nil {
				return err
			}
		}

		if !loader.NextBatch() {
			break
		}
	}

	for i := range writers {
		if err := writers[i].WriteRestOfRecodedChunks(); err != nil {
			return err
		}
	}

	return nil
}

func closeAll(closers ...io.Closer) error {
	var errs error
	for _, closer := range closers {
		errs = errors.Join(errs, closer.Close())
	}
	return errs
}

func adjustBlockMetaTimeRange(blockMeta *tsdb.BlockMeta, mint, maxt int64) {
	if mint < blockMeta.MinTime {
		blockMeta.MinTime = mint
	}

	if maxt > blockMeta.MaxTime {
		blockMeta.MaxTime = maxt
	}
}

func writeBlockMetaFile(fileName string, blockMeta *tsdb.BlockMeta) (int64, error) {
	tmp := fileName + ".tmp"
	defer func() {
		if err := os.RemoveAll(tmp); err != nil {
			// todo: log error
		}
	}()

	metaFile, err := os.Create(tmp)
	if err != nil {
		return 0, fmt.Errorf("failed to create block meta file: %w", err)
	}
	defer func() {
		if metaFile != nil {
			if err = metaFile.Close(); err != nil {
				// todo: log error
			}
		}
	}()

	jsonBlockMeta, err := json.MarshalIndent(blockMeta, "", "\t")
	if err != nil {
		return 0, fmt.Errorf("failed to marshal meta json: %w", err)
	}

	n, err := metaFile.Write(jsonBlockMeta)
	if err != nil {
		return 0, fmt.Errorf("failed to write meta json: %w", err)
	}

	if err = metaFile.Sync(); err != nil {
		return 0, fmt.Errorf("failed to sync meta file: %w", err)
	}

	if err = metaFile.Close(); err != nil {
		return 0, fmt.Errorf("faield to close meta file: %w", err)
	}
	metaFile = nil

	return int64(n), fileutil.Replace(tmp, fileName)
}

func createTmpDir(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}

	return os.MkdirAll(dir, 0o777)
}

func syncDir(dir string) error {
	df, err := fileutil.OpenDir(dir)
	if err != nil {
		return err
	}
	defer func() {
		if df != nil {
			_ = df.Close()
		}
	}()

	if err = df.Sync(); err != nil {
		return err
	}

	return df.Close()
}
