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
		NotifySegmentIsWrittenFunc: func(uint16) {},
		NotifySegmentWriteFunc:     func(uint16) {},
	}

	segmentID := uint32(2)
	sm := &SegmentMarkupMock{
		NextSegmentIDFunc:       func() uint32 { return segmentID },
		SetSegmentIDByShardFunc: func(uint32, uint16) {},
	}

	shardID := uint16(1)
	wrBuf, err := writer.NewBuffered(shardID, sfile, writer.WriteSegment[*EncodedSegmentV2Mock], swn, sm)
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

	s.Require().Len(sm.SetSegmentIDByShardCalls(), 1)
	s.Equal(segmentID, sm.SetSegmentIDByShardCalls()[0].Sid)
	s.Equal(shardID, sm.SetSegmentIDByShardCalls()[0].ShardID)

	err = wrBuf.Close()
	s.Require().NoError(err)
	s.Len(sfile.CloseCalls(), 1)
}

func (s *BufferedSuite) TestDoubleWriteAndFlush() {
	actual := &bytes.Buffer{}
	sfile := s.openfile(actual)

	numberOfSegments := 0
	swn := &SegmentIsWrittenNotifierMock{
		NotifySegmentIsWrittenFunc: func(uint16) {},
		NotifySegmentWriteFunc:     func(uint16) { numberOfSegments++ },
	}

	segmentID := uint32(2)
	sm := &SegmentMarkupMock{
		NextSegmentIDFunc: func() uint32 {
			segmentID++
			return segmentID
		},
		SetSegmentIDByShardFunc: func(uint32, uint16) {},
	}

	shardID := uint16(1)
	wrBuf, err := writer.NewBuffered(shardID, sfile, writer.WriteSegment[*EncodedSegmentV2Mock], swn, sm)
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

	s.Require().Len(sm.SetSegmentIDByShardCalls(), 2)
	s.Equal(segmentID-1, sm.SetSegmentIDByShardCalls()[0].Sid)
	s.Equal(shardID, sm.SetSegmentIDByShardCalls()[0].ShardID)
	s.Equal(segmentID, sm.SetSegmentIDByShardCalls()[1].Sid)
	s.Equal(shardID, sm.SetSegmentIDByShardCalls()[1].ShardID)

	err = wrBuf.Close()
	s.Require().NoError(err)
	s.Len(sfile.CloseCalls(), 1)
}

func (s *BufferedSuite) TestBuffered() {
	actual := &bytes.Buffer{}
	sfile := s.openfile(actual)

	swn := &SegmentIsWrittenNotifierMock{
		NotifySegmentIsWrittenFunc: func(uint16) {},
		NotifySegmentWriteFunc:     func(uint16) {},
	}

	segmentID := uint32(2)
	segments := make([]uint32, 0, 10)
	sm := &SegmentMarkupMock{
		NextSegmentIDFunc: func() uint32 {
			segmentID++
			segments = append(segments, segmentID)
			return segmentID
		},
		SetSegmentIDByShardFunc: func(uint32, uint16) {},
	}

	shardID := uint16(1)
	wrBuf, err := writer.NewBuffered(shardID, sfile, writer.WriteSegment[*EncodedSegmentV2Mock], swn, sm)
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

	s.Require().Len(sm.SetSegmentIDByShardCalls(), len(segments))
	for i, sid := range segments {
		s.Equal(sid, sm.SetSegmentIDByShardCalls()[i].Sid)
		s.Equal(shardID, sm.SetSegmentIDByShardCalls()[i].ShardID)
	}

	err = wrBuf.Close()
	s.Require().NoError(err)
	s.Len(sfile.CloseCalls(), 1)
}

func (s *BufferedSuite) TestStatError() {
	actual := &bytes.Buffer{}
	sfile := s.openfile(actual)
	sfile.StatFunc = func() (os.FileInfo, error) { return nil, errors.New("some error") }

	swn := &SegmentIsWrittenNotifierMock{
		NotifySegmentIsWrittenFunc: func(uint16) {},
		NotifySegmentWriteFunc:     func(uint16) {},
	}

	sm := &SegmentMarkupMock{
		NextSegmentIDFunc: func() uint32 {
			return 0
		},
		SetSegmentIDByShardFunc: func(uint32, uint16) {},
	}

	shardID := uint16(0)
	wrBuf, err := writer.NewBuffered(shardID, sfile, writer.WriteSegment[*EncodedSegmentV2Mock], swn, sm)
	s.Require().Error(err)
	s.Require().Nil(wrBuf)
}

func (s *BufferedSuite) TestSyncError() {
	actual := &bytes.Buffer{}
	sfile := s.openfile(actual)
	sfile.SyncFunc = func() error { return errors.New("some error") }

	swn := &SegmentIsWrittenNotifierMock{
		NotifySegmentIsWrittenFunc: func(uint16) {},
		NotifySegmentWriteFunc:     func(uint16) {},
	}

	segmentID := uint32(2)
	sm := &SegmentMarkupMock{
		NextSegmentIDFunc:       func() uint32 { return segmentID },
		SetSegmentIDByShardFunc: func(uint32, uint16) {},
	}

	shardID := uint16(0)
	wrBuf, err := writer.NewBuffered(shardID, sfile, writer.WriteSegment[*EncodedSegmentV2Mock], swn, sm)
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
	s.Empty(sm.SetSegmentIDByShardCalls())
}

func (s *BufferedSuite) TestWriteToBufferWithError() {
	actual := &bytes.Buffer{}
	sfile := s.openfile(actual)

	swn := &SegmentIsWrittenNotifierMock{
		NotifySegmentIsWrittenFunc: func(uint16) {},
		NotifySegmentWriteFunc:     func(uint16) {},
	}

	scount := 0
	writeSegment := func(w io.Writer, segment *EncodedSegmentV2Mock) (n int, err error) {
		if scount == 5 {
			scount++
			return 0, errors.New("some error")
		}

		scount++
		return writer.WriteSegment(w, segment)
	}

	segmentID := uint32(2)
	segments := make([]uint32, 0, 10)
	sm := &SegmentMarkupMock{
		NextSegmentIDFunc: func() uint32 {
			segmentID++
			segments = append(segments, segmentID)
			return segmentID
		},
		SetSegmentIDByShardFunc: func(uint32, uint16) {},
	}

	shardID := uint16(1)
	wrBuf, err := writer.NewBuffered(shardID, sfile, writeSegment, swn, sm)
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

	s.Require().Len(sm.SetSegmentIDByShardCalls(), len(segments))
	for i, sid := range segments {
		s.Equal(sid, sm.SetSegmentIDByShardCalls()[i].Sid)
		s.Equal(shardID, sm.SetSegmentIDByShardCalls()[i].ShardID)
	}

	err = wrBuf.Close()
	s.Require().NoError(err)
	s.Len(sfile.CloseCalls(), 1)
}

func (s *BufferedSuite) TestFlushWithError() {
	actual := &bytes.Buffer{}
	sfile := s.openfile(actual)

	swn := &SegmentIsWrittenNotifierMock{
		NotifySegmentIsWrittenFunc: func(uint16) {},
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

	segmentID := uint32(2)
	segments := make([]uint32, 0, 10)
	sm := &SegmentMarkupMock{
		NextSegmentIDFunc: func() uint32 {
			segmentID++
			segments = append(segments, segmentID)
			return segmentID
		},
		SetSegmentIDByShardFunc: func(uint32, uint16) {},
	}

	shardID := uint16(1)
	wrBuf, err := writer.NewBuffered(shardID, sfile, writer.WriteSegment[*EncodedSegmentV2Mock], swn, sm)
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

	s.Require().Len(sm.SetSegmentIDByShardCalls(), len(segments))
	for i, sid := range segments {
		s.Equal(sid, sm.SetSegmentIDByShardCalls()[i].Sid)
		s.Equal(shardID, sm.SetSegmentIDByShardCalls()[i].ShardID)
	}

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

func (s *BufferedSuite) genSegment() (segment *EncodedSegmentV2Mock, expected []byte) {
	segmentCrc32 := uint32(0)
	segmentSamples := uint32(42)
	data := []byte(faker.Paragraph())
	var id uint32

	segment = &EncodedSegmentV2Mock{
		IDFunc: func() uint32 {
			return id
		},
		CRC32Func: func() uint32 {
			return segmentCrc32
		},
		SamplesFunc: func() uint32 {
			return segmentSamples
		},
		SetSegmentIDFunc: func(sid uint32) {
			id = sid
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
