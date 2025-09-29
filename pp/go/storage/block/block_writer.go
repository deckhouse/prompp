package block

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/oklog/ulid"

	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/fileutil"

	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/util"
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

func (writer *blockWriter) close() error {
	return util.CloseAll(writer.chunkWriter, writer.indexFileWriter)
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
	return nil
}

type blockWriters []blockWriter

// append appends a writer to the block writers.

//nolint:gocritic // hugeParam // we accumulate the writers
func (bw *blockWriters) append(writer blockWriter) {
	*bw = append(*bw, writer)
}

// close closes the block writers.
func (bw *blockWriters) close() {
	for i := range *bw {
		_ = (*bw)[i].close()
	}
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

// writeIndexAndMoveTmpDirToDir writes the index and moves the temporary directory to the directory.
func (bw *blockWriters) writeIndexAndMoveTmpDirToDir() ([]WrittenBlock, error) {
	writtenBlocks := make([]WrittenBlock, 0, len(*bw))
	for i := range *bw {
		if (*bw)[i].isEmpty() {
			_ = (*bw)[i].close()
			if err := os.RemoveAll((*bw)[i].Dir); err != nil {
				logger.Warnf("failed remove empty block: %s", (*bw)[i].Dir)
			}

			continue
		}

		if err := (*bw)[i].writeIndex(); err != nil {
			return nil, err
		}

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

<<<<<<<< HEAD:pp/go/storage/block/block_writer.go
========
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

	writer.Meta.MaxTime++
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

type blockWriters struct {
	writers []blockWriter
}

func (bw *blockWriters) append(writer blockWriter) {
	bw.writers = append(bw.writers, writer)
}

func (bw *blockWriters) close() {
	for i := range bw.writers {
		_ = bw.writers[i].Close()
	}
}

func (bw *blockWriters) recodeAndWriteChunksBatch() error {
	for i := range bw.writers {
		if err := bw.writers[i].RecodeAndWriteChunksBatch(); err != nil {
			return err
		}
	}

	return nil
}

func (bw *blockWriters) writeRestOfRecodedChunks() error {
	for i := range bw.writers {
		if err := bw.writers[i].WriteRestOfRecodedChunks(); err != nil {
			return err
		}
	}

	return nil
}

func (bw *blockWriters) writeIndexAndMoveTmpDirToDir() ([]WrittenBlock, error) {
	writtenBlocks := make([]WrittenBlock, 0, len(bw.writers))
	for i := range bw.writers {
		if err := bw.writers[i].writeIndex(); err != nil {
			return nil, err
		}

		if err := bw.writers[i].MoveTmpDirToDir(); err != nil {
			return nil, err
		}

		writtenBlocks = append(writtenBlocks, bw.writers[i].WrittenBlock)
	}

	return writtenBlocks, nil
}

type Writer struct {
	dataDir                  string
	maxBlockChunkSegmentSize int64
	blockDurationMs          int64
	blockWriteDuration       *prometheus.GaugeVec
}

func NewWriter(
	dataDir string,
	maxBlockChunkSegmentSize int64,
	blockDuration time.Duration,
	registerer prometheus.Registerer,
) *Writer {
	factory := util.NewUnconflictRegisterer(registerer)
	return &Writer{
		dataDir:                  dataDir,
		maxBlockChunkSegmentSize: maxBlockChunkSegmentSize,
		blockDurationMs:          blockDuration.Milliseconds(),
		blockWriteDuration: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prompp_block_write_duration",
			Help: "Block write duration in milliseconds.",
		}, []string{"block_id"}),
	}
}

func (w *Writer) Write(shard relabeler.Shard) ([]WrittenBlock, error) {
	shard.LSSRLock()
	defer shard.LSSRUnlock()

	writers, err := w.createWriters(shard)
	if err != nil {
		return nil, err
	}

	defer func() {
		writers.close()
	}()

	if err = w.recodeAndWriteChunks(shard, writers); err != nil {
		return nil, err
	}

	return writers.writeIndexAndMoveTmpDirToDir()
}

func (w *Writer) createWriters(shard relabeler.Shard) (blockWriters, error) {
	var writers blockWriters

	shard.DataStorageRLock()
	timeInterval := shard.DataStorage().TimeInterval(false)
	shard.DataStorageRUnlock()

	quantStart := (timeInterval.MinT / w.blockDurationMs) * w.blockDurationMs
	for ; quantStart <= timeInterval.MaxT; quantStart += w.blockDurationMs {
		minT, maxT := quantStart, quantStart+w.blockDurationMs-1
		if minT < timeInterval.MinT {
			minT = timeInterval.MinT
		}
		if maxT > timeInterval.MaxT {
			maxT = timeInterval.MaxT
		}

		shard.DataStorageRLock()
		chunkIterator := NewChunkIterator(shard.LSS().Raw(), LsIdBatchSize, shard.DataStorage().Raw(), minT, maxT)
		shard.DataStorageRUnlock()

		if writer, err := newBlockWriter(w.dataDir, w.maxBlockChunkSegmentSize, NewIndexWriter(shard.LSS().Raw()), chunkIterator); err == nil {
			writers.append(writer)
		} else {
			writers.close()
			return blockWriters{}, err
		}
	}

	return writers, nil
}

func (w *Writer) recodeAndWriteChunks(shard relabeler.Shard, writers blockWriters) error {
	shard.DataStorageRLock()
	loader := shard.DataStorage().CreateRevertableLoader(shard.LSS().Raw(), LsIdBatchSize)
	shard.DataStorageRUnlock()

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
		shard.DataStorageLock()
		hasMoreData, err := loadData()
		shard.DataStorageUnlock()

		if !hasMoreData {
			break
		}

		if err != nil {
			return err
		}

		shard.DataStorageRLock()
		err = writers.recodeAndWriteChunksBatch()
		shard.DataStorageRUnlock()

		if err != nil {
			return err
		}
	}

	return writers.writeRestOfRecodedChunks()
}

func closeAll(closers ...io.Closer) error {
	var errs error
	for _, closer := range closers {
		errs = errors.Join(errs, closer.Close())
	}
	return errs
}

>>>>>>>> 820ec1690 (Head keeper (#154)):pp/go/relabeler/block/writer.go
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
