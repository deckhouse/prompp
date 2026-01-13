package block

import (
	"fmt"
	"io"
	"math"
	"unsafe"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// Chunk represents a recoded chunk.
type Chunk struct {
	rc *cppbridge.RecodedChunk
}

// MinT returns the minimum timestamp of the chunk.
func (c *Chunk) MinT() int64 {
	return c.rc.MinT
}

// MaxT returns the maximum timestamp of the chunk.
func (c *Chunk) MaxT() int64 {
	return c.rc.MaxT
}

// SeriesID returns the series ID of the chunk.
func (c *Chunk) SeriesID() uint32 {
	return c.rc.SeriesId
}

// Encoding returns is the identifier of the encoding of the chunk.
func (*Chunk) Encoding() chunkenc.Encoding {
	return chunkenc.EncXOR
}

// SampleCount returns the number of samples in the chunk.
func (c *Chunk) SampleCount() uint8 {
	return c.rc.SamplesCount
}

// Bytes returns the bytes blob of the chunk data.
func (c *Chunk) Bytes() []byte {
	return c.rc.ChunkData
}

// ChunkIterator represents a chunk iterator, it is used to iterate over the chunks.
type ChunkIterator struct {
	r           *cppbridge.ChunkRecoder
	hasMoreData bool
}

// NewChunkIterator init new [ChunkIterator].
func NewChunkIterator(
	lss *cppbridge.LabelSetStorage,
	lsIdBatchSize uint32,
	ds *cppbridge.DataStorage,
	minT, maxT, downsamplingMs int64,
) ChunkIterator {
	return ChunkIterator{
		r: cppbridge.NewChunkRecoder(
			lss,
			lsIdBatchSize,
			ds,
			cppbridge.TimeInterval{MinT: minT, MaxT: maxT},
			downsamplingMs,
		),
		hasMoreData: true,
	}
}

// RangeBatch iterates over all chunks available in batch, calling fn for each chunk.
func (i *ChunkIterator) RangeBatch(fn func(Chunk) bool) {
	if !i.hasMoreData {
		return
	}
	rc := i.r.RecodeNextChunk()
	for rc.SeriesId != math.MaxUint32 {
		if !fn(Chunk{rc: &rc}) {
			return
		}
		if !rc.HasMoreData {
			return
		}
		rc = i.r.RecodeNextChunk()
	}
}

// NextBatch advances the iterator by one batch, if if there is more data.
func (i *ChunkIterator) NextBatch() bool {
	i.hasMoreData = i.r.NextBatch()
	return i.hasMoreData
}

//
// IndexWriter
//

// IndexWriter represents a index writer, it is used to write the index.
type IndexWriter struct {
	cppIndexWriter  *cppbridge.IndexWriter
	isPrefixWritten bool
}

// NewIndexWriter init new [IndexWriter].
func NewIndexWriter(lss *cppbridge.LabelSetStorage) IndexWriter {
	return IndexWriter{cppIndexWriter: cppbridge.NewIndexWriter(lss)}
}

// WriteRestTo writes the rest of the index to the writer.
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

// WriteSeriesTo writes series(id and chunks) to [io.Writer].
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

// isEmpty returns true if [IndexWriter] contains no samples, an empty block.
func (iw *IndexWriter) isEmpty() bool {
	return !iw.isPrefixWritten
}
