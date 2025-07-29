package headcontainer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/prometheus/pp/go/relabeler/querier"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/prometheus/pp/go/util/locker"
	"github.com/prometheus/prometheus/storage"
)

type Active struct {
	wlocker *locker.Weighted
	head    relabeler.Head

	appendDuration         prometheus.Histogram
	waitLockRotateDuration prometheus.Gauge
	rotationDuration       prometheus.Gauge
}

func NewActive(
	head relabeler.Head,
	registerer prometheus.Registerer,
) *Active {
	factory := util.NewUnconflictRegisterer(registerer)
	return &Active{
		wlocker: locker.NewWeighted(2 * head.Concurrency()), // x2 for back pressure
		head:    head,

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

func (h *Active) Append(
	ctx context.Context,
	incomingData *relabeler.IncomingData,
	state *cppbridge.State,
	relabelerID string,
	commitToWal bool,
) (cppbridge.RelabelerStats, error) {
	start := time.Now()

	runlock, err := h.wlocker.RLock(ctx)
	if err != nil {
		return cppbridge.RelabelerStats{}, fmt.Errorf("Append: weighted locker: %w", err)
	}
	defer runlock()

	defer func() {
		h.appendDuration.Observe(float64(time.Since(start).Microseconds()))
	}()

	_, stats, err := h.head.Append(ctx, incomingData, state, relabelerID, commitToWal)
	if err != nil {
		return cppbridge.RelabelerStats{}, err
	}

	return stats, nil
}

func (h *Active) ChunkQuerier(ctx context.Context, mint, maxt int64) (storage.ChunkQuerier, error) {
	runlock, err := h.wlocker.RLock(ctx)
	if err != nil {
		return nil, fmt.Errorf("ChunkQuerier: weighted locker: %w", err)
	}
	head := h.head.Raw()
	runlock()
	return querier.NewChunkQuerier(
		head,
		querier.NoOpShardedDeduplicatorFactory(),
		mint,
		maxt,
		nil,
	), nil
}

func (h *Active) Close(ctx context.Context) error {
	unlock, err := h.wlocker.LockWithPriority(ctx)
	if err != nil {
		return fmt.Errorf("Close: weighted locker: %w", err)
	}
	defer unlock()

	return errors.Join(h.head.CommitToWal(), h.head.Flush(), h.head.Close())
}

func (h *Active) CommitToWal(ctx context.Context) error {
	runlock, err := h.wlocker.RLock(ctx)
	if err != nil {
		return fmt.Errorf("CommitToWal: weighted locker: %w", err)
	}
	defer runlock()

	return h.head.CommitToWal()
}

func (h *Active) HeadStatus(ctx context.Context, limit int) relabeler.HeadStatus {
	runlock, err := h.wlocker.RLock(ctx)
	if err != nil {
		logger.Warnf("[ActiveHead] HeadStatus: weighted locker: %s", err)
		return relabeler.HeadStatus{}
	}
	defer runlock()

	return h.head.Status(limit)
}

// MergeOutOfOrderChunks merge chunks with out of order data chunks.
func (h *Active) MergeOutOfOrderChunks(ctx context.Context) {
	runlock, err := h.wlocker.RLock(ctx)
	if err != nil {
		logger.Warnf("[ActiveHead] MergeOutOfOrderChunks: weighted locker: %s", err)
		return
	}
	defer runlock()

	h.head.MergeOutOfOrderChunks()
}

func (h *Active) Querier(
	ctx context.Context,
	querierMetrics *querier.Metrics,
	mint, maxt int64,
) (storage.Querier, error) {
	runlock, err := h.wlocker.RLock(ctx)
	if err != nil {
		return nil, fmt.Errorf("Querier: weighted locker: %w", err)
	}
	head := h.head.Raw()
	runlock()

	return querier.NewQuerier(
		head,
		querier.NoOpShardedDeduplicatorFactory(),
		mint,
		maxt,
		nil,
		querierMetrics,
	), nil
}

func (h *Active) Reconfigure(
	ctx context.Context,
	headConfigurator relabeler.HeadConfigurator,
) error {
	unlock, err := h.wlocker.LockWithPriority(ctx)
	if err != nil {
		return fmt.Errorf("Reconfigure: weighted locker: %w", err)
	}
	defer unlock()

	if err := headConfigurator.Configure(h.head); err != nil {
		return fmt.Errorf("failed to reconfigure head: %w", err)
	}

	h.wlocker.Resize(2 * h.head.Concurrency()) // x2 for back pressure

	return nil
}

func (h *Active) Rotate(ctx context.Context) error {
	start := time.Now()

	unlock, err := h.wlocker.LockWithPriority(ctx)
	if err != nil {
		return fmt.Errorf("Rotate: weighted locker: %w", err)
	}
	h.waitLockRotateDuration.Set(float64(time.Since(start).Nanoseconds()))
	defer unlock()

	defer func() {
		h.rotationDuration.Set(float64(time.Since(start).Nanoseconds()))
	}()

	if err := h.head.Rotate(); err != nil {
		return fmt.Errorf("failed to rotate head: %w", err)
	}

	h.wlocker.Resize(2 * h.head.Concurrency()) // x2 for back pressure

	return nil
}

func (h *Active) WriteMetrics(ctx context.Context) {
	runlock, err := h.wlocker.RLock(ctx)
	if err != nil {
		logger.Warnf("[ActiveHead] writeMetrics: weighted locker: %s", err)
		return
	}
	defer runlock()

	h.head.WriteMetrics(ctx)
}
