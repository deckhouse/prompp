package services

import (
	"context"

	"github.com/prometheus/prometheus/pp/go/storage/logger"
)

//
// Committer
//

// Committer finalize segment from encoder and add to wal
// and flush wal segment writer, write all buffered data to storage, do via task.
type Committer[
	TTask Task,
	TShard, TGoShard Shard,
	THead Head[TTask, TShard, TGoShard],
] struct {
	activeHead ActiveHeadContainer[TTask, TShard, TGoShard, THead]
	m          Mediator
	isNewHead  func(headID string) bool
}

// NewCommitter init new [Committer].
func NewCommitter[
	TTask Task,
	TShard, TGoShard Shard,
	THead Head[TTask, TShard, TGoShard],
](
	activeHead ActiveHeadContainer[TTask, TShard, TGoShard, THead],
	m Mediator,
	isNewHead func(headID string) bool,
) *Committer[TTask, TShard, TGoShard, THead] {
	return &Committer[TTask, TShard, TGoShard, THead]{
		activeHead: activeHead,
		m:          m,
		isNewHead:  isNewHead,
	}
}

// Execute starts the [Committer].
//
//revive:disable-next-line:confusing-naming // other type of Service.
func (s *Committer[TTask, TShard, TGoShard, THead]) Execute(ctx context.Context) error {
	logger.Infof("The Committer is running.")

	for range s.m.C() {
		if err := s.activeHead.With(ctx, s.commitFlushSync); err != nil {
			logger.Errorf("wal commit failed: %v", err)
		}
	}

	logger.Infof("The Committer stopped.")

	return nil
}

// commitFlushSync finalize segment from encoder and add to wal
// and flush wal segment writer, write all buffered data to storage and sync, do via range.
func (s *Committer[TTask, TShard, TGoShard, THead]) commitFlushSync(h THead) error {
	if s.isNewHead(h.ID()) {
		return nil
	}

	return CFSViaRange(h)
}
