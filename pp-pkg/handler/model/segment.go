package model

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"

	"github.com/prometheus/prometheus/util/pool"
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

// DecodeFromStream read from stream and decode.
func (s *Segment) DecodeFromStream(stream io.Reader, buffers *pool.Pool) error {
	header := buffers.Get(headerStreamSize).([]byte)
	ResizeBuffer(headerStreamSize, &header)

	if _, err := io.ReadFull(stream, header); err != nil {
		buffers.Put(header)
		return err
	}

	s.Timestamp = int64(binary.LittleEndian.Uint64(header[:8]))
	s.ID = binary.LittleEndian.Uint32(header[8:12])
	s.Size = binary.LittleEndian.Uint32(header[12:16])
	s.CRC = binary.LittleEndian.Uint32(header[16:20])
	buffers.Put(header)

	if s.Size == 0 {
		return nil
	}

	s.Body = buffers.Get(int(s.Size)).([]byte)
	ResizeBuffer(int(s.Size), &s.Body)
	if _, err := io.ReadFull(stream, s.Body); err != nil {
		buffers.Put(s.Body)
		return err
	}

	if !s.IsValid() {
		buffers.Put(s.Body)
		return ErrCorruptedSegment
	}

	s.DestroyFn = func() { buffers.Put(s.Body) }

	return nil
}

// DecodeFromRefill read from refill and decode.
func (s *Segment) DecodeFromRefill(refill io.Reader, buffers *pool.Pool) error {
	header := buffers.Get(headerRefillSize).([]byte)
	ResizeBuffer(headerRefillSize, &header)

	if _, err := io.ReadFull(refill, header); err != nil {
		buffers.Put(header)
		return err
	}

	s.ID = binary.LittleEndian.Uint32(header[:4])
	s.Size = binary.LittleEndian.Uint32(header[4:8])
	s.CRC = binary.LittleEndian.Uint32(header[8:12])
	buffers.Put(header)

	if s.Size == 0 {
		return nil
	}

	s.Body = buffers.Get(int(s.Size)).([]byte)
	ResizeBuffer(int(s.Size), &s.Body)
	if _, err := io.ReadFull(refill, s.Body); err != nil {
		buffers.Put(s.Body)
		return err
	}

	if !s.IsValid() {
		buffers.Put(s.Body)
		return ErrCorruptedSegment
	}

	s.DestroyFn = func() { buffers.Put(s.Body) }

	return nil
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
