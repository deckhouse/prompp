package util

import (
	"os"

	"golang.org/x/sys/unix"
)

type FileReader struct {
	f       *os.File
	current int64
}

func OpenFileReader(path string) (*FileReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &FileReader{f: f}, nil
}

func (fr *FileReader) Read(p []byte) (int, error) {
	n, err := fr.f.Read(p)
	if err := unix.Fadvise(int(fr.f.Fd()), fr.current, int64(n), unix.FADV_NOREUSE); err == nil {
		fr.current += int64(n)
	}
	return n, err
}

func (fr *FileReader) Close() error {
	return fr.f.Close()
}
