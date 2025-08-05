package wal

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

const (
	// FileFormatVersion wal file version.
	FileFormatVersion = 1
)

// SegmentWriter writer for wal segments.
type SegmentWriter interface {
	// CurrentSize return current shard wal size.
	CurrentSize() int64
	// Write encoded segment to writer.
	Write(segment EncodedSegment) error
	// Flush write all buffered segments.
	Flush() error
	// Close closes the storage.
	Close() error
}

// EncodedSegment encoded segment.
type EncodedSegment interface {
	// Size returns the number of bytes in the segment.
	Size() int64
	// CRC32 checksum for segment.
	CRC32() uint32
	io.WriterTo
	cppbridge.SegmentStats
}

// Wal write-ahead log for [Shard].
type Wal struct {
	encoder        *cppbridge.HeadWalEncoder
	segmentWriter  SegmentWriter
	maxSegmentSize uint32
	corrupted      bool
	limitExhausted bool
}

// NewWal init new [Wal].
func NewWal(encoder *cppbridge.HeadWalEncoder, maxSegmentSize uint32, segmentWriter SegmentWriter) *Wal {
	return &Wal{
		encoder:        encoder,
		segmentWriter:  segmentWriter,
		maxSegmentSize: maxSegmentSize,
	}
}

// NewCorruptedWal init new corrupted [Wal].
func NewCorruptedWal() *Wal {
	return &Wal{
		corrupted: true,
	}
}

// CurrentSize returns current wal size.
func (w *Wal) CurrentSize() int64 {
	return w.segmentWriter.CurrentSize()
}

// Write the incoming inner series to wal encoder.
func (w *Wal) Write(innerSeriesSlice []*cppbridge.InnerSeries) (bool, error) {
	if w.corrupted {
		return false, fmt.Errorf("writing in corrupted wal")
	}

	stats, err := w.encoder.Encode(innerSeriesSlice)
	if err != nil {
		return false, fmt.Errorf("failed to encode inner series: %w", err)
	}

	if w.maxSegmentSize == 0 {
		return false, nil
	}

	// memoize reaching of limits to deduplicate triggers
	if !w.limitExhausted && stats.Samples() >= w.maxSegmentSize {
		w.limitExhausted = true
		return true, nil
	}

	return false, nil
}

// Commit finalize segment from encoder and write to [SegmentWriter].
func (w *Wal) Commit() error {
	if w.corrupted {
		return fmt.Errorf("committing corrupted wal")
	}

	segment, err := w.encoder.Finalize()
	if err != nil {
		return fmt.Errorf("failed to finalize segment: %w", err)
	}
	w.limitExhausted = false

	if err = w.segmentWriter.Write(segment); err != nil {
		return fmt.Errorf("failed to write segment: %w", err)
	}

	if err = w.segmentWriter.Flush(); err != nil {
		return fmt.Errorf("failed to flush segment writer: %w", err)
	}

	return nil
}

// Flush wal [SegmentWriter].
func (w *Wal) Flush() error {
	return w.segmentWriter.Flush()
}

// Close closes the wal segmentWriter.
func (w *Wal) Close() error {
	if w.segmentWriter != nil {
		return w.segmentWriter.Close()
	}

	return nil
}

// WriteHeader write header to writer.
func WriteHeader(writer io.Writer, fileFormatVersion, encoderVersion uint8) (n int, err error) {
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

// ReadHeader read header from reader.
//
//revive:disable-next-line:function-result-limit there is no point in packing it into a structure.
func ReadHeader(reader io.Reader) (fileFormatVersion, encoderVersion uint8, n int, err error) {
	br := &byteReader{r: reader}
	fileFormatVersionU64, err := binary.ReadUvarint(br)
	if err != nil {
		return 0, 0, n, fmt.Errorf("failed to read file format version: %w", err)
	}
	fileFormatVersion = uint8(fileFormatVersionU64) // #nosec G115 // no overflow
	n = br.n

	encoderVersionU64, err := binary.ReadUvarint(br)
	if err != nil {
		return 0, 0, n, fmt.Errorf("failed to read encoder version: %w", err)
	}
	encoderVersion = uint8(encoderVersionU64) // #nosec G115 // no overflow
	n = br.n

	return fileFormatVersion, encoderVersion, n, nil
}

// WriteSegment write encoded segment to writer.
func WriteSegment(writer io.Writer, segment EncodedSegment) (n int, err error) {
	var buf [binary.MaxVarintLen32]byte
	var size int
	var bytesWritten int

	segmentSize := uint64(segment.Size()) // #nosec G115 // no overflow
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
	crc32Hash := uint32(crc32HashU64) // #nosec G115 // no overflow

	sampleCountU64, err := binary.ReadUvarint(br)
	if err != nil {
		return decodedSegment, br.n, fmt.Errorf("failed to read segment sample count: %w", err)
	}
	decodedSegment.sampleCount = uint32(sampleCountU64) // #nosec G115 // no overflow

	decodedSegment.data = make([]byte, size)
	n, err = io.ReadFull(reader, decodedSegment.data)
	if err != nil {
		return decodedSegment, br.n, fmt.Errorf("failed to read segment data: %w", err)
	}
	n += br.n

	if crc32Hash != crc32.ChecksumIEEE(decodedSegment.data) {
		return decodedSegment, n, fmt.Errorf(
			"crc32 did not match, want: %d, have: %d", crc32Hash, crc32.ChecksumIEEE(decodedSegment.data),
		)
	}

	return decodedSegment, n, nil
}
