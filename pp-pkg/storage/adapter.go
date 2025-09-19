package storage

import (
	"context"
	"math"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp-pkg/model"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/hatracker"
	pp_storage "github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/appender"
	"github.com/prometheus/prometheus/pp/go/storage/head/services"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/storage"
)

//
// Adapter
//

var _ storage.Storage = (*Adapter)(nil)

// Adapter for implementing the [Queryable] interface and append data.
type Adapter struct {
	proxy            *pp_storage.ProxyHead
	haTracker        *hatracker.HighAvailabilityTracker
	hashdexFactory   cppbridge.HashdexFactory
	hashdexLimits    cppbridge.WALHashdexLimits
	transparentState *cppbridge.StateV2

	activeQuerierMetrics  *querier.Metrics
	storageQuerierMetrics *querier.Metrics
}

// NewAdapter init new [Adapter].
func NewAdapter(
	clock clockwork.Clock,
	proxy *pp_storage.ProxyHead,
	registerer prometheus.Registerer,
) *Adapter {
	return &Adapter{
		proxy:                 proxy,
		haTracker:             hatracker.NewHighAvailabilityTracker(clock, registerer),
		hashdexFactory:        cppbridge.HashdexFactory{},
		hashdexLimits:         cppbridge.DefaultWALHashdexLimits(),
		transparentState:      cppbridge.NewTransitionStateV2(),
		activeQuerierMetrics:  querier.NewMetrics(registerer, querier.QueryableAppenderSource),
		storageQuerierMetrics: querier.NewMetrics(registerer, querier.QueryableStorageSource),
	}
}

// AppendHashdex append incoming [cppbridge.HashdexContent] to [Head].
func (ar *Adapter) AppendHashdex(
	ctx context.Context,
	hashdex cppbridge.ShardedData,
	state *cppbridge.StateV2,
	commitToWal bool,
) error {
	if ar.haTracker.IsDrop(hashdex.Cluster(), hashdex.Replica()) {
		return nil
	}

	return ar.proxy.With(ctx, func(h *pp_storage.HeadOnDisk) error {
		_, _, err := appender.New(h, services.CFViaRange).Append(
			ctx,
			&appender.IncomingData{Hashdex: hashdex},
			state,
			commitToWal,
		)

		return err
	})
}

// AppendScraperHashdex append ScraperHashdex data to [Head].
func (ar *Adapter) AppendScraperHashdex(
	ctx context.Context,
	hashdex cppbridge.ShardedData,
	state *cppbridge.StateV2,
	commitToWal bool,
) (stats cppbridge.RelabelerStats, err error) {
	_ = ar.proxy.With(ctx, func(h *pp_storage.HeadOnDisk) error {
		_, stats, err = appender.New(h, services.CFViaRange).Append(
			ctx,
			&appender.IncomingData{Hashdex: hashdex},
			state,
			commitToWal,
		)

		return nil
	})

	return stats, err
}

// AppendSnappyProtobuf append compressed via snappy Protobuf data to [Head].
func (ar *Adapter) AppendSnappyProtobuf(
	ctx context.Context,
	compressedData model.ProtobufData,
	state *cppbridge.StateV2,
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

	return ar.proxy.With(ctx, func(h *pp_storage.HeadOnDisk) error {
		_, _, err := appender.New(h, services.CFViaRange).Append(
			ctx,
			&appender.IncomingData{Hashdex: hx},
			state,
			commitToWal,
		)

		return err
	})
}

// AppendTimeSeries append TimeSeries data to [Head].
func (ar *Adapter) AppendTimeSeries(
	ctx context.Context,
	data model.TimeSeriesBatch,
	state *cppbridge.StateV2,
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

	_ = ar.proxy.With(ctx, func(h *pp_storage.HeadOnDisk) error {
		_, stats, err = appender.New(h, services.CFViaRange).Append(
			ctx,
			&appender.IncomingData{Hashdex: hx, Data: data},
			state,
			commitToWal,
		)

		return nil
	})

	return stats, err
}

// Appender create a new [storage.Appender] for [Head].
func (ar *Adapter) Appender(ctx context.Context) storage.Appender {
	return newTimeSeriesAppender(ctx, ar, ar.transparentState)
}

// ChunkQuerier provides querying access over time series data of a fixed time range.
// Returns new Chunk Querier that merges results of given primary and secondary chunk queriers.
func (ar *Adapter) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	queriers := make([]storage.ChunkQuerier, 0, 2)
	ahead := ar.proxy.Get()
	queriers = append(
		queriers,
		querier.NewQuerier(ahead, querier.NewNoOpShardedDeduplicator, mint, maxt, nil, ar.activeQuerierMetrics),
	)

	for head := range ar.proxy.RangeQueriableHeads(mint, maxt) {
		if ahead.ID() == head.ID() {
			continue
		}

		queriers = append(
			queriers,
			querier.NewQuerier(head, querier.NewNoOpShardedDeduplicator, mint, maxt, nil, ar.storageQuerierMetrics),
		)
	}

	return storage.NewMergeChunkQuerier(
		nil,
		queriers,
		storage.NewConcatenatingChunkSeriesMerger(),
	), nil
}

// Close closes the storage and all its underlying resources.
// Implements the [storage.Storage] interface.
func (ar *Adapter) Close() error {
	ar.haTracker.Destroy()
	return nil
}

// HeadQuerier returns [storage.Querier] from active head.
func (ar *Adapter) HeadQuerier(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
	return querier.NewQuerier(
		ar.proxy.Get(),
		querier.NewNoOpShardedDeduplicator,
		mint,
		maxt,
		nil,
		ar.activeQuerierMetrics,
	), nil
}

// HeadStatus returns stats of Head.
func (ar *Adapter) HeadStatus(ctx context.Context, limit int) (querier.HeadStatus, error) {
	return querier.QueryHeadStatus(ctx, ar.proxy.Get(), limit)
}

// Querier calls f() with the given parameters.
// Returns a [querier.MultiQuerier] combining of primary and secondary queriers.
func (ar *Adapter) Querier(mint, maxt int64) (storage.Querier, error) {
	queriers := make([]storage.Querier, 0, 2)
	ahead := ar.proxy.Get()
	queriers = append(
		queriers,
		querier.NewQuerier(ahead, querier.NewNoOpShardedDeduplicator, mint, maxt, nil, ar.activeQuerierMetrics),
	)

	for head := range ar.proxy.RangeQueriableHeads(mint, maxt) {
		if ahead.ID() == head.ID() {
			continue
		}

		queriers = append(
			queriers,
			querier.NewQuerier(head, querier.NewNoOpShardedDeduplicator, mint, maxt, nil, ar.storageQuerierMetrics),
		)
	}

	return querier.NewMultiQuerier(queriers, nil), nil
}

// StartTime returns the oldest timestamp stored in the storage.
// Implements the [storage.Storage] interface.
func (*Adapter) StartTime() (int64, error) {
	return math.MaxInt64, nil
}
