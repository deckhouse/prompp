package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/prometheus/prometheus/pp/go/storage/logger"
)

// const (
// 	// walCommit name of task.
// 	walCommit = "wal_commit"
// )

//
// Task
//

// Task the minimum required task [Generic] implementation.
type Task interface {
	// Wait for the task to complete on all shards.
	Wait() error
}

//
// Shard
//

// Shard the minimum required head [Shard] implementation.
type Shard interface {
	// MergeOutOfOrderChunks merge chunks with out of order data chunks in [DataStorage].
	MergeOutOfOrderChunks()

	// ShardID returns the shard ID.
	ShardID() uint16

	// WalCommit finalize segment from encoder and write to wal.
	WalCommit() error

	// WalFlush flush all contetnt into wal.
	WalFlush() error
}

//
// Head
//

// Head the minimum required [Head] implementation.
type Head[
	TTask Task,
	TShard, TGoShard Shard,
] interface {
	// // Close closes wals, query semaphore for the inability to get query and clear metrics.
	// Close(ctx context.Context) error

	// CreateTask create a task for operations on the [Head] shards.
	CreateTask(taskName string, shardFn func(shard TGoShard) error) TTask

	// Enqueue the task to be executed on shards [Head].
	Enqueue(t TTask)

	// Generation returns current generation of [Head].
	Generation() uint64

	// NumberOfShards returns current number of shards in to [Head].
	NumberOfShards() uint16

	// RangeShards returns an iterator over the [Head] [Shard]s, through which the shard can be directly accessed.
	RangeShards() func(func(TShard) bool)

	// SetReadOnly sets the read-only flag for the [Head].
	SetReadOnly()
}

//
// ActiveHeadContainer
//

// ActiveHeadContainer container for active [Head], the minimum required [ActiveHeadContainer] implementation.
type ActiveHeadContainer[
	TTask Task,
	TShard, TGoShard Shard,
	THead Head[TTask, TShard, TGoShard],
] interface {
	// Close closes [ActiveHeadContainer] for the inability work with [Head].
	Close(ctx context.Context) error

	// Get the active head [Head].
	Get() THead

	// Replace the active head [Head] with a new head.
	Replace(ctx context.Context, newHead THead) error

	// With calls fn(h Head).
	With(ctx context.Context, fn func(h THead) error) error
}

//
// Mediator
//

// Mediator notifies about events via the channel.
type Mediator interface {
	// C returns channel with events.
	C() <-chan struct{}

	// Close close channel and stop [Mediator].
	Close()
}

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

// Execute starts the [Committer].
//
//revive:disable-next-line:confusing-naming // other type of Service.
func (s *Committer[TTask, TShard, TGoShard, THead]) Execute(ctx context.Context) error {
	logger.Infof("The Committer is running.")
	for range s.m.C() {
		if err := s.activeHead.With(ctx, CommitAndFlushViaRange); err != nil {
			logger.Errorf("wal commit failed: %v", err)
		}
	}

	logger.Infof("The Committer stopped.")

	return nil
}

// Interrupt interrupts the [Committer] work.
//
//revive:disable-next-line:confusing-naming // other type of Service.
func (s *Committer[TTask, TShard, TGoShard, THead]) Interrupt(_ error) {
	logger.Infof("Stopping Committer...")

	s.m.Close()
}

//
// CommitAndFlushViaRange
//

// CommitAndFlushViaRange finalize segment from encoder and add to wal
// and flush wal segment writer, write all buffered data to storage, do via range.
func CommitAndFlushViaRange[
	TTask Task,
	TShard, TGoShard Shard,
	THead Head[TTask, TShard, TGoShard],
](h THead) error {
	errs := make([]error, 0, h.NumberOfShards()*2)
	for shard := range h.RangeShards() {
		if err := shard.WalCommit(); err != nil {
			errs = append(errs, fmt.Errorf("commit shard id %d: %w", shard.ShardID(), err))
		}

		if err := shard.WalFlush(); err != nil {
			errs = append(errs, fmt.Errorf("flush shard id %d: %w", shard.ShardID(), err))
		}
	}

	return errors.Join(errs...)
}

// // commitAndFlushViaTask finalize segment from encoder and add to wal
// // and flush wal segment writer, write all buffered data to storage, do via task.
// func commitAndFlushViaTask[
// 	TTask Task,
// 	TDataStorage DataStorage,
// 	TLSS LSS,
// 	TShard, TGoShard Shard[TDataStorage, TLSS],
// 	THead Head[TTask, TDataStorage, TLSS, TShard, TGoShard],
// ](h THead) error {
// 	t := h.CreateTask(
// 		WalCommit,
// 		func(shard TGoShard) error {
// 			swal := shard.Wal()

// 			// wal contains LSS and it is necessary to lock the LSS for reading for the commit.
// 			if err := shard.LSS().WithRLock(func(_, _ *cppbridge.LabelSetStorage) error {
// 				return swal.Commit()
// 			}); err != nil {
// 				return err
// 			}

// 			return swal.Flush()
// 		},
// 	)
// 	h.Enqueue(t)

// 	return t.Wait()
// }
