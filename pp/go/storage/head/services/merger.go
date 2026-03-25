package services

import (
	"context"

	"github.com/prometheus/prometheus/pp/go/logger"
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
	isNewHead  func(headID string) bool
}

// NewMerger init new [Merger].
func NewMerger[
	TTask Task,
	TShard, TGoShard Shard,
	THead Head[TTask, TShard, TGoShard],
](
	activeHead ActiveHeadContainer[TTask, TShard, TGoShard, THead],
	m Mediator,
	isNewHead func(headID string) bool,
) *Merger[TTask, TShard, TGoShard, THead] {
	return &Merger[TTask, TShard, TGoShard, THead]{
		activeHead: activeHead,
		m:          m,
		isNewHead:  isNewHead,
	}
}

// Execute starts the [Merger].
//
//revive:disable-next-line:confusing-naming // other type of Service.
func (s *Merger[TTask, TShard, TGoShard, THead]) Execute(ctx context.Context) error {
	logger.Infof("The Merger is running.")

	for range s.m.C() {
		_ = s.activeHead.With(ctx, s.UnloadAndMerge)
	}

	logger.Infof("The Merger stopped.")

	return nil
}

// UnloadAndMerge unload unused series data and merge chunks with out of order data chunks for [Head].
func (s *Merger[TTask, TShard, TGoShard, THead]) UnloadAndMerge(h THead) error {
	if s.isNewHead(h.ID()) {
		return nil
	}

	if err := UnloadUnusedSeriesDataWithHead(h); err != nil {
		logger.Errorf("unload unused series data failed: %v", err)
	}

	if err := MergeOutOfOrderChunksWithHead(h); err != nil {
		logger.Errorf("data storage merge failed: %v", err)
	}

	return nil
}
