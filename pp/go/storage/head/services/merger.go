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
		if err := s.activeHead.With(ctx, s.mergeOutOfOrderChunks); err != nil {
			logger.Errorf("data storage merge failed: %v", err)
		}
	}

	logger.Infof("The Merger stopped.")

	return nil
}

// mergeOutOfOrderChunksWithHead merge chunks with out of order data chunks for [Head].
func (s *Merger[TTask, TShard, TGoShard, THead]) mergeOutOfOrderChunks(h THead) error {
	if s.isNewHead(h.ID()) {
		return nil
	}

	return MergeOutOfOrderChunksWithHead(h)
}
