package block

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/oklog/ulid"

	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/fileutil"

	"github.com/prometheus/prometheus/pp/go/logger"
)

const (
	tmpForCreationBlockDirSuffix = ".tmp-for-creation"

	indexFilename = "index"
	metaFilename  = "meta.json"

	metaVersion1 = 1
)

// WrittenBlock represents a written block.
type WrittenBlock struct {
	Dir  string
	Meta tsdb.BlockMeta
}

// ChunkDir returns the chunk directory.
func (block *WrittenBlock) ChunkDir() string {
	return filepath.Join(block.Dir, "chunks")
}

// IndexFilename returns the index filename.
func (block *WrittenBlock) IndexFilename() string {
	return filepath.Join(block.Dir, indexFilename)
}

// MetaFilename returns the meta filename.
func (block *WrittenBlock) MetaFilename() string {
	return filepath.Join(block.Dir, metaFilename)
}

type blockWriter struct {
	WrittenBlock

	chunkWriter     *ChunkWriter
	indexFileWriter *FileWriter
	indexWriter     IndexWriter

	chunkRecoder chunkRecoder
	finished     bool
}

func newBlockWriter(
	dir string,
	maxBlockChunkSegmentSize int64,
	indexWriter IndexWriter,
	chunkIterator ChunkIterator,
) (writer blockWriter, err error) {
	uid := ulid.MustNew(ulid.Now(), rand.Reader)
	writer.Dir = filepath.Join(dir, uid.String()) + tmpForCreationBlockDirSuffix

	if err = createTmpDir(writer.Dir); err != nil {
		return writer, err
	}

	if err = writer.createWriters(maxBlockChunkSegmentSize); err != nil {
		return writer, err
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

	return writer, err
}

// isEmpty returns true if [IndexWriter] contains no samples, an empty block.
func (writer *blockWriter) isEmpty() bool {
	return writer.indexWriter.isEmpty()
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

// Flush all temporary data.
func (writer *blockWriter) Flush() (err error) {
	if writer.chunkWriter != nil {
		if cwErr := writer.chunkWriter.Close(); cwErr != nil {
			err = errors.Join(err, cwErr)
		}
		writer.chunkWriter = nil
	}
	if writer.indexFileWriter != nil {
		if iwErr := writer.indexFileWriter.Close(); iwErr != nil {
			err = errors.Join(err, iwErr)
		}
		writer.indexFileWriter = nil
	}
	return err
}

// Close closes the block writer.
func (writer *blockWriter) Close() error {
	err := writer.Flush()
	if !writer.finished {
		err = errors.Join(err, os.RemoveAll(writer.Dir))
	}
	return err
}

func (writer *blockWriter) recodeAndWriteChunksBatch() error {
	return writer.chunkRecoder.recode(writer.chunkWriter, &writer.Meta, writer.writeSeries)
}

func (writer *blockWriter) writeRestOfRecodedChunks() error {
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

	writer.Meta.MaxTime++
	if _, err := writeBlockMetaFile(writer.MetaFilename(), &writer.Meta); err != nil {
		return fmt.Errorf("failed to write block meta file: %w", err)
	}

	return nil
}

func (writer *blockWriter) moveTmpDirToDir() error {
	if err := syncDir(writer.Dir); err != nil {
		return fmt.Errorf("failed to sync temporary block dir: %w", err)
	}

	dir := writer.Dir[:len(writer.Dir)-len(tmpForCreationBlockDirSuffix)]

	if err := fileutil.Replace(writer.Dir, dir); err != nil {
		return fmt.Errorf("failed to move temporary block dir {%s} to {%s}: %w", writer.Dir, dir, err)
	}

	writer.Dir = dir
	writer.finished = true
	return nil
}

type blockWriters []blockWriter

// append appends a writer to the block writers.

//nolint:gocritic // hugeParam // we accumulate the writers
func (bw *blockWriters) append(writer blockWriter) {
	*bw = append(*bw, writer)
}

// Close closes the block writers.
func (bw *blockWriters) Close() (err error) {
	for i := range *bw {
		err = errors.Join(err, (*bw)[i].Close())
	}
	return err
}

// recodeAndWriteChunksBatch recodes and writes the chunks batch.
func (bw *blockWriters) recodeAndWriteChunksBatch() error {
	for i := range *bw {
		if err := (*bw)[i].recodeAndWriteChunksBatch(); err != nil {
			return err
		}
	}

	return nil
}

// writeRestOfRecodedChunks writes the rest of the recoded chunks.
func (bw *blockWriters) writeRestOfRecodedChunks() error {
	for i := range *bw {
		if err := (*bw)[i].writeRestOfRecodedChunks(); err != nil {
			return err
		}
	}

	return nil
}

// writeIndexCloseAndMoveTmpDirToDir writes the index and moves the temporary directory to the directory.
func (bw *blockWriters) writeIndexCloseAndMoveTmpDirToDir() ([]WrittenBlock, error) {
	writtenBlocks := make([]WrittenBlock, 0, len(*bw))
	for i := range *bw {
		if (*bw)[i].isEmpty() {
			continue
		}

		if err := (*bw)[i].writeIndex(); err != nil {
			return nil, err
		}

		_ = (*bw)[i].Flush()

		if err := (*bw)[i].moveTmpDirToDir(); err != nil {
			return nil, err
		}

		writtenBlocks = append(writtenBlocks, (*bw)[i].WrittenBlock)
	}

	return writtenBlocks, nil
}

type chunkRecoder struct {
	chunkIterator    ChunkIterator
	chunksMetadata   []ChunkMetadata
	previousSeriesID uint32
}

func (recoder *chunkRecoder) recode(
	chunkWriter *ChunkWriter,
	blockMeta *tsdb.BlockMeta,
	writeSeries func(seriesID uint32, chunksMetadata []ChunkMetadata) error,
) (err error) {
	for chunk := range recoder.chunkIterator.RangeBatch {
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
			logger.Errorf("failed to remove directory: %v", err)
		}
	}()

	metaFile, err := os.Create(tmp) // #nosec G304 // it's meant to be that way
	if err != nil {
		return 0, fmt.Errorf("failed to create block meta file: %w", err)
	}
	defer func() {
		if metaFile != nil {
			if err = metaFile.Close(); err != nil {
				logger.Errorf("failed to close metadata file: %v", err)
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

	return os.MkdirAll( //nolint:gosec // need this permissions
		dir,
		0o777, //revive:disable-line:add-constant // file permissions simple readable as octa-number
	)
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
