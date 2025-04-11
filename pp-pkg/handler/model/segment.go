package model

import (
	"errors"
	"hash/crc32"
)

const (
	// ProcessingStatusOk is ok status.
	ProcessingStatusOk uint16 = 200
	// ProcessingStatusRejected is reject status.
	ProcessingStatusRejected uint16 = 400
)

// ErrCorruptedSegment error when segment is corrupted.
var ErrCorruptedSegment = errors.New("corrupted segment")

//
// Segment
//

// Segment handle segment data.
type Segment struct {
	Timestamp int64 // sentAt
	ID        uint32
	Size      uint32
	CRC       uint32
	Body      []byte
	DestroyFn func()
}

// IsValid check segment body on crc.
func (s Segment) IsValid() bool {
	return crc32.ChecksumIEEE(s.Body) == s.CRC
}

// Destroy body segment if exist DestroyFn, return to pool.
func (s *Segment) Destroy() {
	if s.DestroyFn != nil {
		s.DestroyFn()
	}
}

// ResizeBuffer resize slice and fill zero value.
func ResizeBuffer(size int, buf *[]byte) {
	if cap(*buf) < size {
		*buf = append(*buf, make([]byte, size)...)
	}

	*buf = (*buf)[:size]
	(*buf)[0] = 0

	for i := 1; i < len(*buf); i *= 2 {
		copy((*buf)[i:], (*buf)[:i])
	}
}
