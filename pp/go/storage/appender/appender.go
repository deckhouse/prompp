package appender

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/head/task"
	"github.com/prometheus/prometheus/pp/go/storage/logger"
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
// LSS
//

// LSS the minimum required [LSS] implementation.
type LSS interface {
	// WithLock calls fn on raws [cppbridge.LabelSetStorage] with write lock.
	WithLock(fn func(target, input *cppbridge.LabelSetStorage) error) error

	// WithRLock calls fn on raws [cppbridge.LabelSetStorage] with read lock.
	WithRLock(fn func(target, input *cppbridge.LabelSetStorage) error) error

	// ResetSnapshot resets the current snapshot. Use only WithLock.
	ResetSnapshot()
}

//
// Shard
//

// Shard the minimum required head [Shard] implementation.
type Shard[TLSS LSS] interface {
	// AppendInnerSeriesSlice add InnerSeries to [DataStorage].
	AppendInnerSeriesSlice(innerSeriesSlice []*cppbridge.InnerSeries)

	// LSS returns shard labelset storage [LSS].
	LSS() TLSS

	// Relabeler returns relabeler for shard goroutines.
	Relabeler() *cppbridge.PerGoroutineRelabeler

	// ShardID returns the shard ID.
	ShardID() uint16

	// WalWrite append the incoming inner series to wal encoder.
	WalWrite(innerSeriesSlice []*cppbridge.InnerSeries) (bool, error)
}

//
// Head
//

// Head the minimum required [Head] implementation.
type Head[
	TTask Task,
	TLSS LSS,
	TShard Shard[TLSS],
] interface {
	// CreateTask create a task for operations on the [Head] shards.
	CreateTask(taskName string, shardFn func(shard TShard) error) TTask

	// Enqueue the task to be executed on shards [Head].
	Enqueue(t TTask)

	// Generation returns current generation of [Head].
	Generation() uint64

	// NumberOfShards returns current number of shards in to [Head].
	NumberOfShards() uint16
}

//
// Appender
//

// Appender adds incoming data to the [Head].
type Appender[
	TTask Task,
	TLSS LSS,
	TShard Shard[TLSS],
	THead Head[TTask, TLSS, TShard],
] struct {
	head           THead
	commitAndFlush func(h THead) error
}

// New init new [Appender].
func New[
	TTask Task,
	TLSS LSS,
	TShard Shard[TLSS],
	THead Head[TTask, TLSS, TShard],
](head THead, commitAndFlush func(h THead) error) Appender[TTask, TLSS, TShard, THead] {
	return Appender[TTask, TLSS, TShard, THead]{
		head:           head,
		commitAndFlush: commitAndFlush,
	}
}

// Append incoming data to [Head].
//
//revive:disable-next-line:flag-parameter this is a flag, but it's more convenient this way
func (a Appender[TTask, TLSS, TShard, THead]) Append(
	ctx context.Context,
	incomingData *IncomingData,
	state *cppbridge.State,
	commitToWal bool,
) ([][]*cppbridge.InnerSeries, cppbridge.RelabelerStats, error) {
	if err := a.resolveState(state); err != nil {
		return nil, cppbridge.RelabelerStats{}, err
	}

	shardedInnerSeries := NewShardedInnerSeries(a.head.NumberOfShards())
	shardedRelabeledSeries := NewShardedRelabeledSeries(a.head.NumberOfShards())

	stats, err := a.inputRelabelingStage(
		ctx,
		state,
		NewDestructibleIncomingData(incomingData, int(a.head.NumberOfShards())),
		shardedInnerSeries,
		shardedRelabeledSeries,
	)
	if err != nil {
		return nil, stats, fmt.Errorf("failed input relabeling stage: %w", err)
	}

	if !shardedRelabeledSeries.IsEmpty() {
		shardedStateUpdates := NewShardedStateUpdates(a.head.NumberOfShards())
		if err = a.appendRelabelerSeriesStage(
			ctx,
			shardedInnerSeries,
			shardedRelabeledSeries,
			shardedStateUpdates,
		); err != nil {
			return nil, stats, fmt.Errorf("failed append relabeler series stage: %w", err)
		}

		if err = a.updateRelabelerStateStage(
			ctx,
			state,
			shardedStateUpdates,
		); err != nil {
			return nil, stats, fmt.Errorf("failed update relabeler stage: %w", err)
		}
	}

	atomicLimitExhausted, err := a.appendInnerSeriesAndWriteToWal(shardedInnerSeries)
	if err != nil {
		logger.Errorf("failed to write wal: %v", err)
	}

	if commitToWal || atomicLimitExhausted > 0 {
		if err := a.commitAndFlush(a.head); err != nil {
			logger.Errorf("failed to commit wal: %v", err)
		}
	}

	return shardedInnerSeries.Data(), stats, nil
}

// inputRelabelingStage first stage - relabeling.
//
//revive:disable-next-line:function-length long but this is first stage.
func (a Appender[TTask, TLSS, TShard, THead]) inputRelabelingStage(
	ctx context.Context,
	state *cppbridge.State,
	incomingData *DestructibleIncomingData,
	shardedInnerSeries *ShardedInnerSeries,
	shardedRelabeledSeries *ShardedRelabeledSeries,
) (cppbridge.RelabelerStats, error) {
	stats := make([]cppbridge.RelabelerStats, a.head.NumberOfShards())
	t := a.head.CreateTask(
		lssInputRelabeling,
		func(shard TShard) error {
			var (
				lss       = shard.LSS()
				relabeler = shard.Relabeler()
				shardID   = shard.ShardID()
				ok        bool
			)

			if err := lss.WithRLock(func(target, input *cppbridge.LabelSetStorage) (rErr error) {
				stats[shardID], ok, rErr = relabeler.RelabelingFromCache(
					ctx,
					input,
					target,
					state,
					incomingData.ShardedData(),
					shardedInnerSeries.DataBySourceShard(shardID),
				)

				return rErr
			}); err != nil {
				incomingData.Destroy()
				return fmt.Errorf("shard %d: %w", shardID, err)
			}

			if ok {
				incomingData.Destroy()
				return nil
			}

			var (
				hasReallocations bool
				rstats           = cppbridge.RelabelerStats{}
			)
			err := lss.WithLock(func(target, input *cppbridge.LabelSetStorage) (rErr error) {
				rstats, hasReallocations, rErr = relabeler.Relabeling(
					ctx,
					input,
					target,
					state,
					incomingData.ShardedData(),
					shardedInnerSeries.DataBySourceShard(shardID),
					shardedRelabeledSeries.DataByShard(shardID),
				)

				if hasReallocations {
					lss.ResetSnapshot()
				}

				return rErr
			})

			incomingData.Destroy()
			if err != nil {
				return fmt.Errorf("shard %d: %w", shardID, err)
			}

			stats[shardID].Add(rstats)

			return nil
		},
	)
	a.head.Enqueue(t)

	resStats := cppbridge.RelabelerStats{}
	if err := t.Wait(); err != nil {
		return resStats, err
	}

	resStats.Adds(stats)

	return resStats, nil
}

// appendRelabelerSeriesStage second stage - append to lss relabeling ls.
func (a Appender[TTask, TLSS, TShard, THead]) appendRelabelerSeriesStage(
	ctx context.Context,
	shardedInnerSeries *ShardedInnerSeries,
	shardedRelabeledSeries *ShardedRelabeledSeries,
	shardedStateUpdates *ShardedStateUpdates,
) error {
	t := a.head.CreateTask(
		lssAppendRelabelerSeries,
		func(shard TShard) error {
			shardID := shard.ShardID()

			relabeledSeries, ok := shardedRelabeledSeries.DataBySourceShard(shardID)
			if !ok {
				return nil
			}

			lss := shard.LSS()

			return lss.WithLock(func(target, _ *cppbridge.LabelSetStorage) error {
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
					lss.ResetSnapshot()
				}

				return nil
			})
		},
	)
	a.head.Enqueue(t)

	return t.Wait()
}

// updateRelabelerStateStage third stage - update state cache.
func (a Appender[TTask, TLSS, TShard, THead]) updateRelabelerStateStage(
	ctx context.Context,
	state *cppbridge.State,
	shardedStateUpdates *ShardedStateUpdates,
) error {
	numberOfShards := a.head.NumberOfShards()
	for shardID := range numberOfShards {
		updates, ok := shardedStateUpdates.DataBySourceShard(shardID)
		if !ok {
			continue
		}

		if err := state.CacheByShard(shardID).Update(ctx, updates); err != nil {
			return fmt.Errorf("shard %d: %w", shardID, err)
		}
	}

	return nil
}

// appendInnerSeriesAndWriteToWal append [cppbridge.InnerSeries] to [Shard]'s to [DataStorage] and write to [Wal].
func (a Appender[TTask, TLSS, TShard, THead]) appendInnerSeriesAndWriteToWal(
	shardedInnerSeries *ShardedInnerSeries,
) (uint32, error) {
	tw := task.NewTaskWaiter[TTask](2) //revive:disable-line:add-constant // 2 task for wait

	tAppend := a.head.CreateTask(
		dsAppendInnerSeries,
		func(shard TShard) error {
			shard.AppendInnerSeriesSlice(shardedInnerSeries.DataByShard(shard.ShardID()))

			return nil
		},
	)
	a.head.Enqueue(tAppend)

	var atomicLimitExhausted uint32
	tWalWrite := a.head.CreateTask(
		walWrite,
		func(shard TShard) error {
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
	a.head.Enqueue(tWalWrite)

	tw.Add(tAppend)
	tw.Add(tWalWrite)

	return atomicLimitExhausted, tw.Wait()
}

func (a Appender[TTask, TLSS, TShard, THead]) resolveState(state *cppbridge.State) error {
	if state == nil {
		return errNilState
	}

	// TODO delete generationRelabeler 0, state.Reconfigure on lock
	state.Reconfigure(0, a.head.Generation(), a.head.NumberOfShards())

	return nil
}
