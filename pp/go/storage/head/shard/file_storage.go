package shard

import "os"

type FileStorage struct {
	fileName string
	file     *os.File
}

func NewFileStorage(fileName string) *FileStorage {
	return &FileStorage{fileName: fileName}
}

func (q *FileStorage) ReadAt(p []byte, off int64) (n int, err error) {
	return q.file.ReadAt(p, off)
}

func (q *FileStorage) Open(flags int) (err error) {
	if q.file == nil {
		q.file, err = os.OpenFile(q.fileName, flags, 0666)
	}

	return
}

func (q *FileStorage) Write(p []byte) (n int, err error) {
	return q.file.Write(p)
}

func (q *FileStorage) Close() error {
	if q.file != nil {
		return q.file.Close()
	}

	return nil
}

func (q *FileStorage) Read(p []byte) (n int, err error) {
	return q.file.Read(p)
}

func (q *FileStorage) Seek(offset int64, whence int) (int64, error) {
	return q.file.Seek(offset, whence)
}

func (q *FileStorage) Sync() error {
	return q.file.Sync()
}

func (q *FileStorage) Truncate(size int64) error {
	return q.file.Truncate(size)
}

func (q *FileStorage) IsEmpty() bool {
	if q.file != nil {
		if info, err := q.file.Stat(); err == nil {
			return info.Size() == 0
		}
	}

	return true
}
