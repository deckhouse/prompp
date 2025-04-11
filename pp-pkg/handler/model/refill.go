package model

import (
	"encoding/binary"
	"io"
	"net/http"

	"github.com/prometheus/prometheus/util/pool"
)

// headerRefillSize header size for refill.
const headerRefillSize = 4 + 4 + 4

//
// RefillSegmentEncoder
//

type RefillSegmentEncoder struct {
	writer io.Writer
}

func NewRefillSegmentEncoder(writer io.Writer) *RefillSegmentEncoder {
	return &RefillSegmentEncoder{writer: writer}
}

func (e *RefillSegmentEncoder) Encode(segment Segment) (err error) {
	buf := make([]byte, headerRefillSize+len(segment.Body))
	binary.LittleEndian.PutUint32(buf[:4], segment.ID)
	binary.LittleEndian.PutUint32(buf[4:8], segment.Size)
	binary.LittleEndian.PutUint32(buf[8:12], segment.CRC)
	copy(buf[20:], segment.Body)

	_, err = e.writer.Write(buf)
	return err
}

//
// RefillSegmentDecoder
//

type RefillSegmentDecoder struct {
	reader  io.Reader
	buffers *pool.Pool
}

func NewRefillSegmentDecoder(reader io.Reader, buffers *pool.Pool) *RefillSegmentDecoder {
	return &RefillSegmentDecoder{reader: reader, buffers: buffers}
}

func (d *RefillSegmentDecoder) Decode(segment *Segment) error {
	header := d.buffers.Get(headerRefillSize).([]byte)
	ResizeBuffer(headerRefillSize, &header)

	if _, err := io.ReadFull(d.reader, header); err != nil {
		d.buffers.Put(header)
		return err
	}

	segment.ID = binary.LittleEndian.Uint32(header[:4])
	segment.Size = binary.LittleEndian.Uint32(header[4:8])
	segment.CRC = binary.LittleEndian.Uint32(header[8:12])
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

// RefillProcessingStatus status of processing refill.
type RefillProcessingStatus struct {
	Code    int
	Message string
}

// Write to writer RefillProcessingStatus.
func (s *RefillProcessingStatus) Write(writer http.ResponseWriter) error {
	writer.WriteHeader(s.Code)
	_, err := writer.Write([]byte(s.Message))
	return err
}
