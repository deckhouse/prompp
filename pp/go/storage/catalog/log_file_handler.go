package catalog

import (
	"errors"
	"fmt"
	"io"
	"os"
)

//
// FileHandler
//

// FileHandler handler for work with [os.File].
type FileHandler struct {
	file        *os.File
	size        int
	readOffset  int64
	writeOffset int64
}

// NewFileHandler init new [FileHandler].
func NewFileHandler(filePath string) (*FileHandler, error) {
	return NewFileHandlerWithOpts(filePath, os.O_CREATE|os.O_RDWR, logFilePerm)
}

// NewFileHandlerWithOpts init new [FileHandler] with opts.
func NewFileHandlerWithOpts(filePath string, flag int, perm os.FileMode) (*FileHandler, error) {
	file, err := os.OpenFile(filePath, flag, perm) //#nosec G304 // it's meant to be that way
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, file.Close())
		}
	}()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("read file info: %w", err)
	}

	return &FileHandler{
		file:        file,
		size:        int(fileInfo.Size()),
		writeOffset: fileInfo.Size(),
	}, nil
}

// Close closes the [os.File], rendering it unusable for I/O.
func (fh *FileHandler) Close() error {
	return fh.file.Close()
}

// FileName returns the current name of the file.
func (fh *FileHandler) FileName() string {
	return fh.file.Name()
}

// Read reads len(b) bytes from the [os.File].
func (fh *FileHandler) Read(p []byte) (n int, err error) {
	n, err = fh.file.ReadAt(p, fh.readOffset)
	if err != nil {
		if !errors.Is(err, io.EOF) {
			return 0, fmt.Errorf("read file: %w", err)
		}
	}
	fh.readOffset += int64(n)
	return n, err
}

// SetReadOffset set offset for read file.
func (fh *FileHandler) SetReadOffset(offset int64) {
	fh.readOffset = offset
}

// Size returns current size of file.
func (fh *FileHandler) Size() int {
	return fh.size
}

// Sync commits the current contents of the file to stable storage.
func (fh *FileHandler) Sync() error {
	return fh.file.Sync()
}

// Write writes len(b) bytes to the [os.File].
func (fh *FileHandler) Write(p []byte) (n int, err error) {
	n, err = fh.file.WriteAt(p, fh.writeOffset)
	if err != nil {
		return 0, fmt.Errorf("write file: %w", err)
	}

	if err = fh.file.Sync(); err != nil {
		return 0, fmt.Errorf("sync file: %w", err)
	}

	fh.size += n
	fh.writeOffset += int64(n)
	return n, nil
}
