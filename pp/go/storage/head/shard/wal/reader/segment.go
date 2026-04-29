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
	br := NewByteReader(r)
	size, err := binary.ReadUvarint(br)
	if err != nil {
		return int64(br.readBytes), fmt.Errorf("failed to read segment size: %w", err)
	}

	crc32HashU64, err := binary.ReadUvarint(br)
	if err != nil {
		return int64(br.readBytes), fmt.Errorf("failed to read segment crc32 hash: %w", err)
	}
	crc32Hash := uint32(crc32HashU64) // #nosec G115 // no overflow

	sampleCountU64, err := binary.ReadUvarint(br)
	if err != nil {
		return int64(br.readBytes), fmt.Errorf("failed to read segment sample count: %w", err)
	}
	s.sampleCount = uint32(sampleCountU64) // #nosec G115 // no overflow

	s.resize(int(size)) // #nosec G115 // no overflow
	n, err := io.ReadFull(r, s.data)
	if err != nil {
		return int64(br.readBytes), fmt.Errorf("failed to read segment data: %w", err)
	}
	n += br.readBytes

	if crc32Hash != crc32.ChecksumIEEE(s.data) {
		return int64(n), fmt.Errorf(
			"crc32 did not match, want: %d, have: %d", crc32Hash, crc32.ChecksumIEEE(s.data),
		)
	}

	return int64(n), nil
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

//
// SegmentV2
//

// UnknownSegmentID is the unknown ID of a segment.
const UnknownSegmentID = uint32(math.MaxUint32)

// SegmentV2 encoded segment from wal.
type SegmentV2 struct {
	Segment
	id uint32
}

// NewSegmentV2 init new [SegmentV2].
func NewSegmentV2() *SegmentV2 {
	return &SegmentV2{id: UnknownSegmentID}
}

// ID returns [SegmentV2] ID.
func (s *SegmentV2) ID() uint32 {
	return s.id
}

// ReadBody reads [SegmentV2] body from r [io.Reader]. The return value n is the number of bytes read.
// Any error encountered during the read is also returned.
func (s *SegmentV2) ReadBody(r io.Reader) (int64, error) {
	return s.readSegmentBody(NewByteReader(r))
}

// ReadFrom reads [SegmentV2] data from r [io.Reader]. The return value n is the number of bytes read.
// Any error encountered during the read is also returned.
func (s *SegmentV2) ReadFrom(r io.Reader) (int64, error) {
	br := NewByteReader(r)

	if n, err := s.readSegmentID(br); err != nil {
		return n, err
	}

	return s.readSegmentBody(br)
}

// ReadID reads [SegmentV2] ID from r [io.Reader]. The return value n is the number of bytes read.
// Any error encountered during the read is also returned.
func (s *SegmentV2) ReadID(r io.Reader) (int64, error) {
	return s.readSegmentID(NewByteReader(r))
}

// Reset [SegmentV2] data.
func (s *SegmentV2) Reset() {
	s.data = s.data[:0]
	s.sampleCount = 0
	s.id = UnknownSegmentID
}

// readSegmentID read and decode segment id from [ByteReader] and set to [SegmentV2].
// The return value n is the number of bytes read. Any error encountered during the read is also returned.
func (s *SegmentV2) readSegmentID(br *ByteReader) (int64, error) {
	idU64, err := binary.ReadUvarint(br)
	if err != nil {
		return int64(br.readBytes), fmt.Errorf("v2: failed to read segment id: %w", err)
	}
	s.id = uint32(idU64) // #nosec G115 // no overflow

	return int64(br.readBytes), nil
}

// readSegmentBody read and decode segment body from [ByteReader] and set to [SegmentV2].
// The return value n is the number of bytes read. Any error encountered during the read is also returned.
func (s *SegmentV2) readSegmentBody(br *ByteReader) (int64, error) {
	size, err := binary.ReadUvarint(br)
	if err != nil {
		return int64(br.readBytes), fmt.Errorf("v2: failed to read segment size: %w", err)
	}

	crc32HashU64, err := binary.ReadUvarint(br)
	if err != nil {
		return int64(br.readBytes), fmt.Errorf("v2: failed to read segment crc32 hash: %w", err)
	}
	crc32Hash := uint32(crc32HashU64) // #nosec G115 // no overflow

	sampleCountU64, err := binary.ReadUvarint(br)
	if err != nil {
		return int64(br.readBytes), fmt.Errorf("v2: failed to read segment sample count: %w", err)
	}
	s.sampleCount = uint32(sampleCountU64) // #nosec G115 // no overflow

	s.resize(int(size)) // #nosec G115 // no overflow
	n, err := io.ReadFull(br, s.data)
	if err != nil {
		return int64(br.readBytes), fmt.Errorf("v2: failed to read segment data: %w", err)
	}
	n += br.readBytes

	if crc32Hash != crc32.ChecksumIEEE(s.data) {
		return int64(n), fmt.Errorf(
			"v2: crc32 did not match, want: %d, have: %d", crc32Hash, crc32.ChecksumIEEE(s.data),
		)
	}

	return int64(n), nil
}
