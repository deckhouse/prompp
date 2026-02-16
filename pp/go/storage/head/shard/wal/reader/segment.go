package reader

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"math"
)

// Segment encoded segment from wal.
type Segment struct {
	data        []byte
	sampleCount uint32
}

// NewSegment init new [Segment].
func NewSegment() *Segment {
	return &Segment{}
}

// Bytes returns the data read.
func (s *Segment) Bytes() []byte {
	return s.data
}

// Length returns the length of slice byte.
func (s *Segment) Length() int {
	return len(s.data)
}

// ReadFrom reads [Segment] data from r [io.Reader]. The return value n is the number of bytes read.
// Any error encountered during the read is also returned.
func (s *Segment) ReadFrom(r io.Reader) (int64, error) {
	return readSegment(r, s)
}

// Reset [Segment] data.
func (s *Segment) Reset() {
	s.data = s.data[:0]
	s.sampleCount = 0
}

// Samples returns count of samples in [Segment].
func (s *Segment) Samples() uint32 {
	return s.sampleCount
}

// resize [Segment] data.
func (s *Segment) resize(size int) {
	if cap(s.data) < size {
		s.data = make([]byte, size)
	} else {
		s.data = s.data[:size]
	}
}

// readSegment read and decode [Segment] from [io.Reader] and returns.
func readSegment(reader io.Reader, segment *Segment) (int64, error) {
	br := NewByteReader(reader)
	size, err := binary.ReadUvarint(br)
	if err != nil {
		return int64(br.n), fmt.Errorf("failed to read segment size: %w", err)
	}

	crc32HashU64, err := binary.ReadUvarint(br)
	if err != nil {
		return int64(br.n), fmt.Errorf("failed to read segment crc32 hash: %w", err)
	}
	crc32Hash := uint32(crc32HashU64) // #nosec G115 // no overflow

	sampleCountU64, err := binary.ReadUvarint(br)
	if err != nil {
		return int64(br.n), fmt.Errorf("failed to read segment sample count: %w", err)
	}
	segment.sampleCount = uint32(sampleCountU64) // #nosec G115 // no overflow

	segment.resize(int(size)) // #nosec G115 // no overflow
	n, err := io.ReadFull(reader, segment.data)
	if err != nil {
		return int64(br.n), fmt.Errorf("failed to read segment data: %w", err)
	}
	n += br.n

	if crc32Hash != crc32.ChecksumIEEE(segment.data) {
		return int64(n), fmt.Errorf(
			"crc32 did not match, want: %d, have: %d", crc32Hash, crc32.ChecksumIEEE(segment.data),
		)
	}

	return int64(n), nil
}

//
// SegmentV2
//

// SegmentV2 encoded segment from wal.
type SegmentV2 struct {
	Segment
	id uint32
}

// NewSegmentV2 init new [SegmentV2].
func NewSegmentV2() *SegmentV2 {
	return &SegmentV2{}
}

// ID returns [SegmentV2] ID.
func (s *SegmentV2) ID() uint32 {
	return s.id
}

// ReadFrom reads [SegmentV2] data from r [io.Reader]. The return value n is the number of bytes read.
// Any error encountered during the read is also returned.
func (s *SegmentV2) ReadFrom(r io.Reader) (int64, error) {
	return readSegmentV2(r, s)
}

// Reset [SegmentV2] data.
func (s *SegmentV2) Reset() {
	s.data = s.data[:0]
	s.sampleCount = 0
	s.id = math.MaxUint32
}

// readSegmentV2 read and decode [SegmentV2] from [io.Reader] and returns.
func readSegmentV2(reader io.Reader, segment *SegmentV2) (int64, error) {
	br := NewByteReader(reader)

	idU64, err := binary.ReadUvarint(br)
	if err != nil {
		return int64(br.n), fmt.Errorf("failed to read segment id: %w", err)
	}
	segment.id = uint32(idU64) // #nosec G115 // no overflow

	size, err := binary.ReadUvarint(br)
	if err != nil {
		return int64(br.n), fmt.Errorf("failed to read segment size: %w", err)
	}

	crc32HashU64, err := binary.ReadUvarint(br)
	if err != nil {
		return int64(br.n), fmt.Errorf("failed to read segment crc32 hash: %w", err)
	}
	crc32Hash := uint32(crc32HashU64) // #nosec G115 // no overflow

	sampleCountU64, err := binary.ReadUvarint(br)
	if err != nil {
		return int64(br.n), fmt.Errorf("failed to read segment sample count: %w", err)
	}
	segment.sampleCount = uint32(sampleCountU64) // #nosec G115 // no overflow

	segment.resize(int(size)) // #nosec G115 // no overflow
	n, err := io.ReadFull(reader, segment.data)
	if err != nil {
		return int64(br.n), fmt.Errorf("failed to read segment data: %w", err)
	}
	n += br.n

	if crc32Hash != crc32.ChecksumIEEE(segment.data) {
		return int64(n), fmt.Errorf(
			"crc32 did not match, want: %d, have: %d", crc32Hash, crc32.ChecksumIEEE(segment.data),
		)
	}

	return int64(n), nil
}
