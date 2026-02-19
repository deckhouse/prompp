package writer

import (
	"encoding/binary"
	"fmt"
	"io"
)

//go:generate -command moq go run github.com/matryer/moq --rm --skip-ensure --pkg writer_test --out
//go:generate moq segment_moq_test.go . EncodedSegment EncodedSegmentV2

// EncodedSegment the minimum required Segment implementation for a [WriteSegment].
type EncodedSegment interface {
	CRC32() uint32
	Samples() uint32
	Size() int64
	io.WriterTo
}

// WriteSegment encode [EncodedSegment] to slice byte and write to [io.Writer].
func WriteSegment[TSegment EncodedSegment](writer io.Writer, segment TSegment) (n int, err error) {
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

//
// EncodedSegmentV2
//

// EncodedSegmentV2 the minimum required Segment implementation for a [WriteSegment], version 2.
type EncodedSegmentV2 interface {
	// ID returns the segment ID, filled in from the outside.
	ID() uint32

	// SetSegmentID sets the segment ID.
	SetSegmentID(sid uint32)

	// CRC32 the hash amount according to the data.
	CRC32() uint32

	// Samples returns count of samples in segment.
	Samples() uint32

	// Size returns len of bytes.
	Size() int64

	// WriteTo writes data to w until there's no more data to write or when an error occurs.
	// The return value n is the number of bytes written. Any error encountered during the write is also returned.
	io.WriterTo
}

//
// WriteSegmentV2
//

// WriteSegmentV2 encode [EncodedSegmentV2] to slice byte and write to [io.Writer].
func WriteSegmentV2[TSegment EncodedSegmentV2](writer io.Writer, segment TSegment) (n int, err error) {
	var buf [binary.MaxVarintLen32]byte
	var size int
	var bytesWritten int

	size = binary.PutUvarint(buf[:], uint64(segment.ID()))
	bytesWritten, err = writer.Write(buf[:size])
	if err != nil {
		return n, fmt.Errorf("v2: failed to write segment id: %w", err)
	}
	n += bytesWritten

	segmentSize := uint64(segment.Size()) // #nosec G115 // no overflow
	size = binary.PutUvarint(buf[:], segmentSize)
	bytesWritten, err = writer.Write(buf[:size])
	if err != nil {
		return n, fmt.Errorf("v2: failed to write segment size: %w", err)
	}
	n += bytesWritten

	size = binary.PutUvarint(buf[:], uint64(segment.CRC32()))
	bytesWritten, err = writer.Write(buf[:size])
	if err != nil {
		return n, fmt.Errorf("v2: failed to write segment crc32 hash: %w", err)
	}
	n += bytesWritten

	size = binary.PutUvarint(buf[:], uint64(segment.Samples()))
	bytesWritten, err = writer.Write(buf[:size])
	if err != nil {
		return n, fmt.Errorf("v2: failed to write segment sample count: %w", err)
	}
	n += bytesWritten

	var bytesWritten64 int64
	bytesWritten64, err = segment.WriteTo(writer)
	if err != nil {
		return n, fmt.Errorf("v2: failed to write segment data: %w", err)
	}
	n += int(bytesWritten64)

	return n, nil
}
