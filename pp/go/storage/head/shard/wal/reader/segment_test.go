package reader_test

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"io"
	"testing"

	"github.com/go-faker/faker/v4"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/reader"
)

type SegmentSuite struct {
	suite.Suite
}

func TestSegmentSuite(t *testing.T) {
	suite.Run(t, new(SegmentSuite))
}

func (s *SegmentSuite) TestHappyPath() {
	data := []byte(faker.Paragraph())
	segmentSamples := uint32(42)
	toWrite := []byte{}
	toWrite = append(toWrite, binary.AppendUvarint(nil, uint64(len(data)))...)
	toWrite = append(toWrite, binary.AppendUvarint(nil, uint64(crc32.ChecksumIEEE(data)))...)
	toWrite = append(toWrite, byte(segmentSamples))
	toWrite = append(toWrite, data...)

	buf := &bytes.Buffer{}
	_, err := buf.Write(toWrite)
	s.Require().NoError(err)

	segment := reader.NewSegment()
	_, err = segment.ReadFrom(buf)
	s.Require().NoError(err)

	s.Require().Equal(len(data), segment.Length())
	s.Require().Equal(segmentSamples, segment.Samples())
	s.Require().Equal(data, segment.Bytes())
}

func (s *SegmentSuite) TestReuseSegment() {
	data := []byte(faker.Paragraph())
	segmentSamples := uint32(42)
	toWrite := []byte{}
	toWrite = append(toWrite, binary.AppendUvarint(nil, uint64(len(data)))...)
	toWrite = append(toWrite, binary.AppendUvarint(nil, uint64(crc32.ChecksumIEEE(data)))...)
	toWrite = append(toWrite, byte(segmentSamples))
	toWrite = append(toWrite, data...)

	buf := &bytes.Buffer{}
	_, err := buf.Write(toWrite)
	s.Require().NoError(err)

	segment := reader.NewSegment()
	_, err = segment.ReadFrom(buf)
	s.Require().NoError(err)

	s.Require().Equal(len(data), segment.Length())
	s.Require().Equal(segmentSamples, segment.Samples())
	s.Require().Equal(data, segment.Bytes())

	buf.Reset()
	toWrite = toWrite[:0]
	data = []byte(faker.Paragraph())
	toWrite = append(toWrite, binary.AppendUvarint(nil, uint64(len(data)))...)
	toWrite = append(toWrite, binary.AppendUvarint(nil, uint64(crc32.ChecksumIEEE(data)))...)
	toWrite = append(toWrite, byte(segmentSamples))
	toWrite = append(toWrite, data...)

	_, err = buf.Write(toWrite)
	s.Require().NoError(err)

	segment.Reset()
	_, err = segment.ReadFrom(buf)
	s.Require().NoError(err)

	s.Require().Equal(len(data), segment.Length())
	s.Require().Equal(segmentSamples, segment.Samples())
	s.Require().Equal(data, segment.Bytes())
}

func (s *SegmentSuite) TestCrc32Error() {
	data := []byte(faker.Paragraph())
	segmentSamples := uint32(42)
	toWrite := []byte{}
	toWrite = append(toWrite, binary.AppendUvarint(nil, uint64(len(data)))...)
	toWrite = append(toWrite, binary.AppendUvarint(nil, uint64(0))...)
	toWrite = append(toWrite, byte(segmentSamples))
	toWrite = append(toWrite, data...)

	buf := &bytes.Buffer{}
	_, err := buf.Write(toWrite)
	s.Require().NoError(err)

	segment := reader.NewSegment()
	_, err = segment.ReadFrom(buf)
	s.Require().Error(err)
}

func (s *SegmentSuite) TestCutSegment() {
	data := []byte(faker.Paragraph())
	segmentCrc32 := crc32.ChecksumIEEE(data)
	segmentSamples := uint32(42)
	toWrite := []byte{}
	toWrite = append(toWrite, binary.AppendUvarint(nil, uint64(len(data)))...)
	toWrite = append(toWrite, binary.AppendUvarint(nil, uint64(segmentCrc32))...)
	toWrite = append(toWrite, byte(segmentSamples))
	toWrite = append(toWrite, data[:len(data)-2]...)

	buf := &bytes.Buffer{}
	_, err := buf.Write(toWrite)
	s.Require().NoError(err)

	segment := reader.NewSegment()
	_, err = segment.ReadFrom(buf)
	s.Require().ErrorIs(err, io.ErrUnexpectedEOF)
}
