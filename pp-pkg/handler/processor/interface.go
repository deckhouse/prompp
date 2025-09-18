package processor

import (
	"context"

	"github.com/prometheus/prometheus/pp-pkg/handler/decoder"
	"github.com/prometheus/prometheus/pp-pkg/handler/model"
	pp_pkg_model "github.com/prometheus/prometheus/pp-pkg/model"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

type MetricStream interface {
	Metadata() model.Metadata
	Read(ctx context.Context) (*model.Segment, error)
	Write(ctx context.Context, status model.StreamSegmentProcessingStatus) error
}

type Refill interface {
	Metadata() model.Metadata
	Read(ctx context.Context) (*model.Segment, error)
	Write(ctx context.Context, status model.RefillProcessingStatus) error
}

type RemoteWrite interface {
	Metadata() model.Metadata
	Read(ctx context.Context) (*model.RemoteWriteBuffer, error)
	Write(ctx context.Context, status model.RemoteWriteProcessingStatus) error
}

type DecoderBuilder interface {
	Build(metadata model.Metadata) decoder.Decoder
}

// Adapter for implementing the [Queryable] interface and append data.
type Adapter interface {
	// AppendHashdex append incoming [cppbridge.HashdexContent] to [Head].
	AppendHashdex(
		ctx context.Context,
		hashdex cppbridge.ShardedData,
		state *cppbridge.StateV2,
		commitToWal bool,
	) error

	// AppendTimeSeries append TimeSeries data to [Head].
	AppendTimeSeries(
		ctx context.Context,
		data pp_pkg_model.TimeSeriesBatch,
		state *cppbridge.StateV2,
		commitToWal bool,
	) (cppbridge.RelabelerStats, error)

	// AppendScraperHashdex append ScraperHashdex data to [Head].
	AppendScraperHashdex(
		ctx context.Context,
		hashdex cppbridge.ShardedData,
		state *cppbridge.StateV2,
		commitToWal bool,
	) (cppbridge.RelabelerStats, error)

	// AppendSnappyProtobuf append compressed via snappy Protobuf data to [Head].
	AppendSnappyProtobuf(
		ctx context.Context,
		compressedData pp_pkg_model.ProtobufData,
		state *cppbridge.StateV2,
		commitToWal bool,
	) error

	// MergeOutOfOrderChunks merge chunks with out of order data chunks.
	MergeOutOfOrderChunks(ctx context.Context)
}

// StatesStorage stores the [cppbridge.State]'s.
type StatesStorage interface {
	// GetStateByID returns [cppbridge.State] by state ID if exist.
	GetStateByID(stateID string) (*cppbridge.StateV2, bool)
}
