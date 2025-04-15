package model

import (
	"encoding/binary"
	"io"

	"github.com/prometheus/prometheus/util/pool"
)

// headerStreamSize header size for stream.
const headerStreamSize = 8 + 4 + 4 + 4

//
// SegmentEncoder
//

type SegmentEncoder struct {
	writer io.Writer
}

func NewSegmentEncoder(writer io.Writer) *SegmentEncoder {
	return &SegmentEncoder{writer: writer}
}

func (e *SegmentEncoder) Encode(segment Segment) (err error) {
	buf := make([]byte, headerStreamSize+len(segment.Body))
	binary.LittleEndian.PutUint64(buf[:8], uint64(segment.Timestamp))
	binary.LittleEndian.PutUint32(buf[8:12], segment.ID)
	binary.LittleEndian.PutUint32(buf[12:16], segment.Size)
	binary.LittleEndian.PutUint32(buf[16:20], segment.CRC)
	copy(buf[20:], segment.Body)
	_, err = e.writer.Write(buf)
	return err
}

//
// StreamSegmentDecoder
//

type StreamSegmentDecoder struct {
	reader  io.Reader
	buffers *pool.Pool
}

func NewStreamSegmentDecoder(reader io.Reader, buffers *pool.Pool) *StreamSegmentDecoder {
	return &StreamSegmentDecoder{reader: reader, buffers: buffers}
}

func (d *StreamSegmentDecoder) Decode(segment *Segment) error {
	header := d.buffers.Get(headerStreamSize).([]byte)
	ResizeBuffer(headerStreamSize, &header)

	if _, err := io.ReadFull(d.reader, header); err != nil {
		d.buffers.Put(header)
		return err
	}

	segment.Timestamp = int64(binary.LittleEndian.Uint64(header[:8]))
	segment.ID = binary.LittleEndian.Uint32(header[8:12])
	segment.Size = binary.LittleEndian.Uint32(header[12:16])
	segment.CRC = binary.LittleEndian.Uint32(header[16:20])
	d.buffers.Put(header)

	if segment.Size == 0 {
		return nil
	}

	segment.Body = d.buffers.Get(int(segment.Size)).([]byte)
	ResizeBuffer(int(segment.Size), &segment.Body)
	if _, err := io.ReadFull(d.reader, segment.Body); err != nil {
		d.buffers.Put(segment.Body)
		return err
	}

	if !segment.IsValid() {
		d.buffers.Put(segment.Body)
		return ErrCorruptedSegment
	}

	segment.DestroyFn = func() { d.buffers.Put(segment.Body) }

	return nil
}

//
// StreamSegmentProcessingStatus
//

// StreamSegmentProcessingStatus status of processing segment.
type StreamSegmentProcessingStatus struct {
	SegmentID uint32
	Code      uint16
	Message   string
	Timestamp int64
}

// Encode status to slice byte.
func (s *StreamSegmentProcessingStatus) Encode() []byte {
	buf := make([]byte, 8+4+2+4+len(s.Message))
	binary.LittleEndian.PutUint64(buf, uint64(s.Timestamp))
	binary.LittleEndian.PutUint32(buf[8:], s.SegmentID)
	binary.LittleEndian.PutUint16(buf[12:], s.Code)
	binary.LittleEndian.PutUint32(buf[14:], uint32(len(s.Message)))
	copy(buf[18:], s.Message)
	return buf
}

// Write to writer RefillProcessingStatus.
func (s *StreamSegmentProcessingStatus) Write(writer io.Writer) error {
	_, err := writer.Write(s.Encode())
	return err
}

// DecodeFrom read from reader and decode.
func (s *StreamSegmentProcessingStatus) DecodeFrom(reader io.Reader) error {
	header := make([]byte, 8+4+2+4)
	if _, err := io.ReadFull(reader, header); err != nil {
		return err
	}

	s.Timestamp = int64(binary.LittleEndian.Uint64(header[:8]))
	s.SegmentID = binary.LittleEndian.Uint32(header[8:12])
	s.Code = binary.LittleEndian.Uint16(header[12:14])
	messageLen := binary.LittleEndian.Uint32(header[14:18])
	if messageLen == 0 {
		return nil
	}

	message := make([]byte, messageLen)
	if _, err := io.ReadFull(reader, message); err != nil {
		return err
	}

	s.Message = string(message)

	return nil
}
