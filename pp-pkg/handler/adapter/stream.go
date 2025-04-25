// Copyright OpCore

package adapter

import (
	"context"
	"encoding/binary"
	"io"

	"github.com/prometheus/prometheus/pp-pkg/handler/model"
	"github.com/prometheus/prometheus/util/pool"
)

// headerStreamSize header size for stream.
const headerStreamSize = 8 + 4 + 4 + 4

// Stream wrapper for stream connection.
type Stream struct {
	stream   io.ReadWriter
	buffers  *pool.Pool
	metadata model.Metadata
}

// NewStream init new Stream.
func NewStream(stream io.ReadWriter, buffers *pool.Pool, metadata *model.Metadata) *Stream {
	return &Stream{
		stream:   stream,
		buffers:  buffers,
		metadata: *metadata,
	}
}

// Read segment from connection.
func (s *Stream) Read(_ context.Context) (*model.Segment, error) {
	return s.decode()
}

// Metadata return Metadata.
func (s *Stream) Metadata() model.Metadata {
	return s.metadata
}

// Write response into connection.
func (s *Stream) Write(_ context.Context, status model.StreamSegmentProcessingStatus) error {
	return status.EncodeTo(s.stream)
}

// decode read from stream and decode.
func (s *Stream) decode() (*model.Segment, error) {
	header := s.buffers.Get(headerStreamSize).([]byte)
	model.ResizeBuffer(headerStreamSize, &header)

	if _, err := io.ReadFull(s.stream, header); err != nil {
		s.buffers.Put(header)
		return nil, err
	}

	segment := &model.Segment{}
	segment.Timestamp = int64(binary.LittleEndian.Uint64(header[:8]))
	segment.ID = binary.LittleEndian.Uint32(header[8:12])
	segment.Size = binary.LittleEndian.Uint32(header[12:16])
	segment.CRC = binary.LittleEndian.Uint32(header[16:20])
	s.buffers.Put(header)

	if segment.Size == 0 {
		return segment, nil
	}

	segment.Body = s.buffers.Get(int(segment.Size)).([]byte)
	model.ResizeBuffer(int(segment.Size), &segment.Body)
	if _, err := io.ReadFull(s.stream, segment.Body); err != nil {
		s.buffers.Put(segment.Body)
		return nil, err
	}

	if !segment.IsValid() {
		s.buffers.Put(segment.Body)
		return nil, model.ErrCorruptedSegment
	}

	segment.DestroyFn = func() { s.buffers.Put(segment.Body) }

	return segment, nil
}

//
// EncodeToStream
//

func EncodeToStream(stream io.Writer, segment model.Segment) error {
	buf := make([]byte, headerStreamSize+len(segment.Body))
	binary.LittleEndian.PutUint64(buf[:8], uint64(segment.Timestamp))
	binary.LittleEndian.PutUint32(buf[8:12], segment.ID)
	binary.LittleEndian.PutUint32(buf[12:16], segment.Size)
	binary.LittleEndian.PutUint32(buf[16:20], segment.CRC)
	copy(buf[20:], segment.Body)
	_, err := stream.Write(buf)
	return err
}
