// Copyright OpCore

package adapter

import (
	"context"
	"net"

	"github.com/prometheus/prometheus/pp-pkg/handler/model"
	"github.com/prometheus/prometheus/util/pool"
)

// Stream wrapper for stream connection.
type Stream struct {
	conn     net.Conn
	buffers  *pool.Pool
	metadata model.Metadata
}

// NewStream init new Stream.
func NewStream(conn net.Conn, buffers *pool.Pool, metadata *model.Metadata) *Stream {
	return &Stream{
		conn:     conn,
		buffers:  buffers,
		metadata: *metadata,
	}
}

// Read segment from connection.
func (s *Stream) Read(_ context.Context) (*model.Segment, error) {
	segment := &model.Segment{}
	return segment, model.NewStreamSegmentDecoder(s.conn, s.buffers).Decode(segment)
}

// Metadata return Metadata.
func (s *Stream) Metadata() model.Metadata {
	return s.metadata
}

// Write response into connection.
func (s *Stream) Write(_ context.Context, status model.StreamSegmentProcessingStatus) error {
	return status.Write(s.conn)
}
