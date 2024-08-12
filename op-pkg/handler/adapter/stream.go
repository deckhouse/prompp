package adapter

import (
	"context"
	"net"

	"github.com/prometheus/prometheus/op-pkg/handler/model"
)

type Stream struct {
	conn     net.Conn
	metadata model.Metadata
}

func (s *Stream) Read(_ context.Context) (segment model.Segment, err error) {
	return segment, model.NewSegmentDecoder(s.conn).Decode(&segment)
}

func NewStream(conn net.Conn, metadata model.Metadata) *Stream {
	return &Stream{
		conn:     conn,
		metadata: metadata,
	}
}

func (s *Stream) Metadata() model.Metadata {
	return s.metadata
}

func (s *Stream) Write(_ context.Context, status model.SegmentProcessingStatus) error {
	return model.NewSegmentProcessingStatusEncoder(s.conn).Encode(status)
}
