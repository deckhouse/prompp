package appender

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/storage/head/poolprovider"
)

const (
	// dsAppendInnerSeries name of task.
	dsAppendInnerSeries = "data_storage_append_inner_series"

	// lssInputRelabeling name of task.
	lssInputRelabeling = "lss_input_relabeling"
	// lssAppendRelabelerSeries name of task.
	lssAppendRelabelerSeries = "lss_append_relabeler_series"

	// walWrite name of task.
	walWrite = "wal_write"
)

// errNilState error when incoming state is nil.
var errNilState = errors.New("state is nil")

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
	// AppendInnerSeriesSlice add InnerSeries to [DataStorage].
	AppendInnerSeriesSlice(innerSeriesSlice []cppbridge.InnerSeries)

	// LSSWithLock calls fn on raws [cppbridge.LabelSetStorage] with write lock.
	LSSWithLock(fn func(target, input *cppbridge.LabelSetStorage) error) error

	// LSSWithRLock calls fn on raws [cppbridge.LabelSetStorage] with read lock.
	LSSWithRLock(fn func(target, input *cppbridge.LabelSetStorage) error) error

	// LSSResetSnapshot resets the current snapshot. Use only WithLock.
	LSSResetSnapshot()

	// ShardID returns the shard ID.
	ShardID() uint16

	// WalWrite append the incoming inner series to wal encoder.
	WalWrite(innerSeriesSlice []cppbridge.InnerSeries) (bool, error)

	// DstSrcLsIdsMapping return ids mapping after lss copying
	DstSrcLsIdsMapping() *cppbridge.IdsMapping
}

//
// GoroutineShard
//

// GoroutineShard the minimum required head [GoroutineShard] implementation.
type GoroutineShard interface {
	// Relabeler returns relabeler for shard goroutines.
	Relabeler() *cppbridge.PerGoroutineRelabeler

	// Shard inherit from [Shard] methods.
	Shard
}

//
// Head
//

// Head the minimum required [Head] implementation.
type Head[
	TTask Task,
	TShard Shard,
	TGShard GoroutineShard,
] interface {
	// CreateTask create a task for operations on the [Head] shards.
	CreateTask(taskName string, shardFn func(shard TGShard) error) TTask

	// Enqueue the task to be executed on shards [Head].
	Enqueue(t TTask)

	// Generation returns current generation of [Head].
	Generation() uint64

	// NumberOfShards returns current number of shards in to [Head].
	NumberOfShards() uint16

	// PoolProvider returns the [poolprovider.HeadPool] for the [Head].
	PoolProvider() *poolprovider.HeadPool[TGShard]

	// PutTask adds [TTask] to the pool.
	PutTask(t TTask)

	// Shards returns the [Head] [Shard]s.
	Shards() []TShard
}

//
// Appender
//

// Appender adds incoming data to the [Head].
type Appender[
	TTask Task,
	TShard Shard,
	TGShard GoroutineShard,
	THead Head[TTask, TShard, TGShard],
] struct {
	head           THead
	poolProvider   *poolprovider.HeadPool[TGShard]
	commitAndFlush func(h THead) error
}

// New init new [Appender].
func New[
	TTask Task,
	TShard Shard,
	TGShard GoroutineShard,
	THead Head[TTask, TShard, TGShard],
](
	head THead,
	commitAndFlush func(h THead) error,
) Appender[TTask, TShard, TGShard, THead] {
	return Appender[TTask, TShard, TGShard, THead]{
		head:           head,
		poolProvider:   head.PoolProvider(),
		commitAndFlush: commitAndFlush,
	}
}

// Append incoming data to [Head].
//
//revive:disable-next-line:flag-parameter this is a flag, but it's more convenient this way
func (a Appender[TTask, TShard, TGShard, THead]) Append(
	ctx context.Context,
	incomingData *IncomingData,
	state *cppbridge.StateV2,
	commitToWal bool,
) (cppbridge.RelabelerStats, error) {
	if err := a.resolveState(state); err != nil {
		return cppbridge.RelabelerStats{}, err
	}

	shardedInnerSeries := a.poolProvider.GetShardedInnerSeries()
	defer a.poolProvider.PutShardedInnerSeries(shardedInnerSeries)
	shardedRelabeledSeries := a.poolProvider.GetShardedRelabeledSeries()
	defer a.poolProvider.PutShardedRelabeledSeries(shardedRelabeledSeries)

	stats, err := a.inputRelabelingStage(
		ctx,
		state,
		incomingData,
		shardedInnerSeries,
		shardedRelabeledSeries,
	)
	if err != nil {
		return stats, fmt.Errorf("failed input relabeling stage: %w", err)
	}

	shardedInnerSeries.Transpose()

	if !shardedRelabeledSeries.IsEmpty() {
		shardedRelabeledSeries.Transpose()

		shardedStateUpdates := a.poolProvider.GetShardedStateUpdates()
		defer a.poolProvider.PutShardedStateUpdates(shardedStateUpdates)
		if err = a.appendRelabelerSeriesStage(
			ctx,
			shardedInnerSeries,
			shardedRelabeledSeries,
			shardedStateUpdates,
		); err != nil {
			return stats, fmt.Errorf("failed append relabeler series stage: %w", err)
		}

		shardedStateUpdates.Transpose()
		if err = a.updateRelabelerStateStage(
			ctx,
			state,
			shardedStateUpdates,
		); err != nil {
			return stats, fmt.Errorf("failed update relabeler stage: %w", err)
		}
	}

	a.trackStaleNans(shardedInnerSeries, state)

	atomicLimitExhausted, err := a.appendInnerSeriesAndWriteToWal(shardedInnerSeries)
	if err != nil {
		logger.Errorf("failed to write wal: %v", err)
	}

	if commitToWal || atomicLimitExhausted > 0 {
		if err := a.commitAndFlush(a.head); err != nil {
			logger.Errorf("failed to commit wal: %v", err)
		}
	}

	return stats, nil
}

var errCannotBeRelabeledFromCache = errors.New("cannot be relabeled from cache")

// inputRelabelingStage first stage - relabeling.
//
//revive:disable-next-line:function-length long but this is first stage.
func (a *Appender[TTask, TShard, TGShard, THead]) inputRelabelingStage(
	ctx context.Context,
	state *cppbridge.StateV2,
	incomingData *IncomingData,
	shardedInnerSeries *cppbridge.ShardedInnerSeries,
	shardedRelabeledSeries *cppbridge.ShardedRelabeledSeries,
) (cppbridge.RelabelerStats, error) {
	stats := a.poolProvider.GetRelabelerStats()
	defer a.poolProvider.PutRelabelerStats(stats)
	defer incomingData.Destroy()

	t := a.head.CreateTask(
		lssInputRelabeling,
		func(shard TGShard) error {
			var (
				relabeler   = shard.Relabeler()
				shardID     = shard.ShardID()
				shardedData = incomingData.ShardedData()
				innerSeries = shardedInnerSeries.DataByShard(shardID)
			)

			err := shard.LSSWithRLock(func(target, input *cppbridge.LabelSetStorage) (rErr error) {
				var ok bool
				stats[shardID], ok, rErr = relabeler.RelabelingFromCache(
					ctx,
					input,
					target,
					state,
					shardedData,
					innerSeries,
				)
				if rErr != nil {
					return rErr
				}
				if !ok {
					return errCannotBeRelabeledFromCache
				}

				return nil
			})
			switch {
			case err == nil:
				return nil
			case errors.Is(err, errCannotBeRelabeledFromCache):
				// continue to relabel normally
			default:
				return fmt.Errorf("shard %d: %w", shardID, err)
			}

			rstats := cppbridge.RelabelerStats{}
			err = shard.LSSWithLock(func(target, input *cppbridge.LabelSetStorage) (rErr error) {
				var hasReallocations bool
				rstats, hasReallocations, rErr = relabeler.Relabeling(
					ctx,
					input,
					target,
					state,
					shardedData,
					innerSeries,
					shardedRelabeledSeries.DataByShard(shardID),
				)

				if hasReallocations {
					shard.LSSResetSnapshot()
				}

				return rErr
			})
			if err != nil {
				return fmt.Errorf("shard %d: %w", shardID, err)
			}

			stats[shardID].Add(rstats)

			return nil
		},
	)
	defer a.head.PutTask(t)
	a.head.Enqueue(t)

	resStats := cppbridge.RelabelerStats{}
	if err := t.Wait(); err != nil {
		return resStats, err
	}

	resStats.Add(stats...)

	return resStats, nil
}

// appendRelabelerSeriesStage second stage - append to lss relabeling ls.
func (a *Appender[TTask, TShard, TGShard, THead]) appendRelabelerSeriesStage(
	ctx context.Context,
	shardedInnerSeries *cppbridge.ShardedInnerSeries,
	shardedRelabeledSeries *cppbridge.ShardedRelabeledSeries,
	shardedStateUpdates *cppbridge.ShardedStateUpdates,
) error {
	t := a.head.CreateTask(
		lssAppendRelabelerSeries,
		func(shard TGShard) error {
			shardID := shard.ShardID()

			relabeledSeries := shardedRelabeledSeries.DataByShard(shardID)
			if cppbridge.RelabeledSeriesIsEmpty(relabeledSeries) {
				return nil
			}

			return shard.LSSWithLock(func(target, _ *cppbridge.LabelSetStorage) error {
				hasReallocations, err := shard.Relabeler().AppendRelabelerSeries(
					ctx,
					target,
					shardedInnerSeries.DataByShard(shardID),
					relabeledSeries,
					shardedStateUpdates.DataByShard(shardID),
				)
				if err != nil {
					return fmt.Errorf("shard %d: %w", shardID, err)
				}

				if hasReallocations {
					shard.LSSResetSnapshot()
				}

				return nil
			})
		},
	)
	defer a.head.PutTask(t)
	a.head.Enqueue(t)

	return t.Wait()
}

// updateRelabelerStateStage third stage - update state cache.
func (a *Appender[TTask, TShard, TGShard, THead]) updateRelabelerStateStage(
	ctx context.Context,
	state *cppbridge.StateV2,
	shardedStateUpdates *cppbridge.ShardedStateUpdates,
) error {
	numberOfShards := a.head.NumberOfShards()
	for shardID := range numberOfShards {
		updates := shardedStateUpdates.DataByShard(shardID)
		if cppbridge.RelabelerStateUpdateIsEmpty(updates) {
			continue
		}

		if err := state.CacheByShard(shardID).Update(ctx, updates); err != nil {
			return fmt.Errorf("shard %d: %w", shardID, err)
		}
	}

	return nil
}

// trackStaleNans add stale nans samples if needed
func (a *Appender[TTask, TShard, TGShard, THead]) trackStaleNans(
	shardInnerSeries *cppbridge.ShardedInnerSeries,
	state *cppbridge.StateV2,
) {
	if !state.TrackStaleness() {
		return
	}

	for i := range a.head.NumberOfShards() {
		cppbridge.PerGoroutineRelabelerTrackStaleNans(shardInnerSeries.DataByShard(i), state, i)
	}
}

// appendInnerSeriesAndWriteToWal append [cppbridge.InnerSeries] to [Shard]'s to [DataStorage] and write to [Wal].
func (a *Appender[TTask, TShard, TGShard, THead]) appendInnerSeriesAndWriteToWal(
	shardedInnerSeries *cppbridge.ShardedInnerSeries,
) (uint32, error) {
	tAppend := a.head.CreateTask(
		dsAppendInnerSeries,
		func(shard TGShard) error {
			shard.AppendInnerSeriesSlice(shardedInnerSeries.DataByShard(shard.ShardID()))

			return nil
		},
	)
	defer a.head.PutTask(tAppend)
	a.head.Enqueue(tAppend)

	var atomicLimitExhausted uint32
	tWalWrite := a.head.CreateTask(
		walWrite,
		func(shard TGShard) error {
			limitExhausted, errWrite := shard.WalWrite(shardedInnerSeries.DataByShard(shard.ShardID()))
			if errWrite != nil {
				return fmt.Errorf("shard %d: %w", shard.ShardID(), errWrite)
			}

			if limitExhausted {
				atomic.AddUint32(&atomicLimitExhausted, 1)
			}

			return nil
		},
	)
	defer a.head.PutTask(tWalWrite)
	a.head.Enqueue(tWalWrite)

	err := tAppend.Wait()
	err = errors.Join(err, tWalWrite.Wait())

	return atomicLimitExhausted, err
}

func (a *Appender[TTask, TShard, TGShard, THead]) resolveState(state *cppbridge.StateV2) error {
	if state == nil {
		return errNilState
	}

	state.Reconfigure(a.head.Generation(), a.head.NumberOfShards(), func(maps []*cppbridge.IdsMapping) {
		for id, shard := range a.head.Shards() {
			maps[id] = shard.DstSrcLsIdsMapping()
		}
	})

	return nil
}
