package block

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"

	"github.com/prometheus/prometheus/tsdb/fileutil"
)

// indexWriteBufferSize is the size of the buffered writer used for index files.
const indexWriteBufferSize = 1 << 22

// FileWriter a buffered file writer.
type FileWriter struct {
	file        *os.File
	writeBuffer *bufio.Writer
}

// NewFileWriter init new [FileWriter].
//
// The write buffer is allocated lazily on the first [FileWriter.Write] call, so
// blocks that end up empty (no series written) do not hold a multi-megabyte
// buffer. This matters because a single shard is split into one writer per
// block-duration quant and all of them are kept open simultaneously.
func NewFileWriter(fileName string) (*FileWriter, error) {
	dir := filepath.Dir(fileName)
	df, err := fileutil.OpenDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to open parent dir {%s}: %w", dir, err)
	}
	defer func() { _ = df.Close() }()

	if err = os.RemoveAll(fileName); err != nil {
		return nil, fmt.Errorf("failed to cleanup {%s}: %w", fileName, err)
	}

	indexFile, err := os.OpenFile( //nolint:gosec // need this permissions
		fileName,
		os.O_CREATE|os.O_WRONLY,
		0o666, //revive:disable-line:add-constant // file permissions simple readable as octa-number
	)
	if err != nil {
		return nil, fmt.Errorf(" failed to open file {%s}: %w", fileName, err)
	}

	return &FileWriter{
		file: indexFile,
	}, nil
}

// Close flush buffer to file and sync and closes file.
func (w *FileWriter) Close() error {
	if w.writeBuffer != nil {
		if err := w.writeBuffer.Flush(); err != nil {
			return fmt.Errorf("failed to flush write buffer: %w", err)
		}
	}

	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("failed to sync index file: %w", err)
	}

	return w.file.Close()
}

// Write writes the contents of p into the buffer.
func (w *FileWriter) Write(p []byte) (n int, err error) {
	if w.writeBuffer == nil {
		w.writeBuffer = bufio.NewWriterSize(w.file, indexWriteBufferSize)
	}

	return w.writeBuffer.Write(p)
}
