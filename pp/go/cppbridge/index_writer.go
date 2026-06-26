package cppbridge

import (
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
// The returned bytes alias prompp-owned memory and stay valid only until the next write_* call or
// until the writer is finalized, so callers must consume them before then.
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
	runtime.SetFinalizer(writer, func(writer *IndexWriter) {
		indexWriterDtor(writer.writer)
	})
	return writer
}

func (writer *IndexWriter) WriteHeader() []byte {
	indexWriterWriteHeader(writer.writer)
	return writer.output.bytes()
}

func (writer *IndexWriter) WriteSymbols() []byte {
	indexWriterWriteSymbols(writer.writer)
	return writer.output.bytes()
}

func (writer *IndexWriter) WriteSeries(ls_id uint32, chunks_meta []ChunkMetadata) []byte {
	indexWriterWriteNextSeriesBatch(writer.writer, ls_id, chunks_meta)
	return writer.output.bytes()
}

func (writer *IndexWriter) WriteLabelIndices() []byte {
	indexWriterWriteLabelIndices(writer.writer)
	return writer.output.bytes()
}

// WriteNextPostingsBatch writes one postings batch (up to maxBatchSize bytes) into the writer's
// internal buffer and reports whether more batches remain. The returned slice aliases prompp-owned
// memory and is valid only until the next write_* call, so callers must consume it before looping.
func (writer *IndexWriter) WriteNextPostingsBatch(maxBatchSize uint32) ([]byte, bool) {
	indexWriterWritePostings(writer.writer, maxBatchSize)
	return writer.output.bytes(), writer.output.hasMore()
}

func (writer *IndexWriter) WriteLabelIndicesTable() []byte {
	indexWriterWriteLabelIndicesTable(writer.writer)
	return writer.output.bytes()
}

func (writer *IndexWriter) WritePostingsTableOffsets() []byte {
	indexWriterWritePostingsTableOffsets(writer.writer)
	return writer.output.bytes()
}

func (writer *IndexWriter) WriteTableOfContents() []byte {
	indexWriterWriteTableOfContents(writer.writer)
	return writer.output.bytes()
}
