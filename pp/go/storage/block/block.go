package block

import (
	"fmt"
	"io"
	"math"
	"unsafe"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

type Chunk struct {
	rc *cppbridge.RecodedChunk
}

func (c *Chunk) MinT() int64 {
	return c.rc.MinT
}

func (c *Chunk) MaxT() int64 {
	return c.rc.MaxT
}

func (c *Chunk) SeriesID() uint32 {
	return c.rc.SeriesId
}

func (*Chunk) Encoding() chunkenc.Encoding {
	return chunkenc.EncXOR
}

func (c *Chunk) SampleCount() uint8 {
	return c.rc.SamplesCount
}

func (c *Chunk) Bytes() []byte {
	return c.rc.ChunkData
}

type ChunkIterator struct {
	r  *cppbridge.ChunkRecoder
	rc *cppbridge.RecodedChunk
}

// NewChunkIterator init new [ChunkIterator].
func NewChunkIterator(
	lss *cppbridge.LabelSetStorage,
	lsIdBatchSize uint32,
	ds *cppbridge.HeadDataStorage,
	minT, maxT int64,
) ChunkIterator {
	return ChunkIterator{
		r: cppbridge.NewChunkRecoder(lss, lsIdBatchSize, ds, cppbridge.TimeInterval{MinT: minT, MaxT: maxT}),
	}
}

func (i *ChunkIterator) Next() bool {
	if i.rc != nil && !i.rc.HasMoreData {
		return false
	}

	rc := i.r.RecodeNextChunk()
	i.rc = &rc
	return rc.SeriesId != math.MaxUint32
}

func (i *ChunkIterator) NextBatch() bool {
	i.rc.HasMoreData = i.r.NextBatch()
	return i.rc.HasMoreData
}

func (i *ChunkIterator) At() Chunk {
	return Chunk{rc: i.rc}
}

//
// IndexWriter
//

type IndexWriter struct {
	cppIndexWriter  *cppbridge.IndexWriter
	isPrefixWritten bool
}

// NewIndexWriter init new [IndexWriter].
func NewIndexWriter(lss *cppbridge.LabelSetStorage) IndexWriter {
	return IndexWriter{cppIndexWriter: cppbridge.NewIndexWriter(lss)}
}

func (iw *IndexWriter) WriteRestTo(w io.Writer) (n int64, err error) {
	bytesWritten, err := w.Write(iw.cppIndexWriter.WriteLabelIndices())
	n += int64(bytesWritten)
	if err != nil {
		return n, fmt.Errorf("failed to write label indicies: %w", err)
	}

	for {
		data, hasMoreData := iw.cppIndexWriter.WriteNextPostingsBatch(1 << 20)
		bytesWritten, err = w.Write(data)
		if err != nil {
			return n, fmt.Errorf("failed to write postings: %w", err)
		}
		n += int64(bytesWritten)
		if !hasMoreData {
			break
		}
	}

	bytesWritten, err = w.Write(iw.cppIndexWriter.WriteLabelIndicesTable())
	if err != nil {
		return n, fmt.Errorf("failed to write label indicies table: %w", err)
	}
	n += int64(bytesWritten)

	bytesWritten, err = w.Write(iw.cppIndexWriter.WritePostingsTableOffsets())
	if err != nil {
		return n, fmt.Errorf("failed to write posting table offsets: %w", err)
	}
	n += int64(bytesWritten)

	bytesWritten, err = w.Write(iw.cppIndexWriter.WriteTableOfContents())
	if err != nil {
		return n, fmt.Errorf("failed to write table of content: %w", err)
	}
	n += int64(bytesWritten)

	return n, nil
}

// WriteSeriesTo write series(id and chunks) to [io.Writer].
func (iw *IndexWriter) WriteSeriesTo(id uint32, chunks []ChunkMetadata, w io.Writer) (n int64, err error) {
	if !iw.isPrefixWritten {
		var bytesWritten int
		bytesWritten, err = w.Write(iw.cppIndexWriter.WriteHeader())
		n += int64(bytesWritten)
		if err != nil {
			return n, fmt.Errorf("failed to write header: %w", err)
		}

		bytesWritten, err = w.Write(iw.cppIndexWriter.WriteSymbols())
		n += int64(bytesWritten)
		if err != nil {
			return n, fmt.Errorf("failed to write symbols: %w", err)
		}
		iw.isPrefixWritten = true
	}

	bytesWritten, err := w.Write(iw.cppIndexWriter.WriteSeries(
		id,
		*(*[]cppbridge.ChunkMetadata)(unsafe.Pointer(&chunks)), // #nosec G103 // it's meant to be that way
	))
	n += int64(bytesWritten)
	if err != nil {
		return n, fmt.Errorf("failed to write series: %w", err)
	}

	return n, nil
}
