package writer_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/go-faker/faker/v4"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/writer"
)

type BufferedSuite struct {
	suite.Suite
}

func TestBufferedSuite(t *testing.T) {
	suite.Run(t, new(BufferedSuite))
}

func (s *BufferedSuite) TestWriteFlushSync() {
	actual := &bytes.Buffer{}
	sfile := s.openfile(actual)

	swn := &SegmentIsWrittenNotifierMock{
		NotifySegmentIsWrittenFunc: func() {},
		NotifySegmentWriteFunc:     func(uint16) {},
	}

	shardID := uint16(0)
	wrBuf, err := writer.NewBuffered(shardID, sfile, writer.WriteSegment[*EncodedSegmentMock], swn)
	s.Require().NoError(err)
	s.Equal(int64(0), wrBuf.CurrentSize())

	segment, expectedSegment := s.genSegment()

	err = wrBuf.Write(segment)
	s.Require().NoError(err)
	s.Empty(sfile.WriteCalls())

	err = wrBuf.Flush()
	s.Require().NoError(err)
	s.Equal(int64(len(expectedSegment)), wrBuf.CurrentSize())
	s.Equal(expectedSegment, actual.Bytes())

	err = wrBuf.Sync()
	s.Require().NoError(err)
	s.Len(sfile.SyncCalls(), 1)
	s.Len(swn.NotifySegmentIsWrittenCalls(), 1)

	err = wrBuf.Close()
	s.Require().NoError(err)
	s.Len(sfile.CloseCalls(), 1)
}

func (s *BufferedSuite) TestDoubleWriteAndFlush() {
	actual := &bytes.Buffer{}
	sfile := s.openfile(actual)

	numberOfSegments := 0
	swn := &SegmentIsWrittenNotifierMock{
		NotifySegmentIsWrittenFunc: func() {},
		NotifySegmentWriteFunc:     func(uint16) { numberOfSegments++ },
	}

	shardID := uint16(0)
	wrBuf, err := writer.NewBuffered(shardID, sfile, writer.WriteSegment[*EncodedSegmentMock], swn)
	s.Require().NoError(err)
	s.Equal(int64(0), wrBuf.CurrentSize())

	segment, expectedSegment := s.genSegment()
	expectedLen := len(expectedSegment)

	err = wrBuf.Write(segment)
	s.Require().NoError(err)
	s.Empty(sfile.WriteCalls())

	err = wrBuf.Flush()
	s.Require().NoError(err)
	s.Equal(int64(expectedLen), wrBuf.CurrentSize())
	s.Equal(expectedSegment, actual.Bytes())

	actual.Reset()
	segment, expectedSegment = s.genSegment()

	err = wrBuf.Write(segment)
	s.Require().NoError(err)

	err = wrBuf.Flush()
	s.Require().NoError(err)
	s.Equal(int64(len(expectedSegment)+expectedLen), wrBuf.CurrentSize())
	s.Equal(expectedSegment, actual.Bytes())

	err = wrBuf.Sync()
	s.Require().NoError(err)
	s.Len(sfile.SyncCalls(), 1)
	s.Len(swn.NotifySegmentIsWrittenCalls(), 1)
	s.Len(swn.NotifySegmentWriteCalls(), 2)
	s.Equal(2, numberOfSegments)

	err = wrBuf.Close()
	s.Require().NoError(err)
	s.Len(sfile.CloseCalls(), 1)
}

func (s *BufferedSuite) TestBuffered() {
	actual := &bytes.Buffer{}
	sfile := s.openfile(actual)

	swn := &SegmentIsWrittenNotifierMock{
		NotifySegmentIsWrittenFunc: func() {},
		NotifySegmentWriteFunc:     func(uint16) {},
	}

	shardID := uint16(0)
	wrBuf, err := writer.NewBuffered(shardID, sfile, writer.WriteSegment[*EncodedSegmentMock], swn)
	s.Require().NoError(err)
	s.Equal(int64(0), wrBuf.CurrentSize())

	expectedSegments := []byte{}
	expectedSize := 0
	for range 10 {
		segment, expectedSegment := s.genSegment()
		err = wrBuf.Write(segment)
		s.Require().NoError(err)
		s.Empty(sfile.WriteCalls())
		expectedSegments = append(expectedSegments, expectedSegment...)
		expectedSize += len(expectedSegment)
	}

	err = wrBuf.Flush()
	s.Require().NoError(err)
	s.Equal(int64(expectedSize), wrBuf.CurrentSize())
	s.Equal(expectedSegments, actual.Bytes())
	s.Empty(swn.NotifySegmentIsWrittenCalls())
	s.Len(swn.NotifySegmentWriteCalls(), 10)

	err = wrBuf.Sync()
	s.Require().NoError(err)
	s.Len(sfile.SyncCalls(), 1)
	s.Len(swn.NotifySegmentIsWrittenCalls(), 1)

	err = wrBuf.Close()
	s.Require().NoError(err)
	s.Len(sfile.CloseCalls(), 1)
}

func (s *BufferedSuite) TestStatError() {
	actual := &bytes.Buffer{}
	sfile := s.openfile(actual)
	sfile.StatFunc = func() (os.FileInfo, error) { return nil, errors.New("some error") }

	swn := &SegmentIsWrittenNotifierMock{
		NotifySegmentIsWrittenFunc: func() {},
		NotifySegmentWriteFunc:     func(uint16) {},
	}

	shardID := uint16(0)
	wrBuf, err := writer.NewBuffered(shardID, sfile, writer.WriteSegment[*EncodedSegmentMock], swn)
	s.Require().Error(err)
	s.Require().Nil(wrBuf)
}

func (s *BufferedSuite) TestSyncError() {
	actual := &bytes.Buffer{}
	sfile := s.openfile(actual)
	sfile.SyncFunc = func() error { return errors.New("some error") }

	swn := &SegmentIsWrittenNotifierMock{
		NotifySegmentIsWrittenFunc: func() {},
		NotifySegmentWriteFunc:     func(uint16) {},
	}

	shardID := uint16(0)
	wrBuf, err := writer.NewBuffered(shardID, sfile, writer.WriteSegment[*EncodedSegmentMock], swn)
	s.Require().NoError(err)
	s.Equal(int64(0), wrBuf.CurrentSize())

	segment, expectedSegment := s.genSegment()

	err = wrBuf.Write(segment)
	s.Require().NoError(err)
	s.Empty(sfile.WriteCalls())

	err = wrBuf.Flush()
	s.Require().NoError(err)
	s.Equal(int64(len(expectedSegment)), wrBuf.CurrentSize())
	s.Equal(expectedSegment, actual.Bytes())

	err = wrBuf.Sync()
	s.Require().Error(err)
	s.Len(sfile.SyncCalls(), 1)
	s.Empty(swn.NotifySegmentIsWrittenCalls())
}

func (s *BufferedSuite) TestWriteToBufferWithError() {
	actual := &bytes.Buffer{}
	sfile := s.openfile(actual)

	swn := &SegmentIsWrittenNotifierMock{
		NotifySegmentIsWrittenFunc: func() {},
		NotifySegmentWriteFunc:     func(uint16) {},
	}

	scount := 0
	writeSegment := func(w io.Writer, segment *EncodedSegmentMock) (n int, err error) {
		if scount == 5 {
			scount++
			return 0, errors.New("some error")
		}

		scount++
		return writer.WriteSegment(w, segment)
	}

	shardID := uint16(0)
	wrBuf, err := writer.NewBuffered(shardID, sfile, writeSegment, swn)
	s.Require().NoError(err)
	s.Equal(int64(0), wrBuf.CurrentSize())

	expectedSegments := []byte{}
	expectedSize := 0
	for range 10 {
		segment, expectedSegment := s.genSegment()
		err = wrBuf.Write(segment)
		s.Require().NoError(err)
		s.Empty(sfile.WriteCalls())
		expectedSegments = append(expectedSegments, expectedSegment...)
		expectedSize += len(expectedSegment)
	}

	err = wrBuf.Flush()
	s.Require().Error(err)

	err = wrBuf.Flush()
	s.Require().NoError(err)
	s.Equal(int64(expectedSize), wrBuf.CurrentSize())
	s.Equal(expectedSegments, actual.Bytes())
	s.Empty(swn.NotifySegmentIsWrittenCalls())

	err = wrBuf.Sync()
	s.Require().NoError(err)
	s.Len(sfile.SyncCalls(), 1)
	s.Len(swn.NotifySegmentIsWrittenCalls(), 1)

	err = wrBuf.Close()
	s.Require().NoError(err)
	s.Len(sfile.CloseCalls(), 1)
}

func (s *BufferedSuite) TestFlushWithError() {
	actual := &bytes.Buffer{}
	sfile := s.openfile(actual)

	swn := &SegmentIsWrittenNotifierMock{
		NotifySegmentIsWrittenFunc: func() {},
		NotifySegmentWriteFunc:     func(uint16) {},
	}

	scount := 0
	sfile.WriteFunc = func(p []byte) (int, error) {
		if scount == 4 || scount == 5 {
			scount++
			return 0, errors.New("some error")
		}

		scount++

		return actual.Write(p)
	}

	shardID := uint16(0)
	wrBuf, err := writer.NewBuffered(shardID, sfile, writer.WriteSegment[*EncodedSegmentMock], swn)
	s.Require().NoError(err)
	s.Equal(int64(0), wrBuf.CurrentSize())

	expectedSegments := []byte{}
	expectedSize := 0
	for range 10 {
		segment, expectedSegment := s.genSegment()
		err = wrBuf.Write(segment)
		s.Require().NoError(err)
		s.Empty(sfile.WriteCalls())
		expectedSegments = append(expectedSegments, expectedSegment...)
		expectedSize += len(expectedSegment)
	}

	err = wrBuf.Flush()
	s.Require().Error(err)

	err = wrBuf.Flush()
	s.Require().Error(err)

	err = wrBuf.Flush()
	s.Require().NoError(err)
	s.Equal(int64(expectedSize), wrBuf.CurrentSize())
	s.Equal(expectedSegments, actual.Bytes())
	s.Empty(swn.NotifySegmentIsWrittenCalls())

	err = wrBuf.Sync()
	s.Require().NoError(err)
	s.Len(sfile.SyncCalls(), 1)
	s.Len(swn.NotifySegmentIsWrittenCalls(), 1)

	err = wrBuf.Close()
	s.Require().NoError(err)
	s.Len(sfile.CloseCalls(), 1)
}

func (*BufferedSuite) openfile(buf *bytes.Buffer) *FileWriterMock {
	return &FileWriterMock{
		CloseFunc: func() error { return nil },
		StatFunc:  func() (os.FileInfo, error) { return &FileInfoMock{SizeFunc: func() int64 { return 0 }}, nil },
		SyncFunc:  func() error { return nil },
		WriteFunc: buf.Write,
	}
}

func (s *BufferedSuite) genSegment() (segment *EncodedSegmentMock, expected []byte) {
	segmentCrc32 := uint32(0)
	segmentSamples := uint32(42)
	data := []byte(faker.Paragraph())

	segment = &EncodedSegmentMock{
		CRC32Func: func() uint32 {
			return segmentCrc32
		},
		SamplesFunc: func() uint32 {
			return segmentSamples
		},
		SizeFunc: func() int64 {
			return int64(len(data))
		},
		WriteToFunc: func(w io.Writer) (int64, error) {
			n, errWr := w.Write(data)
			return int64(n), errWr
		},
	}

	buf := &bytes.Buffer{}
	_, err := writer.WriteSegment(buf, segment)
	s.Require().NoError(err)

	return segment, buf.Bytes()
}
