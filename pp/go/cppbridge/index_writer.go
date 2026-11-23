package cppbridge

import (
	"io"
	"runtime"
	"unsafe"
)

type ChunkMetadata struct {
	MinTimestamp int64
	MaxTimestamp int64
	Reference    uint64
}

// indexWriterOutput is a read-only view over the index writer's C++-owned output, the Go side of
// the "C++ writes -> Go reads" contract. Every write_* call fills the writer's internal
// PromPP::Primitives::Go::Slice<char> buffer in place and sets the has-more-postings flag, both at
// stable addresses handed out once by the constructor; Go reads them here without an extra cgo
// call. Go::Slice mirrors the Go slice header layout (data, len, cap) precisely for exactly this
// use, so the buffer pointer is a *[]byte and the flag a *uint8 — no per-read reinterpretation.
type indexWriterOutput struct {
	buffer          *[]byte
	hasMorePostings *uint8
}

func newIndexWriterOutput(buffer, hasMorePostings unsafe.Pointer) indexWriterOutput {
	return indexWriterOutput{
		buffer:          (*[]byte)(buffer),
		hasMorePostings: (*uint8)(hasMorePostings),
	}
}

// bytes returns the buffer filled by the last write_* call.
func (o indexWriterOutput) bytes() []byte {
	return *o.buffer
}

// hasMore reports whether the last write_postings batch left more postings to write.
func (o indexWriterOutput) hasMore() bool {
	return *o.hasMorePostings != 0
}

// IndexWriter serializes index sections to an [io.Writer] using a reusable C++ buffer.
// Its write methods must not be used concurrently or reentrantly.
type IndexWriter struct {
	writer unsafe.Pointer
	output indexWriterOutput
	lss    *LabelSetStorage
}

func NewIndexWriter(lss *LabelSetStorage) *IndexWriter {
	w, buffer, hasMorePostings := indexWriterCtor(lss.Pointer())
	writer := &IndexWriter{
		writer: w,
		output: newIndexWriterOutput(buffer, hasMorePostings),
		lss:    lss,
	}
	runtime.AddCleanup(writer, indexWriterDtor, writer.writer)
	return writer
}

// WriteHeaderTo writes the index header to dst.
func (writer *IndexWriter) WriteHeaderTo(dst io.Writer) (int, error) {
	indexWriterWriteHeader(writer.writer)

	return writer.writeTo(dst)
}

// WriteLabelIndicesTableTo writes the label indices table to dst.
func (writer *IndexWriter) WriteLabelIndicesTableTo(dst io.Writer) (int, error) {
	indexWriterWriteLabelIndicesTable(writer.writer)

	return writer.writeTo(dst)
}

// WriteLabelIndicesTo writes the label indices to dst.
func (writer *IndexWriter) WriteLabelIndicesTo(dst io.Writer) (int, error) {
	indexWriterWriteLabelIndices(writer.writer)

	return writer.writeTo(dst)
}

// WriteNextPostingsBatchTo writes one batch to dst and reports whether more postings remain.
// The size threshold is checked between postings, so a batch can exceed maxBatchSize.
func (writer *IndexWriter) WriteNextPostingsBatchTo(
	maxBatchSize uint32,
	dst io.Writer,
) (n int, hasMore bool, err error) {
	indexWriterWritePostings(writer.writer, maxBatchSize)
	hasMore = writer.output.hasMore()
	n, err = writer.writeTo(dst)

	return n, hasMore, err
}

// WritePostingsTableOffsetsTo writes the postings table offsets to dst.
func (writer *IndexWriter) WritePostingsTableOffsetsTo(dst io.Writer) (int, error) {
	indexWriterWritePostingsTableOffsets(writer.writer)

	return writer.writeTo(dst)
}

// WriteSeriesTo writes the series identified by id and its chunk metadata to dst.
func (writer *IndexWriter) WriteSeriesTo(id uint32, chunks []ChunkMetadata, dst io.Writer) (int, error) {
	indexWriterWriteNextSeriesBatch(writer.writer, id, chunks)

	return writer.writeTo(dst)
}

// WriteSymbolsTo writes the index symbol table to dst.
func (writer *IndexWriter) WriteSymbolsTo(dst io.Writer) (int, error) {
	indexWriterWriteSymbols(writer.writer)

	return writer.writeTo(dst)
}

// WriteTableOfContentsTo writes the index table of contents to dst.
func (writer *IndexWriter) WriteTableOfContentsTo(dst io.Writer) (int, error) {
	indexWriterWriteTableOfContents(writer.writer)

	return writer.writeTo(dst)
}

// writeTo keeps the C++ buffer valid until dst.Write returns.
func (writer *IndexWriter) writeTo(dst io.Writer) (int, error) {
	n, err := dst.Write(writer.output.bytes())
	runtime.KeepAlive(writer)

	return n, err
}
