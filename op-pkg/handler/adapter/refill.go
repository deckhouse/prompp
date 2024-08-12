package adapter

import (
	"context"
	"io"

	"github.com/prometheus/prometheus/op-pkg/handler/model"
)

type Refill struct {
	reader   io.Reader
	metadata model.Metadata
}

func NewRefill(reader io.Reader, metadata model.Metadata) *Refill {
	return &Refill{
		reader:   reader,
		metadata: metadata,
	}
}

func (r *Refill) Metadata() model.Metadata {
	return r.metadata
}

func (r *Refill) Read(_ context.Context) (segment model.Segment, err error) {
	return segment, model.NewRefillSegmentDecoder(r.reader).Decode(&segment)
}
