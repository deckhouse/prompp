package appender

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/prometheus/pp/go/relabeler/querier"
	"github.com/prometheus/prometheus/pp/go/util/locker"
	"github.com/prometheus/prometheus/storage"
)

type QueryableAppender struct {
	ctx            context.Context
	wlocker        *locker.Weighted
	head           relabeler.Head
	distributor    relabeler.Distributor
	querierMetrics *querier.Metrics
}

func NewQueryableAppender(
	ctx context.Context,
	head relabeler.Head,
	distributor relabeler.Distributor,
	querierMetrics *querier.Metrics,
) *QueryableAppender {
	return &QueryableAppender{
		ctx:            ctx,
		wlocker:        locker.NewWeighted(2 * head.Concurrency()), // x2 for back pressure
		head:           head,
		distributor:    distributor,
		querierMetrics: querierMetrics,
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
		qa.querierMetrics.AppendDuration.Observe(float64(time.Since(start).Microseconds()))
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

func (qa *QueryableAppender) Rotate(ctx context.Context) error {
	unlock, err := qa.wlocker.LockWithPriority(ctx)
	if err != nil {
		return fmt.Errorf("Rotate: weighted locker: %w", err)
	}
	defer unlock()

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
	head := qa.head
	runlock()

	return querier.NewQuerier(
		head,
		querier.NoOpShardedDeduplicatorFactory(),
		mint,
		maxt,
		func() error {
			return nil
		},
		qa.querierMetrics,
	), nil
}

func (qa *QueryableAppender) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	runlock, err := qa.wlocker.RLock(qa.ctx)
	if err != nil {
		return nil, fmt.Errorf("ChunkQuerier: weighted locker: %w", err)
	}
	head := qa.head
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
