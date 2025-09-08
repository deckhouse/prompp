package services

import "context"

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
	Close() error

	// Get the active head [Head].
	Get() THead

	// Replace the active head [Head] with a new head.
	Replace(ctx context.Context, newHead THead) error

	// With calls fn(h Head).
	With(ctx context.Context, fn func(h THead) error) error
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

	// ID returns id [Head].
	ID() string

	// NumberOfShards returns current number of shards in to [Head].
	NumberOfShards() uint16

	// RangeQueueSize returns an iterator over the [Head] task channels, to collect metrics.
	RangeQueueSize() func(func(shardID, size int) bool)

	// RangeShards returns an iterator over the [Head] [Shard]s, through which the shard can be directly accessed.
	RangeShards() func(func(TShard) bool)

	// SetReadOnly sets the read-only flag for the [Head].
	SetReadOnly()
}

//
// HeadBuilder
//

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
// HeadStatusSetter
//

// HeadStatusSetter sets status by headID in to catalog.
type HeadStatusSetter interface {
	// SetActiveStatus sets the [catalog.StatusActive] status by headID.
	SetActiveStatus(headID string) error

	// SetRotatedStatus sets the [catalog.StatusRotated] status by headID.
	SetRotatedStatus(headID string) error
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
	RangeQueriableHeads(mint, maxt int64) func(func(THead) bool)
}

//
// Mediator
//

// Mediator notifies about events via the channel.
type Mediator interface {
	// C returns channel with events.
	C() <-chan struct{}
}

//
// Shard
//

// Shard the minimum required head [Shard] implementation.
type Shard interface {
	// DSAllocatedMemory return size of allocated memory for [DataStorage].
	DSAllocatedMemory() uint64

	// LSSAllocatedMemory return size of allocated memory for labelset storages.
	LSSAllocatedMemory() uint64

	// MergeOutOfOrderChunks merge chunks with out of order data chunks in [DataStorage].
	MergeOutOfOrderChunks()

	// ShardID returns the shard ID.
	ShardID() uint16

	// WalCommit finalize segment from encoder and write to wal.
	WalCommit() error

	// WalCurrentSize returns current [Wal] size.
	WalCurrentSize() int64

	// WalFlush flush all contetnt into wal.
	WalFlush() error
}

//
// Task
//

// Task the minimum required task [Generic] implementation.
type Task interface {
	// Wait for the task to complete on all shards.
	Wait() error
}
