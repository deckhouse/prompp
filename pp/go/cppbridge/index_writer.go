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
	writer unsafe.Pointer
	buffer unsafe.Pointer
	lss    *LabelSetStorage
}

func NewIndexWriter(lss *LabelSetStorage) *IndexWriter {
	w, buffer := indexWriterCtor(lss.Pointer())
	writer := &IndexWriter{
		writer: w,
		buffer: buffer,
		lss:    lss,
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

func (writer *IndexWriter) WritePostings() []byte {
	indexWriterWritePostings(writer.writer)
	return writer.bytes()
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
