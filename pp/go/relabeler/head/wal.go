package head

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

const (
	FileFormatVersion = 1
)

type SegmentWriter interface {
	Write(segment EncodedSegment) error
	Flush() error
	Close() error
}

type ShardWal struct {
	corrupted      bool
	shardID        uint16
	encoder        *cppbridge.HeadWalEncoder
	segmentWriter  SegmentWriter
	maxSegmentSize uint32
}

func newShardWal(shardID uint16, encoder *cppbridge.HeadWalEncoder, maxSegmentSize uint32, segmentWriter SegmentWriter) *ShardWal {
	return &ShardWal{
		shardID:        shardID,
		encoder:        encoder,
		segmentWriter:  segmentWriter,
		maxSegmentSize: maxSegmentSize,
	}
}

func newCorruptedShardWal(shardID uint16) *ShardWal {
	return &ShardWal{
		corrupted: true,
		shardID:   shardID,
	}
}

func (w *ShardWal) Write(innerSeriesSlice []*cppbridge.InnerSeries) (bool, error) {
	if w.corrupted {
		return false, fmt.Errorf("writing in corrupted wal")
	}

	stats, err := w.encoder.Encode(innerSeriesSlice)
	if err != nil {
		return false, fmt.Errorf("failed to encode inner series: %w", err)
	}

	if w.maxSegmentSize > 0 && stats.Samples() >= w.maxSegmentSize {
		return true, nil
	}

	return false, nil
}

func (w *ShardWal) Commit() error {
	if w.corrupted {
		return fmt.Errorf("commiting corrupted wal")
	}

	segment, err := w.encoder.Finalize()
	if err != nil {
		return fmt.Errorf("failed to finalize segment: %w", err)
	}

	if err = w.segmentWriter.Write(segment); err != nil {
		return fmt.Errorf("failed to write segment: %w", err)
	}

	if err = w.segmentWriter.Flush(); err != nil {
		return fmt.Errorf("failed to flush segment writer: %w", err)
	}

	return nil
}

func (w *ShardWal) Flush() error {
	return w.segmentWriter.Flush()
}

func (w *ShardWal) Close() error {
	if w.segmentWriter != nil {
		return w.segmentWriter.Close()
	}

	return nil
}

func WriteHeader(writer io.Writer, fileFormatVersion uint8, encoderVersion uint8) (n int, err error) {
	var buf [binary.MaxVarintLen32]byte
	var size int
	var bytesWritten int

	size = binary.PutUvarint(buf[:], uint64(fileFormatVersion))
	bytesWritten, err = writer.Write(buf[:size])
	if err != nil {
		return n, fmt.Errorf("failed to write file format version: %w", err)
	}
	n += bytesWritten

	size = binary.PutUvarint(buf[:], uint64(encoderVersion))
	bytesWritten, err = writer.Write(buf[:size])
	if err != nil {
		return n, fmt.Errorf("failed to write encoder version: %w", err)
	}
	n += bytesWritten

	return n, nil
}

type byteReader struct {
	r io.Reader
	n int
}

func (r *byteReader) ReadByte() (byte, error) {
	b := make([]byte, 1)
	n, err := io.ReadFull(r.r, b)
	if err != nil {
		return 0, err
	}
	r.n += n
	return b[0], nil
}

func ReadHeader(reader io.Reader) (fileFormatVersion uint8, encoderVersion uint8, n int, err error) {
	br := &byteReader{r: reader}
	fileFormatVersionU64, err := binary.ReadUvarint(br)
	if err != nil {
		return 0, 0, n, fmt.Errorf("failed to read file format version: %w", err)
	}
	fileFormatVersion = uint8(fileFormatVersionU64)
	n = br.n

	encoderVersionU64, err := binary.ReadUvarint(br)
	if err != nil {
		return 0, 0, n, fmt.Errorf("failed to read encoder version: %w", err)
	}
	encoderVersion = uint8(encoderVersionU64)
	n = br.n

	return fileFormatVersion, encoderVersion, n, nil
}

type EncodedSegment interface {
	Size() int64
	CRC32() uint32
	io.WriterTo
	cppbridge.SegmentStats
}

func WriteSegment(writer io.Writer, segment EncodedSegment) (n int, err error) {
	var buf [binary.MaxVarintLen32]byte
	var size int
	var bytesWritten int

	segmentSize := uint64(segment.Size())
	size = binary.PutUvarint(buf[:], segmentSize)
	bytesWritten, err = writer.Write(buf[:size])
	if err != nil {
		return n, fmt.Errorf("failed to write segment size: %w", err)
	}
	n += bytesWritten

	size = binary.PutUvarint(buf[:], uint64(segment.CRC32()))
	bytesWritten, err = writer.Write(buf[:size])
	if err != nil {
		return n, fmt.Errorf("failed to write segment crc32 hash: %w", err)
	}
	n += bytesWritten

	size = binary.PutUvarint(buf[:], uint64(segment.Samples()))
	bytesWritten, err = writer.Write(buf[:size])
	if err != nil {
		return n, fmt.Errorf("failed to write segment sample count: %w", err)
	}
	n += bytesWritten

	var bytesWritten64 int64
	bytesWritten64, err = segment.WriteTo(writer)
	if err != nil {
		return n, fmt.Errorf("failed to write segment data: %w", err)
	}
	n += int(bytesWritten64)

	return n, nil
}

type DecodedSegment struct {
	data        []byte
	sampleCount uint32
}

func (d DecodedSegment) Data() []byte {
	return d.data
}

func (d DecodedSegment) SampleCount() uint32 {
	return d.sampleCount
}

func ReadSegment(reader io.Reader) (decodedSegment DecodedSegment, n int, err error) {
	br := &byteReader{r: reader}
	var size uint64
	size, err = binary.ReadUvarint(br)
	if err != nil {
		return decodedSegment, br.n, fmt.Errorf("failed to read segment size: %w", err)
	}

	crc32HashU64, err := binary.ReadUvarint(br)
	if err != nil {
		return decodedSegment, br.n, fmt.Errorf("failed to read segment crc32 hash: %w", err)
	}
	crc32Hash := uint32(crc32HashU64)

	sampleCountU64, err := binary.ReadUvarint(br)
	if err != nil {
		return decodedSegment, br.n, fmt.Errorf("failed to read segment sample count: %w", err)
	}
	decodedSegment.sampleCount = uint32(sampleCountU64)

	decodedSegment.data = make([]byte, size)
	n, err = io.ReadFull(reader, decodedSegment.data)
	if err != nil {
		return decodedSegment, br.n, fmt.Errorf("failed to read segment data: %w", err)
	}
	n += br.n

	if crc32Hash != crc32.ChecksumIEEE(decodedSegment.data) {
		return decodedSegment, n, fmt.Errorf("crc32 did not match, want: %d, have: %d", crc32Hash, crc32.ChecksumIEEE(decodedSegment.data))
	}

	return decodedSegment, n, nil
}
