package util

import (
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

var pageSize = os.Getpagesize()

// FileAppender is a file wrapper for long opened file which appends data sequentially.
type FileAppender struct {
	f      *os.File
	offset int
}

// CreateFileAppender creates or truncates file for sequential writing.
func CreateFileAppender(path string, perm os.FileMode) (*FileAppender, error) {
	//nolint:gosec // It's used only for files controlled by us.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	return &FileAppender{f: f}, nil
}

// OpenFileAppender opens file for sequential appending.
func OpenFileAppender(path string, perm os.FileMode) (*FileAppender, error) {
	//nolint:gosec // It's used only for files controlled by us.
	f, err := os.OpenFile(path, os.O_WRONLY, perm)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	n, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("seek end: %w", err), f.Close())
	}
	return &FileAppender{
		f:      f,
		offset: int(n),
	}, nil
}

// Write writes to file.
func (fa *FileAppender) Write(data []byte) (int, error) {
	n, err := fa.f.Write(data)
	fa.offset += n
	return n, err
}

// Sync syncs file and gives OS cache a hint that data before current position won't be needed anymore.
func (fa *FileAppender) Sync() error {
	if err := fa.f.Sync(); err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	aligned := (fa.offset / pageSize) * pageSize
	_ = unix.Fadvise(int(fa.f.Fd()), 0, int64(aligned), unix.FADV_DONTNEED)
	return nil
}

// Close closes the file.
func (fa *FileAppender) Close() error {
	if err := fa.f.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	return nil
}

// Stat returns file info.
func (fa *FileAppender) Stat() (os.FileInfo, error) {
	return fa.f.Stat()
}
