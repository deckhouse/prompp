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

//
// SegmentV2Suite
//

type SegmentV2Suite struct {
	suite.Suite
}

func TestSegmentV2Suite(t *testing.T) {
	suite.Run(t, new(SegmentV2Suite))
}

func (s *SegmentV2Suite) TestHappyPath() {
	segmentID := uint32(10)
	data := []byte(faker.Paragraph())
	segmentSamples := uint32(42)
	toWrite := []byte{}
	toWrite = append(toWrite, byte(segmentID))
	toWrite = append(toWrite, binary.AppendUvarint(nil, uint64(len(data)))...)
	toWrite = append(toWrite, binary.AppendUvarint(nil, uint64(crc32.ChecksumIEEE(data)))...)
	toWrite = append(toWrite, byte(segmentSamples))
	toWrite = append(toWrite, data...)

	buf := &bytes.Buffer{}
	_, err := buf.Write(toWrite)
	s.Require().NoError(err)

	segment := reader.NewSegmentV2()
	n, err := segment.ReadFrom(buf)
	s.Require().NoError(err)

	s.Require().Equal(int64(len(toWrite)), n)
	s.Require().Equal(segmentID, segment.ID())
	s.Require().Equal(len(data), segment.Length())
	s.Require().Equal(segmentSamples, segment.Samples())
	s.Require().Equal(data, segment.Bytes())
}

func (s *SegmentV2Suite) TestReuseSegment() {
	segmentID := uint32(10)
	data := []byte(faker.Paragraph())
	segmentSamples := uint32(42)
	toWrite := []byte{}
	toWrite = append(toWrite, byte(segmentID))
	toWrite = append(toWrite, binary.AppendUvarint(nil, uint64(len(data)))...)
	toWrite = append(toWrite, binary.AppendUvarint(nil, uint64(crc32.ChecksumIEEE(data)))...)
	toWrite = append(toWrite, byte(segmentSamples))
	toWrite = append(toWrite, data...)

	buf := &bytes.Buffer{}
	_, err := buf.Write(toWrite)
	s.Require().NoError(err)

	segment := reader.NewSegmentV2()
	n, err := segment.ReadFrom(buf)
	s.Require().NoError(err)

	s.Require().Equal(int64(len(toWrite)), n)
	s.Require().Equal(segmentID, segment.ID())
	s.Require().Equal(len(data), segment.Length())
	s.Require().Equal(segmentSamples, segment.Samples())
	s.Require().Equal(data, segment.Bytes())

	buf.Reset()
	toWrite = toWrite[:0]
	segmentID++
	data = []byte(faker.Paragraph())
	toWrite = append(toWrite, byte(segmentID))
	toWrite = append(toWrite, binary.AppendUvarint(nil, uint64(len(data)))...)
	toWrite = append(toWrite, binary.AppendUvarint(nil, uint64(crc32.ChecksumIEEE(data)))...)
	toWrite = append(toWrite, byte(segmentSamples))
	toWrite = append(toWrite, data...)

	_, err = buf.Write(toWrite)
	s.Require().NoError(err)

	segment.Reset()
	n, err = segment.ReadFrom(buf)
	s.Require().NoError(err)

	s.Require().Equal(int64(len(toWrite)), n)
	s.Require().Equal(segmentID, segment.ID())
	s.Require().Equal(len(data), segment.Length())
	s.Require().Equal(segmentSamples, segment.Samples())
	s.Require().Equal(data, segment.Bytes())
}

func (s *SegmentV2Suite) TestCrc32Error() {
	segmentID := uint32(10)
	data := []byte(faker.Paragraph())
	segmentSamples := uint32(42)
	toWrite := []byte{}
	toWrite = append(toWrite, byte(segmentID))
	toWrite = append(toWrite, binary.AppendUvarint(nil, uint64(len(data)))...)
	toWrite = append(toWrite, binary.AppendUvarint(nil, uint64(0))...)
	toWrite = append(toWrite, byte(segmentSamples))
	toWrite = append(toWrite, data...)

	buf := &bytes.Buffer{}
	_, err := buf.Write(toWrite)
	s.Require().NoError(err)

	segment := reader.NewSegmentV2()
	n, err := segment.ReadFrom(buf)
	s.Require().Error(err)
	s.Require().Equal(int64(len(toWrite)), n)
}

func (s *SegmentV2Suite) TestCutSegment() {
	segmentID := uint32(10)
	data := []byte(faker.Paragraph())
	segmentCrc32 := crc32.ChecksumIEEE(data)
	segmentSamples := uint32(42)
	toWrite := []byte{}
	toWrite = append(toWrite, byte(segmentID))
	toWrite = append(toWrite, binary.AppendUvarint(nil, uint64(len(data)))...)
	toWrite = append(toWrite, binary.AppendUvarint(nil, uint64(segmentCrc32))...)
	toWrite = append(toWrite, byte(segmentSamples))
	expectedBytes := int64(len(toWrite))
	toWrite = append(toWrite, data[:len(data)-2]...)

	buf := &bytes.Buffer{}
	_, err := buf.Write(toWrite)
	s.Require().NoError(err)

	segment := reader.NewSegmentV2()
	n, err := segment.ReadFrom(buf)
	s.Require().ErrorIs(err, io.ErrUnexpectedEOF)
	s.Require().Equal(expectedBytes, n)
}

func (s *SegmentV2Suite) TestReadIDAndBody() {
	segmentID := uint32(10)
	data := []byte(faker.Paragraph())
	segmentSamples := uint32(42)
	toWrite := []byte{}
	toWrite = append(toWrite, byte(segmentID))
	expectedIDBytes := int64(len(toWrite))
	toWrite = append(toWrite, binary.AppendUvarint(nil, uint64(len(data)))...)
	toWrite = append(toWrite, binary.AppendUvarint(nil, uint64(crc32.ChecksumIEEE(data)))...)
	toWrite = append(toWrite, byte(segmentSamples))
	toWrite = append(toWrite, data...)

	buf := &bytes.Buffer{}
	_, err := buf.Write(toWrite)
	s.Require().NoError(err)

	segment := reader.NewSegmentV2()
	n, err := segment.ReadID(buf)
	s.Require().NoError(err)

	s.Require().Equal(expectedIDBytes, n)
	s.Require().Equal(segmentID, segment.ID())

	n, err = segment.ReadBody(buf)
	s.Require().NoError(err)

	s.Require().Equal(int64(len(toWrite))-expectedIDBytes, n)
	s.Require().Equal(len(data), segment.Length())
	s.Require().Equal(segmentSamples, segment.Samples())
	s.Require().Equal(data, segment.Bytes())
}
