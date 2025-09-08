package services

import (
	"context"

	"github.com/prometheus/prometheus/pp/go/storage/logger"
)

//
// Merger
//

// Merger a service that merge chunks with out of order data chunks for [Head].
type Merger[
	TTask Task,
	TShard, TGoShard Shard,
	THead Head[TTask, TShard, TGoShard],
] struct {
	activeHead ActiveHeadContainer[TTask, TShard, TGoShard, THead]
	m          Mediator
}

// NewMerger init new [Merger].
func NewMerger[
	TTask Task,
	TShard, TGoShard Shard,
	THead Head[TTask, TShard, TGoShard],
](
	activeHead ActiveHeadContainer[TTask, TShard, TGoShard, THead],
	m Mediator,
) *Merger[TTask, TShard, TGoShard, THead] {
	return &Merger[TTask, TShard, TGoShard, THead]{
		activeHead: activeHead,
		m:          m,
	}
}

// Execute starts the [Merger].
//
//revive:disable-next-line:confusing-naming // other type of Service.
func (s *Merger[TTask, TShard, TGoShard, THead]) Execute(ctx context.Context) error {
	logger.Infof("The Merger is running.")

	for range s.m.C() {
		if err := s.activeHead.With(ctx, MergeOutOfOrderChunksWithHead); err != nil {
			logger.Errorf("data storage merge failed: %v", err)
		}
	}

	logger.Infof("The Merger stopped.")

	return nil
}
