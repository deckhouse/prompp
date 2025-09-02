package appender

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/head/task"
	"github.com/prometheus/prometheus/pp/go/storage/logger"
)

const (
	// DSAppendInnerSeries name of task.
	DSAppendInnerSeries = "data_storage_append_inner_series"

	// LSSInputRelabeling name of task.
	LSSInputRelabeling = "lss_input_relabeling"
	// LSSAppendRelabelerSeries name of task.
	LSSAppendRelabelerSeries = "lss_append_relabeler_series"

	// WalWrite name of task.
	WalWrite = "wal_write"

	// WalCommit name of task.
	WalCommit = "wal_commit"
)

//
// GenericTask
//

// GenericTask the minimum required task [Generic] implementation.
type GenericTask interface {
	// Wait for the task to complete on all shards.
	Wait() error
}

//
// DataStorage
//

// DataStorage the minimum required [DataStorage] implementation.
type DataStorage interface {
	// AppendInnerSeriesSlice add InnerSeries to storage.
	AppendInnerSeriesSlice(innerSeriesSlice []*cppbridge.InnerSeries)
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
// LSS
//

// Wal the minimum required Wal implementation for a [Shard].
type Wal interface {
	// Commit finalize segment from encoder and write to wal.
	// It is necessary to lock the LSS for reading for the commit.
	Commit() error

	// Flush wal segment writer, write all buffered data to storage.
	Flush() error

	// Write append the incoming inner series to wal encoder.
	Write(innerSeriesSlice []*cppbridge.InnerSeries) (bool, error)
}

//
// Shard
//

// Shard the minimum required head [Shard] implementation.
type Shard[TDataStorage DataStorage, TLSS LSS, TWal Wal] interface {
	// DataStorage returns shard [DataStorage].
	DataStorage() TDataStorage

	// LSS returns shard labelset storage [LSS].
	LSS() TLSS

	// Relabeler returns relabeler for shard goroutines.
	Relabeler() *cppbridge.PerGoroutineRelabeler

	// ShardID returns the shard ID.
	ShardID() uint16

	// Wal returns write-ahead log.
	Wal() TWal
}

//
// Head
//

// Head the minimum required [Head] implementation.
type Head[
	TGenericTask GenericTask,
	TDataStorage DataStorage,
	TLSS LSS,
	TWal Wal,
	TShard Shard[TDataStorage, TLSS, TWal],
] interface {
	// CreateTask create a task for operations on the [Head] shards.
	CreateTask(taskName string, shardFn func(shard TShard) error) TGenericTask

	// Enqueue the task to be executed on shards [Head].
	Enqueue(t TGenericTask)

	// NumberOfShards returns current number of shards in to [Head].
	NumberOfShards() uint16
}

//
// Appender
//

type Appender[
	TGenericTask GenericTask,
	TDataStorage DataStorage,
	TLSS LSS,
	TWal Wal,
	TShard Shard[TDataStorage, TLSS, TWal],
	THead Head[TGenericTask, TDataStorage, TLSS, TWal, TShard],
] struct {
	head THead
}

// Append incoming data to head.
func (a *Appender[TGenericTask, TDataStorage, TLSS, TWal, TShard, THead]) Append(
	ctx context.Context,
	incomingData *storage.IncomingData,
	incomingState *cppbridge.State,
	relabelerID string,
	commitToWal bool,
) ([][]*cppbridge.InnerSeries, cppbridge.RelabelerStats, error) {
	// rd, state, err := h.resolveRelabelersData(incomingState, relabelerID)
	// if err != nil {
	// 	return nil, cppbridge.RelabelerStats{}, err
	// }

	// TODO ?
	var state *cppbridge.State

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

	tw := task.NewTaskWaiter[TGenericTask](2)

	tAppend := a.head.CreateTask(
		DSAppendInnerSeries,
		func(shard TShard) error {
			shard.DataStorage().AppendInnerSeriesSlice(shardedInnerSeries.DataByShard(shard.ShardID()))

			return nil
		},
	)
	a.head.Enqueue(tAppend)

	var atomiclimitExhausted uint32
	tWalWrite := a.head.CreateTask(
		WalWrite,
		func(shard TShard) error {
			limitExhausted, errWrite := shard.Wal().Write(shardedInnerSeries.DataByShard(shard.ShardID()))
			if errWrite != nil {
				return fmt.Errorf("shard %d: %w", shard.ShardID(), errWrite)
			}

			if limitExhausted {
				atomic.AddUint32(&atomiclimitExhausted, 1)
			}

			return nil
		},
	)
	a.head.Enqueue(tWalWrite)

	tw.Add(tAppend)
	tw.Add(tWalWrite)

	if err := tw.Wait(); err != nil {
		logger.Errorf("failed to write wal: %v", err)
	}

	if commitToWal || atomiclimitExhausted > 0 {
		t := a.head.CreateTask(
			WalCommit,
			func(shard TShard) error {
				swal := shard.Wal()

				// wal contains LSS and it is necessary to lock the LSS for reading for the commit.
				if err := shard.LSS().WithRLock(func(_, _ *cppbridge.LabelSetStorage) error {
					return swal.Commit()
				}); err != nil {
					return err
				}

				return swal.Flush()
			},
		)
		a.head.Enqueue(t)

		if err := t.Wait(); err != nil {
			logger.Errorf("failed to commit wal: %v", err)
		}
	}

	return shardedInnerSeries.Data(), stats, nil
}

// inputRelabelingStage first stage - relabeling.
func (a *Appender[TGenericTask, TDataStorage, TLSS, TWal, TShard, THead]) inputRelabelingStage(
	ctx context.Context,
	state *cppbridge.State,
	incomingData *DestructibleIncomingData,
	shardedInnerSeries *ShardedInnerSeries,
	shardedRelabeledSeries *ShardedRelabeledSeries,
) (cppbridge.RelabelerStats, error) {
	stats := make([]cppbridge.RelabelerStats, a.head.NumberOfShards())
	t := a.head.CreateTask(
		LSSInputRelabeling,
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
func (a *Appender[TGenericTask, TDataStorage, TLSS, TWal, TShard, THead]) appendRelabelerSeriesStage(
	ctx context.Context,
	shardedInnerSeries *ShardedInnerSeries,
	shardedRelabeledSeries *ShardedRelabeledSeries,
	shardedStateUpdates *ShardedStateUpdates,
) error {
	t := a.head.CreateTask(
		LSSAppendRelabelerSeries,
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
func (a *Appender[TGenericTask, TDataStorage, TLSS, TWal, TShard, THead]) updateRelabelerStateStage(
	ctx context.Context,
	state *cppbridge.State,
	shardedStateUpdates *ShardedStateUpdates,
) error {
	numberOfShards := a.head.NumberOfShards()
	for shardID := uint16(0); shardID < numberOfShards; shardID++ {
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
