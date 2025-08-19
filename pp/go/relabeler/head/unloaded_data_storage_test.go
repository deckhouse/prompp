package head

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/prometheus/prometheus/pp/go/relabeler"
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
	s.storage, _ = NewUnloadedDataStorage(s.storageBuffer)
}

func (s *UnloadedDataStorageSuite) Write(snapshot []byte) {
	header, _ := s.storage.WriteSnapshot(snapshot)
	s.storage.WriteIndex(header)
}

func (s *UnloadedDataStorageSuite) readSnapshots() ([]string, error) {
	var snapshots []string
	return snapshots, s.storage.ForEachSnapshot(func(snapshot []byte, isLast bool) {
		snapshots = append(snapshots, string(snapshot))
	})
}

func (s *UnloadedDataStorageSuite) TestWriteEmptySnapshot() {
	// Arrange

	// Act
	header, err := s.storage.WriteSnapshot(nil)

	// Assert
	s.Require().NoError(err)
	s.Equal(relabeler.UnloadedDataSnapshotHeader{}, header)
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
	s.Write([]byte("12345"))

	// Act
	snapshots, err := s.readSnapshots()

	// Assert
	s.Equal([]string{"12345"}, snapshots)
	s.Equal(nil, err)
}

func (s *UnloadedDataStorageSuite) TestReadMultipleSnapshots() {
	// Arrange
	s.Write([]byte("123"))
	s.Write([]byte("45678"))
	s.Write([]byte("90"))

	// Act
	snapshots, err := s.readSnapshots()

	// Assert
	s.Equal([]string{"123", "45678", "90"}, snapshots)
	s.Equal(nil, err)
}

func (s *UnloadedDataStorageSuite) TestReadEof() {
	// Arrange
	s.Write([]byte("123"))
	s.storageBuffer.buffer = s.storageBuffer.buffer[:len(s.storageBuffer.buffer)-1]

	// Act
	snapshots, err := s.readSnapshots()

	// Assert
	s.Equal([]string(nil), snapshots)
	s.Equal(fmt.Errorf("EOF"), err)
}

func (s *UnloadedDataStorageSuite) TestReadVersionError() {
	// Arrange
	s.storageBuffer.buffer = nil

	// Act
	snapshots, err := s.readSnapshots()

	// Assert
	s.Equal([]string(nil), snapshots)
	s.Equal(io.EOF, err)
}

func (s *UnloadedDataStorageSuite) TestInvalidVersion() {
	// Arrange
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
	file1   *os.File
	file2   *os.File
	storage *QueriedSeriesStorage
}

func TestQueriedSeriesStorageWriterSuite(t *testing.T) {
	suite.Run(t, new(QueriedSeriesStorageSuite))
}

func (s *QueriedSeriesStorageSuite) SetupTest() {
	var err error
	s.file1, s.file2, err = openQueriedSeriesStorageFiles(s.T().TempDir(), 0)
	s.Require().NoError(err)

	s.storage = NewQueriedSeriesStorage(s.file1, s.file2)
}

func (s *QueriedSeriesStorageSuite) TearDownTest() {
	s.Require().NoError(s.file1.Close())
	s.Require().NoError(s.file2.Close())
}

func (s *QueriedSeriesStorageSuite) readFile(file *os.File) []byte {
	_, err := file.Seek(0, io.SeekStart)
	s.Require().NoError(err)

	data, err := io.ReadAll(file)
	s.Require().NoError(err)

	return data
}

func (s *QueriedSeriesStorageSuite) TestWriteInFirstStorage() {
	// Arrange

	// Act
	err := s.storage.Write([]byte("12345"), 1234567890)

	// Assert
	s.NoError(err)
	s.Equal([]byte{
		QueriedSeriesStorageVersion,                    // version
		0xd2, 0x02, 0x96, 0x49, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x4e, 0x78, 0xf9, 0xf3, //crc32
		0x05, 0x00, 0x00, 0x00, // size
		'1', '2', '3', '4', '5', // content
	}, s.readFile(s.file1))
	s.Equal([]byte{}, s.readFile(s.file2))
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
		0x4e, 0x78, 0xf9, 0xf3, //crc32
		0x05, 0x00, 0x00, 0x00, // size
		'1', '2', '3', '4', '5', // content
	}, s.readFile(s.file1))
	s.Equal([]byte{
		QueriedSeriesStorageVersion,                    // version
		0xb1, 0x68, 0xde, 0x3a, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x21, 0x33, 0xf7, 0xb8, //crc32
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
		0x21, 0x33, 0xf7, 0xb8, //crc32
		0x05, 0x00, 0x00, 0x00, // size
		'6', '7', '8', '9', '0', // content
	}, s.readFile(s.file1))
	s.Equal([]byte{
		QueriedSeriesStorageVersion,                    // version
		0xd2, 0x02, 0x96, 0x49, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x4e, 0x78, 0xf9, 0xf3, //crc32
		0x05, 0x00, 0x00, 0x00, // size
		'1', '2', '3', '4', '5', // content
	}, s.readFile(s.file2))
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
	_, _ = s.file1.Write([]byte{QueriedSeriesStorageVersion + 1})
	_, _ = s.file2.Write([]byte{QueriedSeriesStorageVersion + 1})

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
		0x4e, 0x78, 0xf9, 0xf3, //crc32
		0x05, 0x00, 0x00,
	}
	_, _ = s.file1.Write(invalidHeader)
	_, _ = s.file2.Write(invalidHeader)

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
		0x4e, 0x78, 0xf9, 0xf3, //crc32
		0x05, 0x00, 0x00, 0x00,
		'1', '2', '3', '4',
	}
	_, _ = s.file1.Write(invalidData)
	_, _ = s.file2.Write(invalidData)

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
		0x4e, 0x78, 0xf9, 0xf2, //crc32
		0x05, 0x00, 0x00, 0x00,
		'1', '2', '3', '4', '5',
	}
	_, _ = s.file1.Write(invalidCrc32)
	_, _ = s.file2.Write(invalidCrc32)

	// Act
	data, err := s.storage.Read()

	// Assert
	s.Equal([]byte(nil), data)
	s.Equal(errors.New("no valid queried series storage"), err)
}

func (s *QueriedSeriesStorageSuite) TestReadFromFirstStorage() {
	// Arrange
	_, _ = s.file1.Write([]byte{
		QueriedSeriesStorageVersion,                    // version
		0xd2, 0x02, 0x96, 0x49, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x4e, 0x78, 0xf9, 0xf3, //crc32
		0x05, 0x00, 0x00, 0x00,
		'1', '2', '3', '4', '5',
	})

	// Act
	data, err := s.storage.Read()

	// Assert
	s.Require().NoError(err)
	s.Equal([]byte("12345"), data)
}

func (s *QueriedSeriesStorageSuite) TestReadFromSecondStorage() {
	// Arrange
	_, _ = s.file2.Write([]byte{
		QueriedSeriesStorageVersion,                    // version
		0xd2, 0x02, 0x96, 0x49, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x4e, 0x78, 0xf9, 0xf3, //crc32
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
	_, _ = s.file1.Write([]byte{
		QueriedSeriesStorageVersion,                    // version
		0xb1, 0x68, 0xde, 0x3a, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x21, 0x33, 0xf7, 0xb8, //crc32
		0x05, 0x00, 0x00, 0x00, // size
		'6', '7', '8', '9', '0', // content
	})
	_, _ = s.file2.Write([]byte{
		QueriedSeriesStorageVersion,                    // version
		0xd3, 0x02, 0x96, 0x49, 0x00, 0x00, 0x00, 0x00, // timestamp
		0xfd, 0x12, 0xf7, 0xe0, //crc32
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
	_, _ = s.file1.Write([]byte{
		QueriedSeriesStorageVersion,                    // version
		0xb1, 0x68, 0xde, 0x3a, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x41, 0x01, 0x44, 0x30, //crc32
		0x00, 0x00, 0x00, 0x00, // size
	})

	// Act
	data, err := s.storage.Read()

	// Assert
	s.Require().NoError(err)
	s.Equal([]byte(nil), data)
}
