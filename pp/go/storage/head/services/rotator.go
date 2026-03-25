package services

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/util"
)

//
// RotatorConfig
//

// RotatorConfig config for [Rotator].
type RotatorConfig interface {
	// NumberOfShards returns current number of shards.
	NumberOfShards() uint16
}

//
// Rotator
//

// Rotator at the end of the specified interval, it creates a new [Head] and makes it active,
// and sends the old [Head] to the [Keeper].
type Rotator[
	TTask Task,
	TShard, TGoShard Shard,
	THead Head[TTask, TShard, TGoShard],
] struct {
	proxyHead             ProxyHead[TTask, TShard, TGoShard, THead]
	headBuilder           HeadBuilder[TTask, TShard, TGoShard, THead]
	m                     Mediator
	cfg                   RotatorConfig
	headInformer          HeadInformer
	headAddedSeriesCopier func(source, destination THead)
	rotatedTrigger        func()

	// stat
	rotateCounter          prometheus.Counter
	events                 *prometheus.CounterVec
	waitLockRotateDuration prometheus.Gauge
	rotationDuration       prometheus.Gauge
}

// NewRotator init new [Rotator].
func NewRotator[
	TTask Task,
	TShard, TGoShard Shard,
	THead Head[TTask, TShard, TGoShard],
](
	proxyHead ProxyHead[TTask, TShard, TGoShard, THead],
	headBuilder HeadBuilder[TTask, TShard, TGoShard, THead],
	m Mediator,
	cfg RotatorConfig,
	headInformer HeadInformer,
	headAddedSeriesCopier func(source, destination THead),
	rotatedTrigger func(),
	registerer prometheus.Registerer,
) *Rotator[TTask, TShard, TGoShard, THead] {
	factory := util.NewUnconflictRegisterer(registerer)
	return &Rotator[TTask, TShard, TGoShard, THead]{
		proxyHead:             proxyHead,
		headBuilder:           headBuilder,
		m:                     m,
		cfg:                   cfg,
		headInformer:          headInformer,
		headAddedSeriesCopier: headAddedSeriesCopier,
		rotatedTrigger:        rotatedTrigger,
		rotateCounter: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "prompp_rotator_rotate_count",
				Help: "Total counter of rotate rotatable object.",
			},
		),
		events: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "prompp_head_event_count",
				Help: "Number of head events",
			},
			[]string{"type"},
		),
		waitLockRotateDuration: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "prompp_rotator_wait_lock_rotate_duration",
				Help: "The duration of the lock wait for rotation in nanoseconds",
			},
		),
		rotationDuration: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "prompp_rotator_rotate_duration",
				Help: "The duration of the rotate in nanoseconds",
			},
		),
	}
}

// Execute starts the [Rotator].
//
//revive:disable-next-line:confusing-naming // other type of Service.
func (s *Rotator[TTask, TShard, TGoShard, THead]) Execute(ctx context.Context) error {
	logger.Infof("The Rotator is running.")

	for range s.m.C() {
		if err := s.rotate(ctx, s.cfg.NumberOfShards()); err != nil {
			logger.Errorf("rotation failed: %v", err)
		}

		s.rotateCounter.Inc()
	}

	logger.Infof("The Rotator stopped.")

	return nil
}

// rotate it creates a new [Head] and makes it active, and sends the old [Head] to the [Keeper].
//
//revive:disable-next-line:cyclomatic // long but readable.
func (s *Rotator[TTask, TShard, TGoShard, THead]) rotate(
	ctx context.Context,
	numberOfShards uint16,
) error {
	start := time.Now()
	oldHead := s.proxyHead.Get()
	newHead, err := s.headBuilder.Build(oldHead.Generation()+1, numberOfShards)
	if err != nil {
		return fmt.Errorf("failed to build a new head: %w", err)
	}

	if oldHead.NumberOfShards() == newHead.NumberOfShards() {
		s.headAddedSeriesCopier(oldHead, newHead)
	}

	if err = s.proxyHead.AddWithReplace(oldHead, s.headInformer.CreatedAt(oldHead.ID())); err != nil {
		return fmt.Errorf("failed add to keeper old head: %w", err)
	}

	startWait := time.Now()
	if err = s.proxyHead.Replace(ctx, newHead); err != nil {
		if errClose := newHead.Close(); errClose != nil {
			logger.Errorf("failed close new head: %s : %v", newHead.ID(), errClose)
		}

		return fmt.Errorf("failed to replace old to new head: %w", err)
	}
	s.waitLockRotateDuration.Set(float64(time.Since(startWait).Nanoseconds()))

	if err = s.headInformer.SetActiveStatus(newHead.ID()); err != nil {
		logger.Warnf("failed set status active for head{%s}: %s", newHead.ID(), err)
	}

	if err = MergeOutOfOrderChunksWithHead(oldHead); err != nil {
		logger.Warnf("failed merge out of order chunks in data storage: %s", err)
	}

	if err = CFSViaRange(oldHead); err != nil {
		logger.Warnf("failed commit and flush to wal: %s", err)
	}

	if err = s.headInformer.SetRotatedStatus(oldHead.ID()); err != nil {
		logger.Warnf("failed set status rotated for head{%s}: %s", oldHead.ID(), err)
	}
	oldHead.SetReadOnly()
	s.events.With(prometheus.Labels{"type": "rotated"}).Inc()
	s.rotationDuration.Set(float64(time.Since(start).Nanoseconds()))
	s.rotatedTrigger()

	return nil
}
