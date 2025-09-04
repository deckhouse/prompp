package services

import (
	"context"
	"fmt"

	"github.com/prometheus/prometheus/pp/go/storage/logger"
)

const (
	// dsMergeOutOfOrderChunks name of task.
	dsMergeOutOfOrderChunks = "data_storage_merge_out_of_order_chunks"
)

// HeadBuilder building new [Head] with parameters, the minimum required [HeadBuilder] implementation.
type HeadBuilder[
	TTask Task,
	TShard, TGoShard Shard,
	THead Head[TTask, TShard, TGoShard],
] interface {
	// Build new [Head].
	Build(generation uint64, numberOfShards uint16) (THead, error)
}

//
// Keeper
//

type Keeper[
	TTask Task,
	TShard, TGShard Shard,
	THead Head[TTask, TShard, TGShard],
] interface {
	Add(head THead)
}

//
// Rotator
//

type Rotator[
	TTask Task,
	TShard, TGoShard Shard,
	THead Head[TTask, TShard, TGoShard],
] struct {
	activeHead  ActiveHeadContainer[TTask, TShard, TGoShard, THead]
	headBuilder HeadBuilder[TTask, TShard, TGoShard, THead]
	keeper      Keeper[TTask, TShard, TGoShard, THead]
	m           Mediator
}

// Execute starts the [Rotator].
//
//revive:disable-next-line:confusing-naming // other type of Service.
func (s *Rotator[TTask, TShard, TGoShard, THead]) Execute(ctx context.Context) error {
	logger.Infof("The Rotator is running.")

	// TODO
	var numberOfShards uint16

	for range s.m.C() {
		if err := s.rotate(ctx, numberOfShards); err != nil {
			logger.Errorf("rotation failed: %v", err)
		}
	}

	logger.Infof("The Rotator stopped.")

	return nil
}

// Interrupt interrupts the [Rotator] work.
//
//revive:disable-next-line:confusing-naming // other type of Service.
func (s *Rotator[TTask, TShard, TGoShard, THead]) Interrupt(_ error) {
	logger.Infof("Stopping Rotator...")

	s.m.Close()
}

func (s *Rotator[TTask, TShard, TGoShard, THead]) rotate(
	ctx context.Context,
	numberOfShards uint16,
) error {
	oldHead := s.activeHead.Get()

	newHead, err := s.headBuilder.Build(oldHead.Generation()+1, numberOfShards)
	if err != nil {
		return fmt.Errorf("failed to build a new head: %w", err)
	}

	// TODO CopySeriesFrom only old nunber of shards == new
	// newHead.CopySeriesFrom(oldHead)

	s.keeper.Add(oldHead)

	// TODO if replace error?
	err = s.activeHead.Replace(ctx, newHead)
	if err != nil {
		return fmt.Errorf("failed to replace old to new head: %w", err)
	}

	mergeOutOfOrderChunksWithHead(oldHead)

	if err := CommitAndFlushViaRange(oldHead); err != nil {
		logger.Warnf("failed commit and flush to wal: %s", err)
	}

	oldHead.SetReadOnly()

	return nil
}

// mergeOutOfOrderChunksWithHead merge chunks with out of order data chunks for [Head].
func mergeOutOfOrderChunksWithHead[
	TTask Task,
	TShard, TGShard Shard,
	THead Head[TTask, TShard, TGShard],
](h THead) {
	t := h.CreateTask(
		dsMergeOutOfOrderChunks,
		func(shard TGShard) error {
			shard.MergeOutOfOrderChunks()

			return nil
		},
	)
	h.Enqueue(t)

	_ = t.Wait()
}
