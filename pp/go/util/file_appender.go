package util

import (
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// FileAppender is a file wrapper for long opened file which appends data sequentially.
type FileAppender struct {
	f               *os.File
	synced, current int64
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
		f:       f,
		synced:  n,
		current: n,
	}, nil
}

// Write writes to file.
func (fa *FileAppender) Write(data []byte) (int, error) {
	n, err := fa.f.Write(data)
	fa.current += int64(n)
	return n, err
}

// Sync syncs file and gives OS cache a hint that data before current position won't be needed anymore.
func (fa *FileAppender) Sync() error {
	if err := fa.f.Sync(); err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	if err := unix.Fadvise(int(fa.f.Fd()), fa.synced, fa.current-fa.synced, unix.FADV_DONTNEED); err != nil {
		return fmt.Errorf("fadvise: %w", err)
	}
	fa.synced = fa.current
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
