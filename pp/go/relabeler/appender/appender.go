package appender

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/prometheus/pp/go/relabeler/querier"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/prometheus/pp/go/util/locker"
	"github.com/prometheus/prometheus/storage"
)

type QueryableAppender struct {
	ctx            context.Context
	wlocker        *locker.Weighted
	head           relabeler.Head
	distributor    relabeler.Distributor
	querierMetrics *querier.Metrics

	appendDuration         prometheus.Histogram
	waitLockRotateDuration prometheus.Gauge
	rotationDuration       prometheus.Gauge
}

func NewQueryableAppender(
	ctx context.Context,
	head relabeler.Head,
	distributor relabeler.Distributor,
	querierMetrics *querier.Metrics,
	registerer prometheus.Registerer,
) *QueryableAppender {
	factory := util.NewUnconflictRegisterer(registerer)
	return &QueryableAppender{
		ctx:            ctx,
		wlocker:        locker.NewWeighted(2 * head.Concurrency()), // x2 for back pressure
		head:           head,
		distributor:    distributor,
		querierMetrics: querierMetrics,

		appendDuration: factory.NewHistogram(
			prometheus.HistogramOpts{
				Name: "prompp_head_append_duration",
				Help: "Append to head duration in microseconds",
				Buckets: []float64{
					50, 100, 250, 500, 750,
					1000, 2500, 5000, 7500,
					10000, 25000, 50000, 75000,
					100000, 500000,
				},
			},
		),

		waitLockRotateDuration: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "prompp_head_wait_lock_rotate_duration",
				Help: "The duration of the lock wait for rotation in nanoseconds",
			},
		),
		rotationDuration: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "prompp_head_rotate_duration",
				Help: "The duration of the rotate in nanoseconds",
			},
		),
	}
}

func (qa *QueryableAppender) Append(
	ctx context.Context,
	incomingData *relabeler.IncomingData,
	state *cppbridge.State,
	relabelerID string,
	commitToWal bool,
) (cppbridge.RelabelerStats, error) {
	return qa.AppendWithStaleNans(ctx, incomingData, state, relabelerID, commitToWal)
}

func (qa *QueryableAppender) AppendWithStaleNans(
	ctx context.Context,
	incomingData *relabeler.IncomingData,
	state *cppbridge.State,
	relabelerID string,
	commitToWal bool,
) (cppbridge.RelabelerStats, error) {
	start := time.Now()

	runlock, err := qa.wlocker.RLock(ctx)
	if err != nil {
		return cppbridge.RelabelerStats{}, fmt.Errorf("AppendWithStaleNans: weighted locker: %w", err)
	}
	defer runlock()

	defer func() {
		qa.appendDuration.Observe(float64(time.Since(start).Microseconds()))
	}()

	data, stats, err := qa.head.Append(ctx, incomingData, state, relabelerID, commitToWal)
	if err != nil {
		return cppbridge.RelabelerStats{}, err
	}

	if err = qa.distributor.Send(ctx, qa.head, data); err != nil {
		return stats, err
	}

	return stats, nil
}

func (qa *QueryableAppender) WriteMetrics(ctx context.Context) {
	runlock, err := qa.wlocker.RLock(ctx)
	if err != nil {
		logger.Warnf("[QueryableAppender] writeMetrics: weighted locker: %s", err)
		return
	}
	defer runlock()

	qa.head.WriteMetrics(ctx)
	qa.distributor.WriteMetrics(qa.head)
}

// MergeOutOfOrderChunks merge chunks with out of order data chunks.
func (qa *QueryableAppender) MergeOutOfOrderChunks(ctx context.Context) {
	runlock, err := qa.wlocker.RLock(ctx)
	if err != nil {
		logger.Warnf("[QueryableAppender] MergeOutOfOrderChunks: weighted locker: %s", err)
		return
	}
	defer runlock()

	qa.head.MergeOutOfOrderChunks()
}

func (qa *QueryableAppender) HeadStatus(ctx context.Context, limit int) relabeler.HeadStatus {
	runlock, err := qa.wlocker.RLock(ctx)
	if err != nil {
		logger.Warnf("[QueryableAppender] HeadStatus: weighted locker: %s", err)
		return relabeler.HeadStatus{}
	}
	defer runlock()

	return qa.head.Status(limit)
}

func (qa *QueryableAppender) CommitToWal(ctx context.Context) error {
	runlock, err := qa.wlocker.RLock(ctx)
	if err != nil {
		return fmt.Errorf("CommitToWal: weighted locker: %w", err)
	}
	defer runlock()

	return qa.head.CommitToWal()
}

func (qa *QueryableAppender) UnloadUnusedSeriesData(ctx context.Context) {
	runlock, err := qa.wlocker.RLock(ctx)
	if err != nil {
		logger.Warnf("[QueryableAppender] UnloadUnusedSeriesData: weighted locker: %s", err)
		return
	}
	defer runlock()

	qa.head.UnloadUnusedSeriesData()
}

func (qa *QueryableAppender) Rotate(ctx context.Context) error {
	start := time.Now()

	unlock, err := qa.wlocker.LockWithPriority(ctx)
	if err != nil {
		return fmt.Errorf("Rotate: weighted locker: %w", err)
	}
	qa.waitLockRotateDuration.Set(float64(time.Since(start).Nanoseconds()))
	defer unlock()

	defer func() {
		qa.rotationDuration.Set(float64(time.Since(start).Nanoseconds()))
	}()

	qa.head.MergeOutOfOrderChunks()

	if err := qa.head.Rotate(); err != nil {
		return fmt.Errorf("failed to rotate head: %w", err)
	}

	qa.wlocker.Resize(2 * qa.head.Concurrency()) // x2 for back pressure

	if err := qa.distributor.Rotate(); err != nil {
		return fmt.Errorf("failed to rotate distributor: %w", err)
	}

	return nil
}

func (qa *QueryableAppender) Reconfigure(
	ctx context.Context,
	headConfigurator relabeler.HeadConfigurator,
	distributorConfigurator relabeler.DistributorConfigurator,
) error {
	unlock, err := qa.wlocker.LockWithPriority(ctx)
	if err != nil {
		return fmt.Errorf("Reconfigure: weighted locker: %w", err)
	}
	defer unlock()

	qa.head.MergeOutOfOrderChunks()

	if err := headConfigurator.Configure(qa.head); err != nil {
		return fmt.Errorf("failed to reconfigure head: %w", err)
	}

	qa.wlocker.Resize(2 * qa.head.Concurrency()) // x2 for back pressure

	if err := distributorConfigurator.Configure(qa.distributor); err != nil {
		return fmt.Errorf("failed to upgrade distributor: %w", err)
	}

	return nil
}

func (qa *QueryableAppender) Querier(mint, maxt int64) (storage.Querier, error) {
	runlock, err := qa.wlocker.RLock(qa.ctx)
	if err != nil {
		return nil, fmt.Errorf("Querier: weighted locker: %w", err)
	}
	head := qa.head.Raw()
	runlock()

	return querier.NewQuerier(
		head,
		querier.NoOpShardedDeduplicatorFactory(),
		mint,
		maxt,
		nil,
		qa.querierMetrics,
	), nil
}

func (qa *QueryableAppender) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	runlock, err := qa.wlocker.RLock(qa.ctx)
	if err != nil {
		return nil, fmt.Errorf("ChunkQuerier: weighted locker: %w", err)
	}
	head := qa.head.Raw()
	runlock()
	return querier.NewChunkQuerier(
		head,
		querier.NoOpShardedDeduplicatorFactory(),
		mint,
		maxt,
		nil,
	), nil
}

func (qa *QueryableAppender) Close(ctx context.Context) error {
	unlock, err := qa.wlocker.LockWithPriority(ctx)
	if err != nil {
		return fmt.Errorf("Close: weighted locker: %w", err)
	}
	defer unlock()

	return errors.Join(qa.head.CommitToWal(), qa.head.Flush(), qa.head.Close())
}

// FindFromBuilder label set from builder in lss, if not found return EmptyLabels.
func (qa *QueryableAppender) FindFromBuilder(
	ctx context.Context,
	builderSortedAdd []cppbridge.Label,
	builderSortedDel []string,
	builderSnapshot *cppbridge.LabelSetSnapshot,
	hash uint64,
	builderLSID uint32,
	skipCache bool,
) (labels.Labels, bool) {
	runlock, err := qa.wlocker.RLock(ctx)
	if err != nil {
		logger.Warnf("[QueryableAppender] FindFromBuilder: weighted locker: %s", err)
		return labels.EmptyLabels(), false
	}
	defer runlock()

	return qa.head.FindFromBuilder(builderSortedAdd, builderSortedDel, builderSnapshot, hash, builderLSID, skipCache)
}

// FindByHash label set by hash in cache.
func (qa *QueryableAppender) FindByHash(
	ctx context.Context,
	hash uint64,
	builderSortedAdd []cppbridge.Label,
	builderSortedDel []string,
	builderSnapshot *cppbridge.LabelSetSnapshot,
	builderLSID uint32,
) (labels.Labels, bool) {
	runlock, err := qa.wlocker.RLock(ctx)
	if err != nil {
		logger.Warnf("[QueryableAppender] FindByHash: weighted locker: %s", err)
		return labels.EmptyLabels(), false
	}
	defer runlock()

	return qa.head.FindByHash(
		hash,
		builderSortedAdd,
		builderSortedDel,
		builderSnapshot,
		builderLSID,
	)
}
