package services

import (
	"context"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/util"
)

// HeadStatus holds information about number of series from [Head].
type HeadStatus interface {
	// NumSeries returns number of series.
	NumSeries() uint64
}

//
// MetricsUpdater
//

// MetricsUpdater a service that updates [Head] metrics.
type MetricsUpdater[
	TTask Task,
	TShard, TGoShard Shard,
	THead Head[TTask, TShard, TGoShard],
	THeadStatus HeadStatus,
] struct {
	proxyHead       ProxyHead[TTask, TShard, TGoShard, THead]
	m               Mediator
	queryHeadStatus func(ctx context.Context, head THead, limit int) (THeadStatus, error)

	// [Head] metrics for an active head.
	memoryInUse *prometheus.GaugeVec
	series      prometheus.Gauge
	walSize     *prometheus.GaugeVec
	queueSize   *prometheus.GaugeVec
}

// NewMetricsUpdater init new [MetricsUpdater].
func NewMetricsUpdater[
	TTask Task,
	TShard, TGoShard Shard,
	THead Head[TTask, TShard, TGoShard],
	THeadStatus HeadStatus,
](
	proxyHead ProxyHead[TTask, TShard, TGoShard, THead],
	m Mediator,
	queryHeadStatus func(ctx context.Context, head THead, limit int) (THeadStatus, error),
	r prometheus.Registerer,
) *MetricsUpdater[TTask, TShard, TGoShard, THead, THeadStatus] {
	factory := util.NewUnconflictRegisterer(r)
	return &MetricsUpdater[TTask, TShard, TGoShard, THead, THeadStatus]{
		proxyHead:       proxyHead,
		m:               m,
		queryHeadStatus: queryHeadStatus,

		memoryInUse: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_head_cgo_memory_bytes",
				Help: "Current value memory in use in bytes.",
			},
			[]string{"head_id", "allocator", "shard_id"},
		),
		series: factory.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_head_series",
			Help: "Total number of series in the head block.",
		}),
		walSize: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_head_current_wal_size",
				Help: "The size of the wall of the current head.",
			},
			[]string{"shard_id"},
		),
		queueSize: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_head_queue_tasks_size",
				Help: "The size of the queue of tasks of the current head.",
			},
			[]string{"shard_id"},
		),
	}
}

// Execute starts the [MetricsUpdater].
//
//revive:disable-next-line:confusing-naming // other type of Service.
func (s *MetricsUpdater[TTask, TShard, TGoShard, THead, THeadStatus]) Execute(ctx context.Context) error {
	logger.Infof("The MetricsUpdater is running.")

	for range s.m.C() {
		s.collect(ctx)
	}

	logger.Infof("The MetricsUpdater stopped.")

	return nil
}

// collect metrics from the head.
func (s *MetricsUpdater[TTask, TShard, TGoShard, THead, THeadStatus]) collect(ctx context.Context) {
	ahead := s.proxyHead.Get()

	status, err := s.queryHeadStatus(ctx, ahead, 0)
	if err != nil {
		// error may be only head is rotated, skip
		return
	}

	s.series.Set(float64(status.NumSeries()))

	for shardID, size := range ahead.RangeQueueSize() {
		s.queueSize.With(prometheus.Labels{"shard_id": strconv.Itoa(shardID)}).Set(float64(size))
	}

	s.collectFromShards(ahead, true)

	for _, head := range s.proxyHead.Heads() {
		if head.ID() == ahead.ID() {
			continue
		}

		s.collectFromShards(head, false)
	}
}

// fromShards collects metrics from the head's shards.
//
//revive:disable-next-line:flag-parameter this is a flag, but it's more convenient this way
func (s *MetricsUpdater[TTask, TShard, TGoShard, THead, THeadStatus]) collectFromShards(head THead, active bool) {
	headID := head.ID()
	for shard := range head.RangeShards() {
		ls := make(prometheus.Labels, 3) //revive:disable-line:add-constant it's labels count

		ls["shard_id"] = strconv.FormatUint(uint64(shard.ShardID()), 10) //revive:disable-line:add-constant it's base 10
		if active {
			s.walSize.With(ls).Set(float64(shard.WalCurrentSize()))
		}

		ls["head_id"] = headID
		ls["allocator"] = "data_storage"
		s.memoryInUse.With(ls).Set(float64(shard.DSAllocatedMemory()))

		ls["allocator"] = "main_lss"
		s.memoryInUse.With(ls).Set(float64(shard.LSSAllocatedMemory()))
	}
}
