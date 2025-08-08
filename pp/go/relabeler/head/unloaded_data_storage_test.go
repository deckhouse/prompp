package head

import (
	"bytes"
	"errors"
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

type BufferWriteTruncateCloser struct {
	buffer *bytes.Buffer
}

func NewBufferWriteTruncateCloser() *BufferWriteTruncateCloser {
	return &BufferWriteTruncateCloser{
		buffer: bytes.NewBuffer(nil),
	}
}

func (b *BufferWriteTruncateCloser) Write(p []byte) (n int, err error) {
	return b.buffer.Write(p)
}

func (b *BufferWriteTruncateCloser) Close() error {
	return nil
}

func (b *BufferWriteTruncateCloser) Truncate(size int64) error {
	b.buffer.Truncate(int(size))
	return nil
}

func (b *BufferWriteTruncateCloser) Bytes() []byte {
	return b.buffer.Bytes()
}

type QueriedSeriesStorageWriterSuite struct {
	suite.Suite
	buffer1 *BufferWriteTruncateCloser
	buffer2 *BufferWriteTruncateCloser
	writer  *QueriedSeriesStorageWriter
}

func TestQueriedSeriesStorageWriterSuite(t *testing.T) {
	suite.Run(t, new(QueriedSeriesStorageWriterSuite))
}

func (s *QueriedSeriesStorageWriterSuite) SetupTest() {
	s.buffer1 = NewBufferWriteTruncateCloser()
	s.buffer2 = NewBufferWriteTruncateCloser()
	s.writer = NewQueriedSeriesStorageWriter(s.buffer1, s.buffer2)
}

func (s *QueriedSeriesStorageWriterSuite) TestWriteInFirstStorage() {
	// Arrange

	// Act
	err := s.writer.Write([]byte("12345"), 1234567890)

	// Assert
	s.NoError(err)
	s.Equal([]byte{
		QueriedSeriesStorageVersion,                    // version
		0xd2, 0x02, 0x96, 0x49, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x4e, 0x78, 0xf9, 0xf3, //crc32
		0x05, 0x00, 0x00, 0x00, // size
		'1', '2', '3', '4', '5', // content
	}, s.buffer1.Bytes())
	s.Equal([]byte(nil), s.buffer2.Bytes())
}

func (s *QueriedSeriesStorageWriterSuite) TestWriteInAllStorages() {
	// Arrange

	// Act
	err1 := s.writer.Write([]byte("12345"), 1234567890)
	err2 := s.writer.Write([]byte("67890"), 987654321)

	// Assert
	s.NoError(err1)
	s.NoError(err2)
	s.Equal([]byte{
		QueriedSeriesStorageVersion,                    // version
		0xd2, 0x02, 0x96, 0x49, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x4e, 0x78, 0xf9, 0xf3, //crc32
		0x05, 0x00, 0x00, 0x00, // size
		'1', '2', '3', '4', '5', // content
	}, s.buffer1.Bytes())
	s.Equal([]byte{
		QueriedSeriesStorageVersion,                    // version
		0xb1, 0x68, 0xde, 0x3a, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x21, 0x33, 0xf7, 0xb8, //crc32
		0x05, 0x00, 0x00, 0x00, // size
		'6', '7', '8', '9', '0', // content
	}, s.buffer2.Bytes())
}

func (s *QueriedSeriesStorageWriterSuite) TestMultipleWriteInFirstStorage() {
	// Arrange

	// Act
	_ = s.writer.Write([]byte("12345"), 1234567890)
	_ = s.writer.Write([]byte("67890"), 987654321)
	_ = s.writer.Write([]byte("67890"), 987654321)
	_ = s.writer.Write([]byte("12345"), 1234567890)

	// Assert
	s.Equal([]byte{
		QueriedSeriesStorageVersion,                    // version
		0xb1, 0x68, 0xde, 0x3a, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x21, 0x33, 0xf7, 0xb8, //crc32
		0x05, 0x00, 0x00, 0x00, // size
		'6', '7', '8', '9', '0', // content
	}, s.buffer1.Bytes())
	s.Equal([]byte{
		QueriedSeriesStorageVersion,                    // version
		0xd2, 0x02, 0x96, 0x49, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x4e, 0x78, 0xf9, 0xf3, //crc32
		0x05, 0x00, 0x00, 0x00, // size
		'1', '2', '3', '4', '5', // content
	}, s.buffer2.Bytes())
}

type QueriedSeriesStorageReaderSuite struct {
	QueriedSeriesStorageWriterSuite
}

func TestQueriedSeriesStorageReaderSuite(t *testing.T) {
	suite.Run(t, new(QueriedSeriesStorageReaderSuite))
}

func (s *QueriedSeriesStorageReaderSuite) createReader() *QueriedSeriesStorageReader {
	return NewQueriedSeriesStorageReader(bytes.NewReader(s.buffer1.Bytes()), bytes.NewReader(s.buffer2.Bytes()))
}

func (s *QueriedSeriesStorageReaderSuite) TestReadInEmptyFiles() {
	// Arrange
	reader := s.createReader()

	// Act
	data, err := reader.Read()

	// Assert
	s.Equal([]byte(nil), data)
	s.Equal(errors.New("no valid queried series storage"), err)
}

func (s *QueriedSeriesStorageReaderSuite) TestInvalidVersionInAllStorages() {
	// Arrange
	_, _ = s.buffer1.Write([]byte{QueriedSeriesStorageVersion + 1})
	_, _ = s.buffer2.Write([]byte{QueriedSeriesStorageVersion + 1})
	reader := s.createReader()

	// Act
	data, err := reader.Read()

	// Assert
	s.Equal([]byte(nil), data)
	s.Equal(errors.New("no valid queried series storage"), err)
}

func (s *QueriedSeriesStorageReaderSuite) TestInvalidHeaderInAllStorages() {
	// Arrange
	invalidHeader := []byte{
		QueriedSeriesStorageVersion,                    // version
		0xd2, 0x02, 0x96, 0x49, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x4e, 0x78, 0xf9, 0xf3, //crc32
		0x05, 0x00, 0x00,
	}
	_, _ = s.buffer1.Write(invalidHeader)
	_, _ = s.buffer2.Write(invalidHeader)
	reader := s.createReader()

	// Act
	data, err := reader.Read()

	// Assert
	s.Equal([]byte(nil), data)
	s.Equal(errors.New("no valid queried series storage"), err)
}

func (s *QueriedSeriesStorageReaderSuite) TestInvalidDataInAllStorages() {
	// Arrange
	invalidData := []byte{
		QueriedSeriesStorageVersion,                    // version
		0xd2, 0x02, 0x96, 0x49, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x4e, 0x78, 0xf9, 0xf3, //crc32
		0x05, 0x00, 0x00, 0x00,
		'1', '2', '3', '4',
	}
	_, _ = s.buffer1.Write(invalidData)
	_, _ = s.buffer2.Write(invalidData)
	reader := s.createReader()

	// Act
	data, err := reader.Read()

	// Assert
	s.Equal([]byte(nil), data)
	s.Equal(errors.New("no valid queried series storage"), err)
}

func (s *QueriedSeriesStorageReaderSuite) TestInvalidCrc32InAllStorages() {
	// Arrange
	invalidCrc32 := []byte{
		QueriedSeriesStorageVersion,                    // version
		0xd2, 0x02, 0x96, 0x49, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x4e, 0x78, 0xf9, 0xf2, //crc32
		0x05, 0x00, 0x00, 0x00,
		'1', '2', '3', '4', '5',
	}
	_, _ = s.buffer1.Write(invalidCrc32)
	_, _ = s.buffer2.Write(invalidCrc32)
	reader := s.createReader()

	// Act
	data, err := reader.Read()

	// Assert
	s.Equal([]byte(nil), data)
	s.Equal(errors.New("no valid queried series storage"), err)
}

func (s *QueriedSeriesStorageReaderSuite) TestReadFromFirstStorage() {
	// Arrange
	_, _ = s.buffer1.Write([]byte{
		QueriedSeriesStorageVersion,                    // version
		0xd2, 0x02, 0x96, 0x49, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x4e, 0x78, 0xf9, 0xf3, //crc32
		0x05, 0x00, 0x00, 0x00,
		'1', '2', '3', '4', '5',
	})
	reader := s.createReader()

	// Act
	data, err := reader.Read()

	// Assert
	s.Require().NoError(err)
	s.Equal([]byte("12345"), data)
}

func (s *QueriedSeriesStorageReaderSuite) TestReadFromSecondStorage() {
	// Arrange
	_, _ = s.buffer2.Write([]byte{
		QueriedSeriesStorageVersion,                    // version
		0xd2, 0x02, 0x96, 0x49, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x4e, 0x78, 0xf9, 0xf3, //crc32
		0x05, 0x00, 0x00, 0x00,
		'1', '2', '3', '4', '5',
	})
	reader := s.createReader()

	// Act
	data, err := reader.Read()

	// Assert
	s.Require().NoError(err)
	s.Equal([]byte("12345"), data)
}

func (s *QueriedSeriesStorageReaderSuite) TestReadFromStorageWithMaxTimestamp() {
	// Arrange
	_, _ = s.buffer1.Write([]byte{
		QueriedSeriesStorageVersion,                    // version
		0xb1, 0x68, 0xde, 0x3a, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x21, 0x33, 0xf7, 0xb8, //crc32
		0x05, 0x00, 0x00, 0x00, // size
		'6', '7', '8', '9', '0', // content
	})
	_, _ = s.buffer2.Write([]byte{
		QueriedSeriesStorageVersion,                    // version
		0xd3, 0x02, 0x96, 0x49, 0x00, 0x00, 0x00, 0x00, // timestamp
		0xfd, 0x12, 0xf7, 0xe0, //crc32
		0x04, 0x00, 0x00, 0x00,
		'6', '7', '8', '9',
	})
	reader := s.createReader()

	// Act
	data, err := reader.Read()

	// Assert
	s.Require().NoError(err)
	s.Equal([]byte("6789"), data)
}

func (s *QueriedSeriesStorageReaderSuite) TestReadEmptyContent() {
	// Arrange
	_, _ = s.buffer1.Write([]byte{
		QueriedSeriesStorageVersion,                    // version
		0xb1, 0x68, 0xde, 0x3a, 0x00, 0x00, 0x00, 0x00, // timestamp
		0x41, 0x01, 0x44, 0x30, //crc32
		0x00, 0x00, 0x00, 0x00, // size
	})
	reader := s.createReader()

	// Act
	data, err := reader.Read()

	// Assert
	s.Require().NoError(err)
	s.Equal([]byte(nil), data)
}
