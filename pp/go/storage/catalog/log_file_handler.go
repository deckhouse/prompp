package catalog

import (
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
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
	// fixedSize marks a preallocated fixed-size file (the log file version 3): writes only
	// overwrite already allocated bytes and never change the accounted size.
	fixedSize bool
	// refuseGrowth makes a positioned write whose end offset exceeds the accounted size fail
	// with ENOSPC instead of touching the file; a test seam for proving that fixed-size v3
	// writes never need disk space, false in production.
	refuseGrowth bool
}

// NewFileHandler init new [FileHandler].
func NewFileHandler(filePath string) (*FileHandler, error) {
	return NewFileHandlerWithOpts(filePath, os.O_CREATE|os.O_RDWR, logFilePerm)
}

// NewFileHandlerWithOpts init new [FileHandler] with opts.
func NewFileHandlerWithOpts(filePath string, flag int, perm os.FileMode) (*FileHandler, error) {
	file, err := os.OpenFile(filePath, flag, perm) //nolint:gosec // it's meant to be that way
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

// SetWriteOffset set offset for write file.
func (fh *FileHandler) SetWriteOffset(offset int64) {
	fh.writeOffset = offset
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

// WriteFixed writes len(b) bytes at the current write offset of a fixed-size file: the write only
// overwrites already allocated bytes, so neither the accounted size nor the write offset change.
// The file is synced before returning.
func (fh *FileHandler) WriteFixed(p []byte) error {
	if !fh.fixedSize {
		return fmt.Errorf("write fixed: %s is not a fixed-size file", fh.file.Name())
	}

	if _, err := fh.writeAt(p, fh.writeOffset); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	if err := fh.Sync(); err != nil {
		return fmt.Errorf("sync file: %w", err)
	}

	return nil
}

// writeAt writes p at off through the growth-refusal seam: with [FileHandler.refuseGrowth]
// armed, a write whose end offset exceeds the accounted size fails with ENOSPC without
// touching the file, while an overwrite inside [0, size) goes through to the file.
func (fh *FileHandler) writeAt(p []byte, off int64) (int, error) {
	if fh.refuseGrowth && off+int64(len(p)) > int64(fh.size) {
		return 0, fmt.Errorf("write %d bytes at %d exceeds size %d: %w", len(p), off, fh.size, syscall.ENOSPC)
	}

	return fh.file.WriteAt(p, off)
}
