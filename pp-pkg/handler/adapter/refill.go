// Copyright OpCore

package adapter

import (
	"context"
	"encoding/binary"
	"io"
	"net/http"

	"github.com/prometheus/prometheus/pp-pkg/handler/model"
	"github.com/prometheus/prometheus/util/pool"
)

// headerRefillSize header size for refill.
const headerRefillSize = 4 + 4 + 4

// Refill wrapper for refill reader.
type Refill struct {
	reader   io.Reader
	writer   http.ResponseWriter
	buffers  *pool.Pool
	metadata model.Metadata
}

// NewRefill init new Refill.
func NewRefill(reader io.Reader, writer http.ResponseWriter, buffers *pool.Pool, metadata *model.Metadata) *Refill {
	return &Refill{
		reader:   reader,
		writer:   writer,
		buffers:  buffers,
		metadata: *metadata,
	}
}

// Metadata return Metadata.
func (r *Refill) Metadata() model.Metadata {
	return r.metadata
}

// Read read from reader Segment and return him.
func (r *Refill) Read(_ context.Context) (*model.Segment, error) {
	return r.decode()
}

// Write response into writer.
func (r *Refill) Write(_ context.Context, status model.RefillProcessingStatus) error {
	return status.Write(r.writer)
}

// decode read from refill and decode.
func (r *Refill) decode() (*model.Segment, error) {
	header := r.buffers.Get(headerRefillSize).([]byte)
	model.ResizeBuffer(headerRefillSize, &header)

	if _, err := io.ReadFull(r.reader, header); err != nil {
		r.buffers.Put(header)
		return nil, err
	}

	segment := &model.Segment{}
	segment.ID = binary.LittleEndian.Uint32(header[:4])
	segment.Size = binary.LittleEndian.Uint32(header[4:8])
	segment.CRC = binary.LittleEndian.Uint32(header[8:12])
	r.buffers.Put(header)

	if segment.Size == 0 {
		return segment, nil
	}

	segment.Body = r.buffers.Get(int(segment.Size)).([]byte)
	model.ResizeBuffer(int(segment.Size), &segment.Body)
	if _, err := io.ReadFull(r.reader, segment.Body); err != nil {
		r.buffers.Put(segment.Body)
		return nil, err
	}

	if !segment.IsValid() {
		r.buffers.Put(segment.Body)
		return nil, model.ErrCorruptedSegment
	}

	segment.DestroyFn = func() { r.buffers.Put(segment.Body) }

	return segment, nil
}

//
// EncodeToRefill
//

func EncodeToRefill(refill io.Writer, segment model.Segment) error {
	buf := make([]byte, headerRefillSize+len(segment.Body))
	binary.LittleEndian.PutUint32(buf[:4], segment.ID)
	binary.LittleEndian.PutUint32(buf[4:8], segment.Size)
	binary.LittleEndian.PutUint32(buf[8:12], segment.CRC)
	copy(buf[12:], segment.Body)

	_, err := refill.Write(buf)
	return err
}
