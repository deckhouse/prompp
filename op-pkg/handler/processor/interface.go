package processor

import (
	"context"

	"github.com/odarix/odarix-core-go/cppbridge"

	"github.com/prometheus/prometheus/op-pkg/handler/decoder"
	"github.com/prometheus/prometheus/op-pkg/handler/model"
)

type MetricStream interface {
	Metadata() model.Metadata
	Read(ctx context.Context) (model.Segment, error)
	Write(ctx context.Context, status model.SegmentProcessingStatus) error
}

type Refill interface {
	Metadata() model.Metadata
	Read(ctx context.Context) (model.Segment, error)
}

type DecoderBuilder interface {
	Build(metadata model.Metadata) decoder.Decoder
}

type Receiver interface {
	AppendHashdex(ctx context.Context, hashdex cppbridge.ShardedData, relabelerID string) error
}
