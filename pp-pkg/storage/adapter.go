package storage

import (
	"context"
	"math"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/relabeler/querier"
	pp_storage "github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/storage"
)

//
// Proxy
//

// Proxy it proxies requests to the active [Head] and the keeper of old [Head]s.
type Proxy[THead any] interface {
	// Get the active [Head].
	Get() THead

	// RangeQueriableHeadsWithActive returns the iterator to queriable [Head]s:
	// the active [Head] and the [Head]s from the [Keeper].
	RangeQueriableHeadsWithActive(mint int64, maxt int64) func(func(THead) bool)

	// With calls fn(h Head) on active [Head].
	With(ctx context.Context, fn func(h THead) error) error
}

//
// HATracker
//

// HATracker interface for High Availability Tracker.
type HATracker interface {
	// IsDrop check whether data needs to be sent or discarded immediately.
	IsDrop(cluster, replica string) bool

	// Destroy clear all clusters and stop work.
	Destroy()
}

//
// HeadAppender
//

// HeadAppender adds incoming data to the [Head].
type HeadAppender interface {
	Append(
		ctx context.Context,
		incomingData *pp_storage.IncomingData,
		state *cppbridge.State,
		commitToWal bool,
	) ([][]*cppbridge.InnerSeries, cppbridge.RelabelerStats, error)
}

//
// ProtobufData
//

// ProtobufData is an universal interface for blob protobuf data.
type ProtobufData interface {
	Bytes() []byte
	Destroy()
}

//
// TimeSeriesData
//

// TimeSeriesBatch is an universal interface for batch [model.TimeSeries].
type TimeSeriesBatch interface {
	TimeSeries() []model.TimeSeries
	Destroy()
}

//
// Adapter
//

var _ storage.Storage = (*Adapter[any, HeadAppender])(nil)

// Adapter for implementing the [Queryable] interface and append data.
type Adapter[THead any, THeadAppender HeadAppender] struct {
	proxy          Proxy[THead]
	haTracker      HATracker
	appenderCtor   func(THead) THeadAppender
	hashdexFactory cppbridge.HashdexFactory
	hashdexLimits  cppbridge.WALHashdexLimits
}

// AppendHashdex append incoming [cppbridge.HashdexContent] to [Head].
func (ar *Adapter[THead, THeadAppender]) AppendHashdex(
	ctx context.Context,
	hashdex cppbridge.ShardedData,
	state *cppbridge.State,
	commitToWal bool,
) error {
	if ar.haTracker.IsDrop(hashdex.Cluster(), hashdex.Replica()) {
		return nil
	}

	return ar.proxy.With(ctx, func(h THead) error {
		_, _, err := ar.appenderCtor(h).Append(
			ctx,
			&pp_storage.IncomingData{Hashdex: hashdex},
			state,
			commitToWal,
		)

		return err
	})
}

// AppendScraperHashdex append ScraperHashdex data to [Head].
func (ar *Adapter[THead, THeadAppender]) AppendScraperHashdex(
	ctx context.Context,
	hashdex cppbridge.ShardedData,
	state *cppbridge.State,
	commitToWal bool,
) (stats cppbridge.RelabelerStats, err error) {
	_ = ar.proxy.With(ctx, func(h THead) error {
		_, stats, err = ar.appenderCtor(h).Append(
			ctx,
			&pp_storage.IncomingData{Hashdex: hashdex},
			state,
			commitToWal,
		)

		return nil
	})

	return stats, err
}

// AppendSnappyProtobuf append compressed via snappy Protobuf data to [Head].
func (ar *Adapter[THead, THeadAppender]) AppendSnappyProtobuf(
	ctx context.Context,
	compressedData ProtobufData,
	state *cppbridge.State,
	commitToWal bool,
) error {
	hx, err := cppbridge.NewWALSnappyProtobufHashdex(compressedData.Bytes(), ar.hashdexLimits)
	compressedData.Destroy()
	if err != nil {
		return err
	}

	if ar.haTracker.IsDrop(hx.Cluster(), hx.Replica()) {
		return nil
	}

	return ar.proxy.With(ctx, func(h THead) error {
		_, _, err := ar.appenderCtor(h).Append(
			ctx,
			&pp_storage.IncomingData{Hashdex: hx},
			state,
			commitToWal,
		)

		return err
	})
}

// AppendTimeSeries append TimeSeries data to [pp_storage.Head].
func (ar *Adapter[THead, THeadAppender]) AppendTimeSeries(
	ctx context.Context,
	data TimeSeriesBatch,
	state *cppbridge.State,
	commitToWal bool,
) (stats cppbridge.RelabelerStats, err error) {
	hx, err := ar.hashdexFactory.GoModel(data.TimeSeries(), ar.hashdexLimits)
	if err != nil {
		data.Destroy()
		return stats, err
	}

	if ar.haTracker.IsDrop(hx.Cluster(), hx.Replica()) {
		data.Destroy()
		return stats, nil
	}

	_ = ar.proxy.With(ctx, func(h THead) error {
		_, stats, err = ar.appenderCtor(h).Append(
			ctx,
			&pp_storage.IncomingData{Hashdex: hx, Data: data},
			state,
			commitToWal,
		)

		return nil
	})

	return stats, err
}

// Appender create a new [storage.Appender] for [pp_storage.Head].
func (ar *Adapter[THead, THeadAppender]) Appender(ctx context.Context) storage.Appender {
	//  TODO  state *cppbridge.State
	var state *cppbridge.State

	return newTimeSeriesAppender(ctx, ar, state)
}

// ChunkQuerier provides querying access over time series data of a fixed time range.
// Returns new Chunk Querier that merges results of given primary and secondary chunk queriers.
func (ar *Adapter[THead, THeadAppender]) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	// TODO
	return storage.NewMergeChunkQuerier(
		nil,
		[]storage.ChunkQuerier{},
		storage.NewConcatenatingChunkSeriesMerger(),
	), nil
}

// Close closes the storage and all its underlying resources.
// Implements the [storage.Storage] interface.
func (*Adapter[THead, THeadAppender]) Close() error {
	return nil
}

// HeadQuerier returns [storage.Querier] from active head.
func (ar *Adapter[THead, THeadAppender]) HeadQuerier(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
	// TODO
	return nil, nil
}

// HeadStatus returns stats of Head.
func (ar *Adapter[THead, THeadAppender]) HeadStatus(ctx context.Context, limit int) pp_storage.HeadStatus {
	// TODO
	// ar.proxy.
	return pp_storage.HeadStatus{}
}

// Querier calls f() with the given parameters.
// Returns a [querier.MultiQuerier] combining of primary and secondary queriers.
func (ar *Adapter[THead, THeadAppender]) Querier(mint, maxt int64) (storage.Querier, error) {
	// TODO
	return querier.NewMultiQuerier([]storage.Querier{}, nil), nil
}

// StartTime returns the oldest timestamp stored in the storage.
// Implements the [storage.Storage] interface.
func (*Adapter[THead, THeadAppender]) StartTime() (int64, error) {
	return math.MaxInt64, nil
}
