package shard

import "os"

// FileStorage wrapper over [os.File] for convenient operation.
type FileStorage struct {
	fileName string
	file     *os.File
}

// NewFileStorage init new [FileStorage].
func NewFileStorage(fileName string) *FileStorage {
	return &FileStorage{fileName: fileName}
}

// Close closes the [File], rendering it unusable for I/O. On files that support [File.SetDeadline],
// any pending I/O operations will be canceled and return immediately with an [ErrClosed] error.
// Close will return an error if it has already been called.
func (q *FileStorage) Close() error {
	if q.file != nil {
		return q.file.Close()
	}

	return nil
}

// IsEmpty returns true if file is empty.
func (q *FileStorage) IsEmpty() bool {
	if q.file != nil {
		if info, err := q.file.Stat(); err == nil {
			return info.Size() == 0
		}
	}

	return true
}

// Open open file for [FileStorage] with flags.
func (q *FileStorage) Open(flags int) (err error) {
	if q.file == nil {
		q.file, err = os.OpenFile( //nolint:gosec // need this permissions
			q.fileName,
			flags,
			0o666, //revive:disable-line:add-constant // file permissions simple readable as octa-number
		)
	}

	return err
}

// Read reads up to len(b) bytes from the File and stores them in b.
// It returns the number of bytes read and any error encountered. At end of file, Read returns 0, io.EOF.
func (q *FileStorage) Read(p []byte) (n int, err error) {
	return q.file.Read(p)
}

// ReadAt reads len(b) bytes from the File starting at byte offset off.
// It returns the number of bytes read and the error, if any.
// ReadAt always returns a non-nil error when n < len(b). At end of file, that error is io.EOF.
func (q *FileStorage) ReadAt(p []byte, off int64) (n int, err error) {
	return q.file.ReadAt(p, off)
}

// Seek sets the offset for the next Read or Write on file to offset,
// interpreted according to whence: 0 means relative to the origin of the file,
// 1 means relative to the current offset, and 2 means relative to the end.
// It returns the new offset and an error, if any.
// The behavior of Seek on a file opened with [O_APPEND] is not specified.
func (q *FileStorage) Seek(offset int64, whence int) (int64, error) {
	return q.file.Seek(offset, whence)
}

// Sync commits the current contents of the file to stable storage.
// Typically, this means flushing the file system's in-memory copy of recently written data to disk.
func (q *FileStorage) Sync() error {
	return q.file.Sync()
}

// Truncate changes the size of the file. It does not change the I/O offset.
// If there is an error, it will be of type [*PathError].
func (q *FileStorage) Truncate(size int64) error {
	return q.file.Truncate(size)
}

// Write writes len(b) bytes from b to the File. It returns the number of bytes written and an error, if any.
// Write returns a non-nil error when n != len(b).
func (q *FileStorage) Write(p []byte) (n int, err error) {
	return q.file.Write(p)
}
