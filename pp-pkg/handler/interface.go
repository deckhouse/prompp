package handler

import (
	"context"

	"github.com/prometheus/prometheus/pp-pkg/handler/processor"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/storage"
)

// Receiver interface.
type Receiver interface {
	AppendSnappyProtobuf(ctx context.Context, compressedData relabeler.ProtobufData, relabelerID string, commitToWal bool) error
	AppendHashdex(ctx context.Context, hashdex cppbridge.ShardedData, relabelerID string, commitToWal bool) error
	HeadQueryable() storage.Queryable
	HeadStatus(limit int) relabeler.HeadStatus
	// MergeOutOfOrderChunks merge chunks with out of order data chunks.
	MergeOutOfOrderChunks()
	RelabelerIDIsExist(relabelerID string) bool
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
