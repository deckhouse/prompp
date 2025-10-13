package handler

import (
	"context"

	"github.com/prometheus/prometheus/pp-pkg/handler/processor"
	"github.com/prometheus/prometheus/pp-pkg/model"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/storage"
)

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
		data model.TimeSeriesBatch,
		state *cppbridge.StateV2,
		commitToWal bool,
	) (cppbridge.RelabelerStats, error)

	// AppendSnappyProtobuf append compressed via snappy Protobuf data to [Head].
	AppendSnappyProtobuf(
		ctx context.Context,
		compressedData model.ProtobufData,
		state *cppbridge.StateV2,
		commitToWal bool,
	) error

	// HeadQuerier returns [storage.Querier] from active head.
	HeadQuerier(mint, maxt int64) (storage.Querier, error)

	// HeadStatus returns stats of Head.
	HeadStatus(ctx context.Context, limit int) (*querier.HeadStatus, error)

	// MergeOutOfOrderChunks send signal to merge chunks with out of order data chunks.
	MergeOutOfOrderChunks()
}

// StreamProcessor interface.
type StreamProcessor interface {
	Process(ctx context.Context, stream processor.MetricStream) error
}

// RefillProcessor interface.
type RefillProcessor interface {
	Process(ctx context.Context, refill processor.Refill) error
}

// RemoteWriteProcessor interface.
type RemoteWriteProcessor interface {
	Process(ctx context.Context, remoteWrite processor.RemoteWrite) error
}
