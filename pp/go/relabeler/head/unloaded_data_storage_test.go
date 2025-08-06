package head

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

type BufferReaderAtWriterCloser struct {
	buffer []byte
}

func (s *BufferReaderAtWriterCloser) ReadAt(p []byte, off int64) (n int, err error) {
	return bytes.NewReader(s.buffer).ReadAt(p, off)
}

func (s *BufferReaderAtWriterCloser) Write(p []byte) (n int, err error) {
	s.buffer = append(s.buffer, p...)
	return len(p), nil
}

func (s *BufferReaderAtWriterCloser) Close() error {
	return nil
}

type UnloadedDataStorageSuite struct {
	suite.Suite
	storageBuffer *BufferReaderAtWriterCloser
	storage       *UnloadedDataStorage
}

func TestUnloadedDataStorageSuite(t *testing.T) {
	suite.Run(t, new(UnloadedDataStorageSuite))
}

func (s *UnloadedDataStorageSuite) SetupTest() {
	s.storageBuffer = &BufferReaderAtWriterCloser{}
	s.storage = NewUnloadedDataStorage(s.storageBuffer)
}

func (s *UnloadedDataStorageSuite) readSnapshots() ([]string, error) {
	var snapshots []string
	return snapshots, s.storage.ForEachSnapshot(func(snapshot []byte, isLast bool) {
		snapshots = append(snapshots, string(snapshot))
	})
}

func (s *UnloadedDataStorageSuite) TestReadEmptySnapshots() {
	// Arrange

	// Act
	snapshots, err := s.readSnapshots()

	// Assert
	s.Equal([]string(nil), snapshots)
	s.Equal(nil, err)
}

func (s *UnloadedDataStorageSuite) TestReadOneSnapshot() {
	// Arrange
	_ = s.storage.Write([]byte("12345"))

	// Act
	snapshots, err := s.readSnapshots()

	// Assert
	s.Equal([]string{"12345"}, snapshots)
	s.Equal(nil, err)
}

func (s *UnloadedDataStorageSuite) TestReadMultipleSnapshots() {
	// Arrange
	_ = s.storage.Write([]byte("123"))
	_ = s.storage.Write([]byte("45678"))
	_ = s.storage.Write([]byte("90"))

	// Act
	snapshots, err := s.readSnapshots()

	// Assert
	s.Equal([]string{"123", "45678", "90"}, snapshots)
	s.Equal(nil, err)
}

func (s *UnloadedDataStorageSuite) TestReadEof() {
	// Arrange
	_ = s.storage.Write([]byte("123"))
	s.storageBuffer.buffer = s.storageBuffer.buffer[:len(s.storageBuffer.buffer)-1]

	// Act
	snapshots, err := s.readSnapshots()

	// Assert
	s.Equal([]string(nil), snapshots)
	s.Equal(fmt.Errorf("EOF"), err)
}

func (s *UnloadedDataStorageSuite) TestInvalidVersion() {
	// Arrange
	var invalidVersion byte = UnloadedDataStorageVersion + 1
	_, _ = s.storageBuffer.Write([]byte{invalidVersion})

	// Act
	snapshots, err := s.readSnapshots()

	// Assert
	s.Equal([]string(nil), snapshots)
	s.Equal(fmt.Errorf("UnloadedDataStorage invalid version %d", invalidVersion), err)
}

func (s *UnloadedDataStorageSuite) TestReadInvalidSnapshot() {
	// Arrange
	_ = s.storage.Write([]byte("123"))
	_ = s.storage.Write([]byte("45678"))
	s.storageBuffer.buffer[4] = 0x00

	// Act
	snapshots, err := s.readSnapshots()

	// Assert
	s.Equal([]string{"123"}, snapshots)
	s.Equal(fmt.Errorf("invalid snapshot at index 1"), err)
}
