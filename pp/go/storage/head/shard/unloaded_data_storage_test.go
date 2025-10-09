package shard

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
)

// BufferReaderAtWriterCloser implementation [AppendFile].
type BufferReaderAtWriterCloser struct {
	buffer []byte
}

// IsEmpty implementation [AppendFile].
func (*BufferReaderAtWriterCloser) IsEmpty() bool {
	return true
}

// Open implementation [AppendFile].
func (*BufferReaderAtWriterCloser) Open() error {
	return nil
}

// Read implementation [AppendFile].
func (*BufferReaderAtWriterCloser) Read([]byte) (n int, err error) {
	return 0, nil
}

// Reader implementation [AppendFile].
func (s *BufferReaderAtWriterCloser) Reader() (StorageReader, error) {
	return s, nil
}

// Sync implementation [AppendFile].
func (*BufferReaderAtWriterCloser) Sync() error {
	return nil
}

// ReadAt implementation [AppendFile].
func (s *BufferReaderAtWriterCloser) ReadAt(p []byte, off int64) (n int, err error) {
	return bytes.NewReader(s.buffer).ReadAt(p, off)
}

// Write implementation [AppendFile].
func (s *BufferReaderAtWriterCloser) Write(p []byte) (n int, err error) {
	s.buffer = append(s.buffer, p...)
	return len(p), nil
}

// Close implementation [AppendFile].
func (*BufferReaderAtWriterCloser) Close() error {
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

func (s *UnloadedDataStorageSuite) Write(snapshot []byte) {
	header, _ := s.storage.WriteSnapshot(snapshot)
	s.storage.WriteIndex(header)
}

func (s *UnloadedDataStorageSuite) readSnapshots() ([]string, error) {
	var snapshots []string
	return snapshots, s.storage.ForEachSnapshot(func(snapshot []byte, _ bool) {
		snapshots = append(snapshots, string(snapshot))
	})
}

func (s *UnloadedDataStorageSuite) TestWriteEmptySnapshot() {
	// Arrange

	// Act
	header, err := s.storage.WriteSnapshot(nil)

	// Assert
	s.Require().NoError(err)
	s.Equal(UnloadedDataSnapshotHeader{}, header)
}

func (s *UnloadedDataStorageSuite) TestReadEmptySnapshots() {
	// Arrange
	s.storageBuffer.buffer = []byte{UnloadedDataStorageVersion}

	// Act
	snapshots, err := s.readSnapshots()

	// Assert
	s.Require().NoError(err)
	s.Equal([]string(nil), snapshots)
}

func (s *UnloadedDataStorageSuite) TestReadOneSnapshot() {
	// Arrange
	s.Write([]byte("12345"))

	// Act
	snapshots, err := s.readSnapshots()

	// Assert
	s.Require().NoError(err)
	s.Equal([]string{"12345"}, snapshots)
}

func (s *UnloadedDataStorageSuite) TestReadMultipleSnapshots() {
	// Arrange
	s.Write([]byte("123"))
	s.Write([]byte("45678"))
	s.Write([]byte("90"))

	// Act
	snapshots, err := s.readSnapshots()

	// Assert
	s.Require().NoError(err)
	s.Equal([]string{"123", "45678", "90"}, snapshots)
}

func (s *UnloadedDataStorageSuite) TestReadEof() {
	// Arrange
	s.Write([]byte("123"))
	s.storageBuffer.buffer = s.storageBuffer.buffer[:len(s.storageBuffer.buffer)-1]

	// Act
	snapshots, err := s.readSnapshots()

	// Assert
	s.Require().ErrorIs(err, io.EOF)
	s.Equal([]string(nil), snapshots)
}

func (s *UnloadedDataStorageSuite) TestReadVersionError() {
	// Arrange
	s.Write([]byte("123"))
	s.storageBuffer.buffer = nil

	// Act
	snapshots, err := s.readSnapshots()

	// Assert
	s.Require().Equal(io.EOF, err)
	s.Equal([]string(nil), snapshots)
}

func (s *UnloadedDataStorageSuite) TestInvalidVersion() {
	// Arrange
	s.Write([]byte("123"))
	var invalidVersion byte = UnloadedDataStorageVersion + 1
	s.storageBuffer.buffer = []byte{invalidVersion}

	// Act
	snapshots, err := s.readSnapshots()

	// Assert
	s.Equal([]string(nil), snapshots)
	s.Equal(fmt.Errorf("UnloadedDataStorage invalid version %d", invalidVersion), err)
}

func (s *UnloadedDataStorageSuite) TestReadInvalidSnapshot() {
	// Arrange
	s.Write([]byte("123"))
	s.Write([]byte("45678"))
	s.storageBuffer.buffer[4] = 0x00

	// Act
	snapshots, err := s.readSnapshots()

	// Assert
	s.Equal([]string{"123"}, snapshots)
	s.Equal(fmt.Errorf("invalid snapshot at index 1"), err)
}

type QueriedSeriesStorageSuite struct {
	suite.Suite
	file1   *FileStorage
	file2   *FileStorage
	storage *QueriedSeriesStorage
}

func TestQueriedSeriesStorageWriterSuite(t *testing.T) {
	suite.Run(t, new(QueriedSeriesStorageSuite))
}

func (s *QueriedSeriesStorageSuite) SetupTest() {
	tempDir := s.T().TempDir()
	s.file1 = &FileStorage{fileName: filepath.Join(tempDir, "file1")}
	s.file2 = &FileStorage{fileName: filepath.Join(tempDir, "file2")}
	s.storage = NewQueriedSeriesStorage(s.file1, s.file2)
}

func (s *QueriedSeriesStorageSuite) TearDownTest() {
	s.Require().NoError(s.storage.Close())
}

func (s *QueriedSeriesStorageSuite) writeFile(file *FileStorage, data []byte) {
	s.Require().NoError(file.Open(os.O_RDWR | os.O_CREATE | os.O_TRUNC))
	_, err := file.Write(data)
	s.Require().NoError(err)
}

func (s *QueriedSeriesStorageSuite) readFile(file *FileStorage) []byte {
	_, err := file.Seek(0, io.SeekStart)
	s.Require().NoError(err)

	data, err := io.ReadAll(file)
	s.Require().NoError(err)

	return data
}

func (s *QueriedSeriesStorageSuite) TestOpenErrorOnWrite() {
	// Arrange
	s.file1.fileName = ""

	// Act
	err := s.storage.Write([]byte("12345"), 1234567890)

	// Assert
	s.Require().Error(err)
	s.Nil(s.storage.validStorage)
	s.Equal(s.file2, s.storage.storages[0])
}

func (s *QueriedSeriesStorageSuite) TestWriteInFirstStorage() {
	// Arrange

	// Act
	err := s.storage.Write([]byte("12345"), 1234567890)

	// Assert
	s.Require().NoError(err)
	s.Equal([]byte{
		QueriedSeriesStorageVersion,                    // version
		0xd2, 0x02, 0x96, 0x49, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x4e, 0x78, 0xf9, 0xf3, // crc32
		0x05, 0x00, 0x00, 0x00, // size
		'1', '2', '3', '4', '5', // content
	}, s.readFile(s.file1))
	s.Nil(s.file2.file)
}

func (s *QueriedSeriesStorageSuite) TestWriteInAllStorages() {
	// Arrange

	// Act
	err1 := s.storage.Write([]byte("12345"), 1234567890)
	err2 := s.storage.Write([]byte("67890"), 987654321)

	// Assert
	s.NoError(err1)
	s.NoError(err2)
	s.Equal([]byte{
		QueriedSeriesStorageVersion,                    // version
		0xd2, 0x02, 0x96, 0x49, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x4e, 0x78, 0xf9, 0xf3, // crc32
		0x05, 0x00, 0x00, 0x00, // size
		'1', '2', '3', '4', '5', // content
	}, s.readFile(s.file1))
	s.Equal([]byte{
		QueriedSeriesStorageVersion,                    // version
		0xb1, 0x68, 0xde, 0x3a, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x21, 0x33, 0xf7, 0xb8, // crc32
		0x05, 0x00, 0x00, 0x00, // size
		'6', '7', '8', '9', '0', // content
	}, s.readFile(s.file2))
}

func (s *QueriedSeriesStorageSuite) TestMultipleWriteInFirstStorage() {
	// Arrange

	// Act
	_ = s.storage.Write([]byte("12345"), 1234567890)
	_ = s.storage.Write([]byte("67890"), 987654321)
	_ = s.storage.Write([]byte("67890"), 987654321)
	_ = s.storage.Write([]byte("12345"), 1234567890)

	// Assert
	s.Equal([]byte{
		QueriedSeriesStorageVersion,                    // version
		0xb1, 0x68, 0xde, 0x3a, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x21, 0x33, 0xf7, 0xb8, // crc32
		0x05, 0x00, 0x00, 0x00, // size
		'6', '7', '8', '9', '0', // content
	}, s.readFile(s.file1))
	s.Equal([]byte{
		QueriedSeriesStorageVersion,                    // version
		0xd2, 0x02, 0x96, 0x49, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x4e, 0x78, 0xf9, 0xf3, // crc32
		0x05, 0x00, 0x00, 0x00, // size
		'1', '2', '3', '4', '5', // content
	}, s.readFile(s.file2))
}

func (s *QueriedSeriesStorageSuite) TestOpenErrorInRead() {
	// Arrange
	s.file1.fileName = ""

	// Act
	data, err := s.storage.Read()

	// Assert
	s.Equal([]byte(nil), data)
	s.Error(err)
}

func (s *QueriedSeriesStorageSuite) TestChangeActiveFileOnOpenErrorWithoutValidFile() {
	// Arrange
	s.file1.fileName = ""

	// Act
	writeErr1 := s.storage.Write([]byte("12345"), 1234567890)
	writeErr2 := s.storage.Write([]byte("12345"), 1234567890)

	// Assert
	s.Require().Error(writeErr1)
	s.Require().NoError(writeErr2)
	s.Equal([]byte{
		QueriedSeriesStorageVersion,                    // version
		0xd2, 0x02, 0x96, 0x49, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x4e, 0x78, 0xf9, 0xf3, // crc32
		0x05, 0x00, 0x00, 0x00, // size
		'1', '2', '3', '4', '5', // content
	}, s.readFile(s.file2))
}

func (s *QueriedSeriesStorageSuite) TestNoChangeActiveFileOnOpenErrorWithValidFile() {
	// Arrange
	s.file2.fileName = ""
	s.writeFile(s.file1, []byte{
		QueriedSeriesStorageVersion,                    // version
		0xd2, 0x02, 0x96, 0x49, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x4e, 0x78, 0xf9, 0xf3, // crc32
		0x05, 0x00, 0x00, 0x00,
		'1', '2', '3', '4', '5',
	})

	// Act
	data, readErr := s.storage.Read()
	writeErr1 := s.storage.Write([]byte("67890"), 987654321)
	writeErr2 := s.storage.Write([]byte("67890"), 987654321)

	// Assert
	s.Require().NoError(readErr)
	s.Equal([]byte("12345"), data)
	s.Require().Error(writeErr1)
	s.Require().Error(writeErr2)
}

func (s *QueriedSeriesStorageSuite) TestReadEmptyFiles() {
	// Arrange

	// Act
	data, err := s.storage.Read()

	// Assert
	s.Equal([]byte(nil), data)
	s.Equal(errors.New("no valid queried series storage"), err)
}

func (s *QueriedSeriesStorageSuite) TestInvalidVersionInAllStorages() {
	// Arrange
	s.writeFile(s.file1, []byte{QueriedSeriesStorageVersion + 1})
	s.writeFile(s.file2, []byte{QueriedSeriesStorageVersion + 1})

	// Act
	data, err := s.storage.Read()

	// Assert
	s.Equal([]byte(nil), data)
	s.Equal(errors.New("no valid queried series storage"), err)
}

func (s *QueriedSeriesStorageSuite) TestInvalidHeaderInAllStorages() {
	// Arrange
	invalidHeader := []byte{
		QueriedSeriesStorageVersion,                    // version
		0xd2, 0x02, 0x96, 0x49, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x4e, 0x78, 0xf9, 0xf3, // crc32
		0x05, 0x00, 0x00,
	}
	s.writeFile(s.file1, invalidHeader)
	s.writeFile(s.file2, invalidHeader)

	// Act
	data, err := s.storage.Read()

	// Assert
	s.Equal([]byte(nil), data)
	s.Equal(errors.New("no valid queried series storage"), err)
}

func (s *QueriedSeriesStorageSuite) TestInvalidDataInAllStorages() {
	// Arrange
	invalidData := []byte{
		QueriedSeriesStorageVersion,                    // version
		0xd2, 0x02, 0x96, 0x49, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x4e, 0x78, 0xf9, 0xf3, // crc32
		0x05, 0x00, 0x00, 0x00,
		'1', '2', '3', '4',
	}
	s.writeFile(s.file1, invalidData)
	s.writeFile(s.file2, invalidData)

	// Act
	data, err := s.storage.Read()

	// Assert
	s.Equal([]byte(nil), data)
	s.Equal(errors.New("no valid queried series storage"), err)
}

func (s *QueriedSeriesStorageSuite) TestInvalidCrc32InAllStorages() {
	// Arrange
	invalidCrc32 := []byte{
		QueriedSeriesStorageVersion,                    // version
		0xd2, 0x02, 0x96, 0x49, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x4e, 0x78, 0xf9, 0xf2, // crc32
		0x05, 0x00, 0x00, 0x00,
		'1', '2', '3', '4', '5',
	}
	s.writeFile(s.file1, invalidCrc32)
	s.writeFile(s.file2, invalidCrc32)

	// Act
	data, err := s.storage.Read()

	// Assert
	s.Equal([]byte(nil), data)
	s.Equal(errors.New("no valid queried series storage"), err)
}

func (s *QueriedSeriesStorageSuite) TestReadFromFirstStorageAndChangeActiveStorage() {
	// Arrange
	s.writeFile(s.file1, []byte{
		QueriedSeriesStorageVersion,                    // version
		0xd2, 0x02, 0x96, 0x49, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x4e, 0x78, 0xf9, 0xf3, // crc32
		0x05, 0x00, 0x00, 0x00,
		'1', '2', '3', '4', '5',
	})

	// Act
	data, readErr := s.storage.Read()
	_ = s.storage.Write([]byte("67890"), 987654321)

	// Assert
	s.Require().NoError(readErr)
	s.Equal([]byte("12345"), data)
	s.Equal([]byte{
		QueriedSeriesStorageVersion,                    // version
		0xb1, 0x68, 0xde, 0x3a, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x21, 0x33, 0xf7, 0xb8, // crc32
		0x05, 0x00, 0x00, 0x00, // size
		'6', '7', '8', '9', '0', // content
	}, s.readFile(s.file2))
}

func (s *QueriedSeriesStorageSuite) TestReadFromSecondStorage() {
	// Arrange
	s.writeFile(s.file2, []byte{
		QueriedSeriesStorageVersion,                    // version
		0xd2, 0x02, 0x96, 0x49, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x4e, 0x78, 0xf9, 0xf3, // crc32
		0x05, 0x00, 0x00, 0x00,
		'1', '2', '3', '4', '5',
	})

	// Act
	data, err := s.storage.Read()

	// Assert
	s.Require().NoError(err)
	s.Equal([]byte("12345"), data)
}

func (s *QueriedSeriesStorageSuite) TestReadFromStorageWithMaxTimestamp() {
	// Arrange
	s.writeFile(s.file1, []byte{
		QueriedSeriesStorageVersion,                    // version
		0xb1, 0x68, 0xde, 0x3a, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x21, 0x33, 0xf7, 0xb8, // crc32
		0x05, 0x00, 0x00, 0x00, // size
		'6', '7', '8', '9', '0', // content
	})
	s.writeFile(s.file2, []byte{
		QueriedSeriesStorageVersion,                    // version
		0xd3, 0x02, 0x96, 0x49, 0x00, 0x00, 0x00, 0x00, // timestamp
		0xfd, 0x12, 0xf7, 0xe0, // crc32
		0x04, 0x00, 0x00, 0x00,
		'6', '7', '8', '9',
	})

	// Act
	data, err := s.storage.Read()

	// Assert
	s.Require().NoError(err)
	s.Equal([]byte("6789"), data)
}

func (s *QueriedSeriesStorageSuite) TestReadEmptyContent() {
	// Arrange
	s.writeFile(s.file1, []byte{
		QueriedSeriesStorageVersion,                    // version
		0xb1, 0x68, 0xde, 0x3a, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x41, 0x01, 0x44, 0x30, // crc32
		0x00, 0x00, 0x00, 0x00, // size
	})

	// Act
	data, err := s.storage.Read()

	// Assert
	s.Require().NoError(err)
	s.Equal([]byte{}, data)
}
