package util

import (
	"os"

	"golang.org/x/sys/unix"
)

// FileReader is a file wrapper for long opened file which reads sequentially.
type FileReader struct {
	f      *os.File
	offset int
}

// OpenFileReader opens file for sequential reading.
//
// It uses default os.Open with all default flags.
func OpenFileReader(path string) (*FileReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &FileReader{f: f}, nil
}

// Read reads from file and gives read data to OS cache a hint that it won't be needed anymore.
func (fr *FileReader) Read(p []byte) (int, error) {
	n, err := fr.f.Read(p)
	fr.offset += n
	aligned := (fr.offset / pageSize) * pageSize
	_ = unix.Fadvise(int(fr.f.Fd()), 0, int64(aligned), unix.FADV_DONTNEED)
	return n, err
}

// Close closes the file.
func (fr *FileReader) Close() error {
	return fr.f.Close()
}
