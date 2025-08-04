package storage

import (
	"context"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler/querier"
	pp_storage "github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/storage"
)

var _ storage.Storage = (*Adapter)(nil)

// Adapter for implementing the [Queryable] interface and append data.
type Adapter struct {
	//
}

// AppendHashdex append incoming [cppbridge.HashdexContent] to [pp_storage.Head].
func (ar *Adapter) AppendHashdex(
	ctx context.Context,
	hashdex cppbridge.ShardedData,
	relabelerID string,
	commitToWal bool,
) error {
	// if rr.haTracker.IsDrop(hashdex.Cluster(), hashdex.Replica()) {
	// 	return nil
	// }
	// incomingData := &relabeler.IncomingData{Hashdex: hashdex}
	// _, err := rr.activeHead.Append(ctx, incomingData, nil, relabelerID, commitToWal)
	return nil
}

// AppendScraperHashdex append ScraperHashdex data to [pp_storage.Head].
func (ar *Adapter) AppendScraperHashdex(
	ctx context.Context,
	hashdex cppbridge.ShardedData,
	state *cppbridge.State,
	relabelerID string,
	commitToWal bool,
) (cppbridge.RelabelerStats, error) {
	// return rr.activeHead.Append(
	// 	ctx,
	// 	&relabeler.IncomingData{Hashdex: hashdex},
	// 	state,
	// 	relabelerID,
	// 	commitToWal,
	// )

	return cppbridge.RelabelerStats{}, nil
}

// AppendSnappyProtobuf append compressed via snappy Protobuf data to [pp_storage.Head].
func (ar *Adapter) AppendSnappyProtobuf(
	ctx context.Context,
	compressedData pp_storage.ProtobufData,
	relabelerID string,
	commitToWal bool,
) error {
	// hx, err := cppbridge.NewWALSnappyProtobufHashdex(compressedData.Bytes(), rr.hashdexLimits)
	// compressedData.Destroy()
	// if err != nil {
	// 	return err
	// }

	// if rr.haTracker.IsDrop(hx.Cluster(), hx.Replica()) {
	// 	return nil
	// }

	// incomingData := &relabeler.IncomingData{Hashdex: hx}
	// _, err = rr.activeHead.Append(ctx, incomingData, nil, relabelerID, commitToWal)
	return nil
}

// AppendTimeSeries append TimeSeries data to [pp_storage.Head].
func (ar *Adapter) AppendTimeSeries(
	ctx context.Context,
	data pp_storage.TimeSeriesBatch,
	state *cppbridge.State,
	relabelerID string,
	commitToWal bool,
) (cppbridge.RelabelerStats, error) {
	// hx, err := rr.hashdexFactory.GoModel(data.TimeSeries(), rr.hashdexLimits)
	// if err != nil {
	// 	data.Destroy()
	// 	return cppbridge.RelabelerStats{}, err
	// }

	// if rr.haTracker.IsDrop(hx.Cluster(), hx.Replica()) {
	// 	data.Destroy()
	// 	return cppbridge.RelabelerStats{}, nil
	// }
	// incomingData := &relabeler.IncomingData{Hashdex: hx, Data: data}
	// return rr.activeHead.Append(
	// 	ctx,
	// 	incomingData,
	// 	state,
	// 	relabelerID,
	// 	commitToWal,
	// )

	return cppbridge.RelabelerStats{}, nil
}

// Appender create a new [storage.Appender] for [pp_storage.Head].
func (ar *Adapter) Appender(ctx context.Context) storage.Appender {
	// return newPromAppender(ctx, rr, prom_config.TransparentRelabeler)
	return nil
}

// ChunkQuerier provides querying access over time series data of a fixed time range.
// Returns new Chunk Querier that merges results of given primary and secondary chunk queriers.
func (ar *Adapter) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	// TODO
	return storage.NewMergeChunkQuerier(
		nil,
		[]storage.ChunkQuerier{},
		storage.NewConcatenatingChunkSeriesMerger(),
	), nil
}

// Close closes the storage and all its underlying resources.
// Implements the [storage.Storage] interface.
func (*Adapter) Close() error {
	return nil
}

// HeadQuerier returns [storage.Querier] from active head.
func (ar *Adapter) HeadQuerier(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
	// TODO
	return nil, nil
}

// HeadStatus returns stats of Head.
func (ar *Adapter) HeadStatus(ctx context.Context, limit int) pp_storage.HeadStatus {
	// TODO
	return pp_storage.HeadStatus{}
}

// Querier calls f() with the given parameters.
// Returns a [querier.MultiQuerier] combining of primary and secondary queriers.
func (ar *Adapter) Querier(mint, maxt int64) (storage.Querier, error) {
	// TODO
	return querier.NewMultiQuerier([]storage.Querier{}, nil), nil
}

// RelabelerIDIsExist check on exist relabelerID.
func (ar *Adapter) RelabelerIDIsExist(relabelerID string) bool {
	// TODO
	return true
}

// StartTime returns the oldest timestamp stored in the storage.
// Implements the [storage.Storage] interface.
func (*Adapter) StartTime() (int64, error) {
	return int64(model.Latest), nil
}
