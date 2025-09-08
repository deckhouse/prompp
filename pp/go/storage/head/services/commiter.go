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
}

// NewCommitter init new [Committer].
func NewCommitter[
	TTask Task,
	TShard, TGoShard Shard,
	THead Head[TTask, TShard, TGoShard],
](
	activeHead ActiveHeadContainer[TTask, TShard, TGoShard, THead],
	m Mediator,
) *Committer[TTask, TShard, TGoShard, THead] {
	return &Committer[TTask, TShard, TGoShard, THead]{
		activeHead: activeHead,
		m:          m,
	}
}

// Execute starts the [Committer].
//
//revive:disable-next-line:confusing-naming // other type of Service.
func (s *Committer[TTask, TShard, TGoShard, THead]) Execute(ctx context.Context) error {
	logger.Infof("The Committer is running.")

	for range s.m.C() {
		if err := s.activeHead.With(ctx, s.commitAndFlushViaRange); err != nil {
			logger.Errorf("wal commit failed: %v", err)
		}
	}

	logger.Infof("The Committer stopped.")

	return nil
}

func (s *Committer[TTask, TShard, TGoShard, THead]) commitAndFlushViaRange(h THead) error {
	return CommitAndFlushViaRange(h)
}
