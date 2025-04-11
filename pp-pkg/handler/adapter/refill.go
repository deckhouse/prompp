// Copyright OpCore

package adapter

import (
	"context"
	"io"
	"net/http"

	"github.com/prometheus/prometheus/pp-pkg/handler/model"
	"github.com/prometheus/prometheus/util/pool"
)

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
	segment := &model.Segment{}
	return segment, model.NewRefillSegmentDecoder(r.reader, r.buffers).Decode(segment)
}

// Write response into writer.
func (r *Refill) Write(_ context.Context, status model.RefillProcessingStatus) error {
	return status.Write(r.writer)
}
