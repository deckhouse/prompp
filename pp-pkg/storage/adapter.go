package storage

import (
	"context"
	"math"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp-pkg/model"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/hatracker"
	pp_storage "github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/appender"
	"github.com/prometheus/prometheus/pp/go/storage/head/services"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/prometheus/storage"
)

//
// Adapter
//

var _ storage.Storage = (*Adapter)(nil)

// Adapter for implementing the [Queryable] interface and append data.
type Adapter struct {
	proxy                 *pp_storage.Proxy
	builder               *pp_storage.Builder
	haTracker             *hatracker.HighAvailabilityTracker
	hashdexFactory        cppbridge.HashdexFactory
	hashdexLimits         cppbridge.WALHashdexLimits
	transparentState      *cppbridge.StateV2
	mergeOutOfOrderChunks func()
	longtermIntervalMs    int64

	// stat
	activeQuerierMetrics  *querier.Metrics
	storageQuerierMetrics *querier.Metrics
	appendDuration        prometheus.Histogram
	samplesAppended       prometheus.Counter
}

// NewAdapter init new main [Adapter].
func NewAdapter(
	clock clockwork.Clock,
	proxy *pp_storage.Proxy,
	builder *pp_storage.Builder,
	mergeOutOfOrderChunks func(),
	registerer prometheus.Registerer,
) *Adapter {
	return newAdapter(
		clock,
		proxy,
		builder,
		mergeOutOfOrderChunks,
		0,
		querier.QueryableAppenderSource,
		querier.QueryableStorageSource,
		registerer,
	)
}

// NewLongtermAdapter init new longterm [Adapter].
func NewLongtermAdapter(
	clock clockwork.Clock,
	proxy *pp_storage.Proxy,
	builder *pp_storage.Builder,
	mergeOutOfOrderChunks func(),
	longtermIntervalMs int64,
	registerer prometheus.Registerer,
) *Adapter {
	return newAdapter(
		clock,
		proxy,
		builder,
		mergeOutOfOrderChunks,
		longtermIntervalMs,
		querier.QueryableLongtermAppenderSource,
		querier.QueryableLongtermStorageSource,
		registerer,
	)
}

// newAdapter init new [Adapter].
func newAdapter(
	clock clockwork.Clock,
	proxy *pp_storage.Proxy,
	builder *pp_storage.Builder,
	mergeOutOfOrderChunks func(),
	longtermIntervalMs int64,
	activeSource, storageSource string,
	registerer prometheus.Registerer,
) *Adapter {
	factory := util.NewUnconflictRegisterer(registerer)
	return &Adapter{
		proxy:                 proxy,
		builder:               builder,
		haTracker:             hatracker.NewHighAvailabilityTracker(clock, registerer),
		hashdexFactory:        cppbridge.HashdexFactory{},
		hashdexLimits:         cppbridge.DefaultWALHashdexLimits(),
		transparentState:      cppbridge.NewTransitionStateV2(),
		mergeOutOfOrderChunks: mergeOutOfOrderChunks,
		longtermIntervalMs:    longtermIntervalMs,
		activeQuerierMetrics:  querier.NewMetrics(registerer, activeSource),
		storageQuerierMetrics: querier.NewMetrics(registerer, storageSource),
		appendDuration: factory.NewHistogram(
			prometheus.HistogramOpts{
				Name: "prompp_adapter_append_duration",
				Help: "Append to head duration in microseconds",
				Buckets: []float64{
					50, 100, 250, 500, 750,
					1000, 2500, 5000, 7500,
					10000, 25000, 50000, 75000,
					100000, 500000,
				},
			},
		),
		samplesAppended: factory.NewCounter(prometheus.CounterOpts{
			Name:        "prometheus_tsdb_head_samples_appended_total",
			Help:        "Total number of appended samples.",
			ConstLabels: prometheus.Labels{"type": "float"},
		}),
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

	var floatsAppended float64
	defer func(start time.Time) {
		ar.appendDuration.Observe(float64(time.Since(start).Microseconds()))
		ar.samplesAppended.Add(floatsAppended)
	}(time.Now())

	return ar.proxy.With(ctx, func(h *pp_storage.Head) error {
		stats, err := appender.New(h, services.CFViaRange).Append(
			ctx,
			&appender.IncomingData{Hashdex: hashdex},
			state,
			commitToWal,
		)
		floatsAppended = float64(stats.SamplesAdded)

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
	defer func(start time.Time) {
		ar.appendDuration.Observe(float64(time.Since(start).Microseconds()))
		ar.samplesAppended.Add(float64(stats.SamplesAdded))
	}(time.Now())

	_ = ar.proxy.With(ctx, func(h *pp_storage.Head) error {
		stats, err = appender.New(h, services.CFViaRange).Append(
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

	var floatsAppended float64
	defer func(start time.Time) {
		ar.appendDuration.Observe(float64(time.Since(start).Microseconds()))
		ar.samplesAppended.Add(floatsAppended)
	}(time.Now())

	return ar.proxy.With(ctx, func(h *pp_storage.Head) error {
		stats, err := appender.New(h, services.CFViaRange).Append(
			ctx,
			&appender.IncomingData{Hashdex: hx},
			state,
			commitToWal,
		)
		floatsAppended = float64(stats.SamplesAdded)

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

	defer func(start time.Time) {
		ar.appendDuration.Observe(float64(time.Since(start).Microseconds()))
		ar.samplesAppended.Add(float64(stats.SamplesAdded))
	}(time.Now())

	_ = ar.proxy.With(ctx, func(h *pp_storage.Head) error {
		stats, err = appender.New(h, services.CFViaRange).Append(
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

// BatchStorage creates a new [storage.BatchStorage] for appending time series data to [TransactionHead]
// and reading appended series data.
func (ar *Adapter) BatchStorage() storage.BatchStorage {
	return NewBatchStorage(
		ar.hashdexFactory,
		ar.hashdexLimits,
		ar.builder.BuildTransactionHead(),
		ar.transparentState,
		ar,
	)
}

// ChunkQuerier provides querying access over time series data of a fixed time range.
// Returns new Chunk Querier that merges results of given primary and secondary chunk queriers.
func (ar *Adapter) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	queriers := make([]storage.ChunkQuerier, 0, 1) //revive:disable-line:add-constant // the best way
	ahead := ar.proxy.Get()
	queriers = append(
		queriers,
		querier.NewChunkQuerier(
			ahead,
			querier.NewNoOpShardedDeduplicator,
			mint,
			maxt,
			ar.longtermIntervalMs,
			nil,
		),
	)

	for _, head := range ar.proxy.Heads() {
		if ahead.ID() == head.ID() {
			continue
		}

		queriers = append(
			queriers,
			querier.NewChunkQuerier(
				head,
				querier.NewNoOpShardedDeduplicator,
				mint,
				maxt,
				ar.longtermIntervalMs,
				nil,
			),
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
func (ar *Adapter) HeadQuerier(mint, maxt int64) (storage.Querier, error) {
	return querier.NewQuerier(
		ar.proxy.Get(),
		querier.NewNoOpShardedDeduplicator,
		mint,
		maxt,
		ar.longtermIntervalMs,
		nil,
		ar.activeQuerierMetrics,
	), nil
}

// HeadStatus returns stats of Head.
func (ar *Adapter) HeadStatus(ctx context.Context, limit int) (*querier.HeadStatus, error) {
	return querier.QueryHeadStatus(ctx, ar.proxy.Get(), limit)
}

// LowestSentTimestamp returns the lowest sent timestamp across all queues.
func (*Adapter) LowestSentTimestamp() int64 {
	return 0
}

// MergeOutOfOrderChunks send signal to merge chunks with out of order data chunks.
func (ar *Adapter) MergeOutOfOrderChunks() {
	ar.mergeOutOfOrderChunks()
}

// Querier calls f() with the given parameters.
// Returns a [querier.MultiQuerier] combining of primary and secondary queriers.
func (ar *Adapter) Querier(mint, maxt int64) (storage.Querier, error) {
	queriers := make([]storage.Querier, 0, 1) //revive:disable-line:add-constant // the best way
	ahead := ar.proxy.Get()
	queriers = append(
		queriers,
		querier.NewQuerier(
			ahead,
			querier.NewNoOpShardedDeduplicator,
			mint,
			maxt,
			ar.longtermIntervalMs,
			nil,
			ar.activeQuerierMetrics,
		),
	)

	for _, head := range ar.proxy.Heads() {
		if ahead.ID() == head.ID() {
			continue
		}

		timeInterval := headTimeInterval(head)
		if !timeInterval.IsInvalid() && mint > timeInterval.MaxT {
			continue
		}

		queriers = append(
			queriers,
			querier.NewQuerier(
				head,
				querier.NewNoOpShardedDeduplicator,
				mint,
				maxt,
				ar.longtermIntervalMs,
				nil,
				ar.storageQuerierMetrics,
			),
		)
	}

	return querier.NewMultiQuerier(queriers, nil), nil
}

// StartTime returns the oldest timestamp stored in the storage.
// Implements the [storage.Storage] interface.
func (*Adapter) StartTime() (int64, error) {
	return math.MaxInt64, nil
}

// headTimeInterval returns [cppbridge.TimeInterval] from [pp_storage.Head].
func headTimeInterval(head *pp_storage.Head) cppbridge.TimeInterval {
	timeInterval := cppbridge.NewInvalidTimeInterval()
	for _, shard := range head.Shards() {
		interval := shard.TimeInterval(false)
		timeInterval.MinT = min(interval.MinT, timeInterval.MinT)
		timeInterval.MaxT = max(interval.MaxT, timeInterval.MaxT)
	}

	return timeInterval
}
