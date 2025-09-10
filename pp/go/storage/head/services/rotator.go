package services

import (
	"context"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/storage/logger"
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
	proxyHead        ProxyHead[TTask, TShard, TGoShard, THead]
	headBuilder      HeadBuilder[TTask, TShard, TGoShard, THead]
	m                Mediator
	cfg              RotatorConfig
	headStatusSetter HeadStatusSetter
	rotateCounter    prometheus.Counter
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
	headStatusSetter HeadStatusSetter,
	r prometheus.Registerer,
) *Rotator[TTask, TShard, TGoShard, THead] {
	factory := util.NewUnconflictRegisterer(r)
	return &Rotator[TTask, TShard, TGoShard, THead]{
		proxyHead:        proxyHead,
		headBuilder:      headBuilder,
		m:                m,
		cfg:              cfg,
		headStatusSetter: headStatusSetter,
		rotateCounter: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "prompp_rotator_rotate_count",
				Help: "Total counter of rotate rotatable object.",
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
func (s *Rotator[TTask, TShard, TGoShard, THead]) rotate(
	ctx context.Context,
	numberOfShards uint16,
) error {
	oldHead := s.proxyHead.Get()

	newHead, err := s.headBuilder.Build(oldHead.Generation()+1, numberOfShards)
	if err != nil {
		return fmt.Errorf("failed to build a new head: %w", err)
	}

	// TODO CopySeriesFrom only old nunber of shards == new
	// newHead.CopySeriesFrom(oldHead)

	s.proxyHead.Add(oldHead)

	// TODO if replace error?
	if err = s.proxyHead.Replace(ctx, newHead); err != nil {
		return fmt.Errorf("failed to replace old to new head: %w", err)
	}

	if err = s.headStatusSetter.SetActiveStatus(newHead.ID()); err != nil {
		logger.Warnf("failed set status active for head{%s}: %s", newHead.ID(), err)
	}

	if err = MergeOutOfOrderChunksWithHead(oldHead); err != nil {
		logger.Warnf("failed merge out of order chunks in data storage: %s", err)
	}

	if err = CFSViaRange(oldHead); err != nil {
		logger.Warnf("failed commit and flush to wal: %s", err)
	}

	if err = s.headStatusSetter.SetRotatedStatus(oldHead.ID()); err != nil {
		logger.Warnf("failed set status rotated for head{%s}: %s", oldHead.ID(), err)
	}
	oldHead.SetReadOnly()

	return nil
}
