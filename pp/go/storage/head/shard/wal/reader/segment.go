package reader

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
)

// TODO rebuild ReadFrom

// Segment encoded segment from wal.
type Segment struct {
	data        []byte
	sampleCount uint32
}

// Bytes returns the data read.
func (s *Segment) Bytes() []byte {
	return s.data
}

// Reset segment data.
func (s *Segment) Reset() {
	s.data = s.data[:0]
	s.sampleCount = 0
}

// Samples returns count of samples in segment.
func (s *Segment) Samples() uint32 {
	return s.sampleCount
}

// resize segment data.
func (s *Segment) resize(size int) {
	if cap(s.data) < size {
		s.data = make([]byte, size)
	} else {
		s.data = s.data[:size]
	}
}

// ReadSegment read and decode [Segment] from [io.Reader] and returns.
func ReadSegment(reader io.Reader, segment *Segment) (n int, err error) {
	br := newByteReader(reader)
	var size uint64
	size, err = binary.ReadUvarint(br)
	if err != nil {
		return br.n, fmt.Errorf("failed to read segment size: %w", err)
	}

	crc32HashU64, err := binary.ReadUvarint(br)
	if err != nil {
		return br.n, fmt.Errorf("failed to read segment crc32 hash: %w", err)
	}
	crc32Hash := uint32(crc32HashU64) // #nosec G115 // no overflow

	sampleCountU64, err := binary.ReadUvarint(br)
	if err != nil {
		return br.n, fmt.Errorf("failed to read segment sample count: %w", err)
	}
	segment.sampleCount = uint32(sampleCountU64) // #nosec G115 // no overflow

	segment.resize(int(size)) // #nosec G115 // no overflow
	n, err = io.ReadFull(reader, segment.data)
	if err != nil {
		return br.n, fmt.Errorf("failed to read segment data: %w", err)
	}
	n += br.n

	if crc32Hash != crc32.ChecksumIEEE(segment.data) {
		return n, fmt.Errorf(
			"crc32 did not match, want: %d, have: %d", crc32Hash, crc32.ChecksumIEEE(segment.data),
		)
	}

	return n, nil
}
