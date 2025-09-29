package block

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"

	"github.com/prometheus/prometheus/tsdb/fileutil"
)

// FileWriter a buffered file writer.
type FileWriter struct {
	file        *os.File
	writeBuffer *bufio.Writer
}

// NewFileWriter init new [FileWriter].
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
		os.O_CREATE|os.O_RDWR,
		0o666, //revive:disable-line:add-constant // file permissions simple readable as octa-number
	)
	if err != nil {
		return nil, fmt.Errorf(" failed to open file {%s}: %w", fileName, err)
	}

	return &FileWriter{
		file:        indexFile,
		writeBuffer: bufio.NewWriterSize(indexFile, 1<<22),
	}, nil
}

// Close flush buffer to file and sync and closes file.
func (w *FileWriter) Close() error {
	if err := w.writeBuffer.Flush(); err != nil {
		return fmt.Errorf("failed to flush write buffer: %w", err)
	}

	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("failed to sync index file: %w", err)
	}

	return w.file.Close()
}

// Write writes the contents of p into the buffer.
func (w *FileWriter) Write(p []byte) (n int, err error) {
	return w.writeBuffer.Write(p)
}
