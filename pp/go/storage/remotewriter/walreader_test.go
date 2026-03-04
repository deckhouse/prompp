package remotewriter

import (
	"hash/crc32"
	"io"
	"testing"

	"github.com/go-faker/faker/v4"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/writer"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/writer/mock"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/stretchr/testify/suite"
)

type WalReaderSuite struct {
	suite.Suite
}

func TestWalReaderSuite(t *testing.T) {
	suite.Run(t, new(WalReaderSuite))
}

func (s *WalReaderSuite) TestReadWalV1() {
	shardFilePath := storage.GetShardWalFilename(s.T().TempDir(), 0)
	shardFile, err := util.CreateFileAppender(shardFilePath, 0o666)
	s.Require().NoError(err)
	_, err = writer.WriteHeader(shardFile, wal.FileFormatVersion, cppbridge.EncodersVersion())
	s.Require().NoError(err)

	expectedSamples := uint32(42)
	data := []byte(faker.Paragraph())
	mockSegment := &mock.EncodedSegmentMock{
		CRC32Func:   func() uint32 { return crc32.ChecksumIEEE(data) },
		SamplesFunc: func() uint32 { return expectedSamples },
		SizeFunc: func() int64 {
			return int64(len(data))
		},
		WriteToFunc: func(w io.Writer) (int64, error) {
			n, werr := w.Write(data)
			return int64(n), werr
		},
	}

	_, err = writer.WriteSegment(shardFile, mockSegment)
	s.Require().NoError(err)
	s.Require().NoError(shardFile.Close())

	walReader, encoderVersion, err := newWalReader(shardFilePath)
	s.Require().NoError(err)
	s.Require().Equal(cppbridge.EncodersVersion(), encoderVersion)
	s.Require().Equal(uint8(wal.FileFormatVersion), walReader.fileFormatVersion)

	segment, err := walReader.EmptySegment()
	s.Require().NoError(err)
	_, ok := segment.(*SegmentV1)
	s.Require().True(ok)
	s.Require().Equal(uint32(0), segment.ID())
	s.Require().Equal(uint32(0), segment.Samples())

	err = walReader.Read(segment)
	s.Require().NoError(err)
	s.Require().Equal(data, segment.Bytes())
	s.Require().Equal(uint32(0), segment.ID())
	s.Require().Equal(expectedSamples, segment.Samples())
	s.Require().Equal(len(data), segment.Length())
}

func (s *WalReaderSuite) TestReadWalV2() {
	shardFilePath := storage.GetShardWalFilename(s.T().TempDir(), 0)
	shardFile, err := util.CreateFileAppender(shardFilePath, 0o666)
	s.Require().NoError(err)
	_, err = writer.WriteHeader(shardFile, wal.FileFormatVersionV2, cppbridge.EncodersVersion())
	s.Require().NoError(err)

	expectedSamples := uint32(42)
	expectedID := uint32(7)
	data := []byte(faker.Paragraph())
	mockSegment := &mock.EncodedSegmentV2Mock{
		CRC32Func:   func() uint32 { return crc32.ChecksumIEEE(data) },
		IDFunc:      func() uint32 { return expectedID },
		SamplesFunc: func() uint32 { return expectedSamples },
		SizeFunc: func() int64 {
			return int64(len(data))
		},
		WriteToFunc: func(w io.Writer) (int64, error) {
			n, werr := w.Write(data)
			return int64(n), werr
		},
	}

	_, err = writer.WriteSegmentV2(shardFile, mockSegment)
	s.Require().NoError(err)
	s.Require().NoError(shardFile.Close())

	walReader, encoderVersion, err := newWalReader(shardFilePath)
	s.Require().NoError(err)
	s.Require().Equal(cppbridge.EncodersVersion(), encoderVersion)
	s.Require().Equal(uint8(wal.FileFormatVersionV2), walReader.fileFormatVersion)

	segment, err := walReader.EmptySegment()
	s.Require().NoError(err)
	_, ok := segment.(*SegmentV2)
	s.Require().True(ok)
	s.Require().Equal(uint32(0), segment.ID())
	s.Require().Equal(uint32(0), segment.Samples())

	err = walReader.Read(segment)
	s.Require().NoError(err)
	s.Require().Equal(data, segment.Bytes())
	s.Require().Equal(expectedID, segment.ID())
	s.Require().Equal(expectedSamples, segment.Samples())
	s.Require().Equal(len(data), segment.Length())
}

func (s *WalReaderSuite) TestReadWalErrorVersion() {
	shardFilePath := storage.GetShardWalFilename(s.T().TempDir(), 0)
	shardFile, err := util.CreateFileAppender(shardFilePath, 0o666)
	s.Require().NoError(err)

	unknownVersion := uint8(3)
	_, err = writer.WriteHeader(shardFile, unknownVersion, cppbridge.EncodersVersion())
	s.Require().NoError(err)

	expectedSamples := uint32(42)
	expectedID := uint32(7)
	data := []byte(faker.Paragraph())
	mockSegment := &mock.EncodedSegmentV2Mock{
		CRC32Func:   func() uint32 { return crc32.ChecksumIEEE(data) },
		IDFunc:      func() uint32 { return expectedID },
		SamplesFunc: func() uint32 { return expectedSamples },
		SizeFunc: func() int64 {
			return int64(len(data))
		},
		WriteToFunc: func(w io.Writer) (int64, error) {
			n, werr := w.Write(data)
			return int64(n), werr
		},
	}

	_, err = writer.WriteSegmentV2(shardFile, mockSegment)
	s.Require().NoError(err)
	s.Require().NoError(shardFile.Close())

	walReader, encoderVersion, err := newWalReader(shardFilePath)
	s.Require().NoError(err)
	s.Require().Equal(cppbridge.EncodersVersion(), encoderVersion)
	s.Require().Equal(unknownVersion, walReader.fileFormatVersion)

	segment, err := walReader.EmptySegment()
	s.Require().Error(err)
	s.Require().Nil(segment)
}

func (s *WalReaderSuite) TestReadIDAndBodyV1() {
	shardFilePath := storage.GetShardWalFilename(s.T().TempDir(), 0)
	shardFile, err := util.CreateFileAppender(shardFilePath, 0o666)
	s.Require().NoError(err)
	_, err = writer.WriteHeader(shardFile, wal.FileFormatVersion, cppbridge.EncodersVersion())
	s.Require().NoError(err)

	expectedSamples := uint32(42)
	data := []byte(faker.Paragraph())
	mockSegment := &mock.EncodedSegmentMock{
		CRC32Func:   func() uint32 { return crc32.ChecksumIEEE(data) },
		SamplesFunc: func() uint32 { return expectedSamples },
		SizeFunc: func() int64 {
			return int64(len(data))
		},
		WriteToFunc: func(w io.Writer) (int64, error) {
			n, werr := w.Write(data)
			return int64(n), werr
		},
	}

	_, err = writer.WriteSegment(shardFile, mockSegment)
	s.Require().NoError(err)
	s.Require().NoError(shardFile.Close())

	walReader, encoderVersion, err := newWalReader(shardFilePath)
	s.Require().NoError(err)
	s.Require().Equal(cppbridge.EncodersVersion(), encoderVersion)
	s.Require().Equal(uint8(wal.FileFormatVersion), walReader.fileFormatVersion)

	segment, err := walReader.EmptySegment()
	s.Require().NoError(err)
	_, ok := segment.(*SegmentV1)
	s.Require().True(ok)
	s.Require().Equal(uint32(0), segment.ID())
	s.Require().Equal(uint32(0), segment.Samples())

	err = walReader.ReadSegmentID(segment)
	s.Require().NoError(err)
	s.Require().Equal(uint32(0), segment.ID())

	err = walReader.ReadSegmentBody(segment)
	s.Require().NoError(err)
	s.Require().Equal(data, segment.Bytes())
	s.Require().Equal(expectedSamples, segment.Samples())
	s.Require().Equal(len(data), segment.Length())
}

func (s *WalReaderSuite) TestReadIDAndBodyV2() {
	shardFilePath := storage.GetShardWalFilename(s.T().TempDir(), 0)
	shardFile, err := util.CreateFileAppender(shardFilePath, 0o666)
	s.Require().NoError(err)
	_, err = writer.WriteHeader(shardFile, wal.FileFormatVersionV2, cppbridge.EncodersVersion())
	s.Require().NoError(err)

	expectedSamples := uint32(42)
	expectedID := uint32(7)
	data := []byte(faker.Paragraph())
	mockSegment := &mock.EncodedSegmentV2Mock{
		CRC32Func:   func() uint32 { return crc32.ChecksumIEEE(data) },
		IDFunc:      func() uint32 { return expectedID },
		SamplesFunc: func() uint32 { return expectedSamples },
		SizeFunc: func() int64 {
			return int64(len(data))
		},
		WriteToFunc: func(w io.Writer) (int64, error) {
			n, werr := w.Write(data)
			return int64(n), werr
		},
	}

	_, err = writer.WriteSegmentV2(shardFile, mockSegment)
	s.Require().NoError(err)
	s.Require().NoError(shardFile.Close())

	walReader, encoderVersion, err := newWalReader(shardFilePath)
	s.Require().NoError(err)
	s.Require().Equal(cppbridge.EncodersVersion(), encoderVersion)
	s.Require().Equal(uint8(wal.FileFormatVersionV2), walReader.fileFormatVersion)

	segment, err := walReader.EmptySegment()
	s.Require().NoError(err)
	_, ok := segment.(*SegmentV2)
	s.Require().True(ok)
	s.Require().Equal(uint32(0), segment.ID())
	s.Require().Equal(uint32(0), segment.Samples())

	err = walReader.ReadSegmentID(segment)
	s.Require().NoError(err)
	s.Require().Equal(expectedID, segment.ID())

	err = walReader.ReadSegmentBody(segment)
	s.Require().NoError(err)
	s.Require().Equal(data, segment.Bytes())
	s.Require().Equal(expectedSamples, segment.Samples())
	s.Require().Equal(len(data), segment.Length())
}
