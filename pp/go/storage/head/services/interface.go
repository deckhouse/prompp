package services

import (
	"context"
	"time"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/block"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
)

//go:generate -command moq go run github.com/matryer/moq --rm --skip-ensure --pkg mock --out
//go:generate moq mock/persistener.go . HeadBlockWriter WriteNotifier
//go:generate moq mock/mediator.go . Mediator
//go:generate moq mock/head_builder.go . HeadBuilder
//go:generate moq mock/head_informer.go . HeadInformer
//go:generate moq mock/rotator_config.go . RotatorConfig

//
// ActiveHeadContainer
//

// ActiveHeadContainer container for active [Head], the minimum required [ActiveHeadContainer] implementation.
type ActiveHeadContainer[
	TTask Task,
	TShard, TGoShard Shard,
	THead Head[TTask, TShard, TGoShard],
] interface {
	// With calls fn(h Head).
	With(ctx context.Context, fn func(h THead) error) error
}

//
// Head
//

// RangeHead the minimum required [Head] implementation.
type RangeHead[TShard Shard] interface {
	// RangeShards returns an iterator over the [Head] [Shard]s, through which the shard can be directly accessed.
	RangeShards() func(func(TShard) bool)
}

// Head the minimum required [Head] implementation.
type Head[
	TTask Task,
	TShard, TGoShard Shard,
] interface {
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

	// IsReadOnly returns true if the [Head] has switched to read-only.
	IsReadOnly() bool

	// SetReadOnly sets the read-only flag for the [Head].
	SetReadOnly()

	// Close closes wals, query semaphore for the inability to get query and clear metrics.
	Close() error
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
// HeadInformer
//

// HeadInformer sets status by headID in to catalog and get info.
type HeadInformer interface {
	// CreatedAt returns the timestamp when the [Record]([Head]) was created.
	CreatedAt(headID string) time.Duration

	// SetActiveStatus sets the [catalog.StatusActive] status by headID.
	SetActiveStatus(headID string) error

	// SetRotatedStatus sets the [catalog.StatusRotated] status by headID.
	SetRotatedStatus(headID string) error
}

//
// Keeper
//

// Keeper holds outdated heads until conversion.
type Keeper[
	TTask Task,
	TShard, TGoShard Shard,
	THead Head[TTask, TShard, TGoShard],
] interface {
	// Add the [Head] to the [Keeper] if there is a free slot.
	Add(head THead, createdAt time.Duration) error

	// AddWithReplace the [Head] to the [Keeper] with replace if the createdAt is earlier.
	AddWithReplace(head THead, createdAt time.Duration) error

	// HasSlot returns the tru if there is a slot in the [Keeper].
	HasSlot() bool

	// Heads returns a slice of the [Head]s stored in the [Keeper].
	Heads() []THead

	// Remove removes [Head]s from the [Keeper].
	Remove(headsForRemove []THead)
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
// ProxyHead
//

// ProxyHead it proxies requests to the active [Head] and the keeper of old [Head]s.
type ProxyHead[
	TTask Task,
	TShard, TGoShard Shard,
	THead Head[TTask, TShard, TGoShard],
] interface {
	// Add the [Head] to the [Keeper] if there is a free slot.
	Add(head THead, createdAt time.Duration) error

	// AddWithReplace the [Head] to the [Keeper] with replace if the createdAt is earlier.
	AddWithReplace(head THead, createdAt time.Duration) error

	// Get the active [Head].
	Get() THead

	// HasSlot returns the tru if there is a slot in the [Keeper].
	HasSlot() bool

	// Heads returns a slice of the [Head]s stored in the [Keeper].
	Heads() []THead

	// Remove removes [Head]s from the [Keeper].
	Remove(headsForRemove []THead)

	// Replace the active head [Head] with a new head.
	Replace(ctx context.Context, newHead THead) error

	// With calls fn(h Head) on active [Head].
	With(ctx context.Context, fn func(h THead) error) error
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

	// WalSync commits the current contents of the [Wal].
	WalSync() error

	// TimeInterval get time interval of data storage
	TimeInterval(bool) cppbridge.TimeInterval

	// UnloadUnusedSeriesData unload unused series data
	UnloadUnusedSeriesData() error
}

//
// Task
//

// Task the minimum required task [Generic] implementation.
type Task interface {
	// Wait for the task to complete on all shards.
	Wait() error
}

//
// WriteNotifier
//

// WriteNotifier sends a notify that the writing is completed.
type WriteNotifier interface {
	// Notify sends a notify that the writing is completed.
	Notify()
}

//
// Loader
//

// Loader loads [Head] from [Wal].
type Loader[
	TTask Task,
	TShard, TGoShard Shard,
	THead Head[TTask, TShard, TGoShard],
] interface {
	// Load [Head] from [Wal] by head ID.
	Load(headRecord *catalog.Record, generation uint64) THead
}

//
// HeadBlockWriter
//

// HeadBlockWriter writes block on disk from [Head].
type HeadBlockWriter[TShard Shard] interface {
	Write(shard TShard) ([]block.WrittenBlock, error)
}
