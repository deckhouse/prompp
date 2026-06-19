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

type IndexWriter struct {
	writer          unsafe.Pointer
	buffer          unsafe.Pointer
	hasMorePostings unsafe.Pointer
	lss             *LabelSetStorage
}

func NewIndexWriter(lss *LabelSetStorage) *IndexWriter {
	w, buffer, hasMorePostings := indexWriterCtor(lss.Pointer())
	writer := &IndexWriter{
		writer:          w,
		buffer:          buffer,
		hasMorePostings: hasMorePostings,
		lss:             lss,
	}
	runtime.SetFinalizer(writer, func(writer *IndexWriter) {
		indexWriterDtor(writer.writer)
	})
	return writer
}

// bytes reads the writer's internal output buffer (a C-side []byte header) filled by the last
// write_* call. The slice aliases prompp-owned memory and stays valid only until the next
// write_* call or until the writer is finalized, so callers must consume it before then.
func (writer *IndexWriter) bytes() []byte {
	return *(*[]byte)(writer.buffer)
}

func (writer *IndexWriter) WriteHeader() []byte {
	indexWriterWriteHeader(writer.writer)
	return writer.bytes()
}

func (writer *IndexWriter) WriteSymbols() []byte {
	indexWriterWriteSymbols(writer.writer)
	return writer.bytes()
}

func (writer *IndexWriter) WriteSeries(ls_id uint32, chunks_meta []ChunkMetadata) []byte {
	indexWriterWriteNextSeriesBatch(writer.writer, ls_id, chunks_meta)
	return writer.bytes()
}

func (writer *IndexWriter) WriteLabelIndices() []byte {
	indexWriterWriteLabelIndices(writer.writer)
	return writer.bytes()
}

// WriteNextPostingsBatch writes one postings batch (up to maxBatchSize bytes) into the writer's
// internal buffer and reports whether more batches remain. The returned slice aliases prompp-owned
// memory and is valid only until the next write_* call, so callers must consume it before looping.
func (writer *IndexWriter) WriteNextPostingsBatch(maxBatchSize uint32) ([]byte, bool) {
	indexWriterWritePostings(writer.writer, maxBatchSize)
	return writer.bytes(), *(*uint8)(writer.hasMorePostings) != 0
}

func (writer *IndexWriter) WriteLabelIndicesTable() []byte {
	indexWriterWriteLabelIndicesTable(writer.writer)
	return writer.bytes()
}

func (writer *IndexWriter) WritePostingsTableOffsets() []byte {
	indexWriterWritePostingsTableOffsets(writer.writer)
	return writer.bytes()
}

func (writer *IndexWriter) WriteTableOfContents() []byte {
	indexWriterWriteTableOfContents(writer.writer)
	return writer.bytes()
}
