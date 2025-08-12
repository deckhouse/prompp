package appender

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/logger"
)

const (
	// DSAppendInnerSeries name of task.
	DSAppendInnerSeries = "data_storage_append_inner_series"

	// LSSInputRelabeling name of task.
	LSSInputRelabeling = "lss_input_relabeling"

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
	// TODO
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
}

//
// Shard
//

// Shard the minimum required head [Shard] implementation.
type Shard[TDataStorage DataStorage, TLSS LSS] interface {
	// DataStorage returns shard [DataStorage].
	DataStorage() TDataStorage

	// LSS returns shard labelset storage [LSS].
	LSS() TLSS

	// ShardID returns the shard ID.
	ShardID() uint16
}

//
// Head
//

// Head the minimum required [Head] implementation.
type Head[
	TGenericTask GenericTask,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
] interface {
	// CreateTask create a task for operations on the [Head] shards.
	CreateTask(taskName string, shardFn func(shard TShard) error) TGenericTask

	// Enqueue the task to be executed on shards [Head].
	Enqueue(t TGenericTask)

	// NumberOfShards returns current number of shards in to [Head].
	NumberOfShards() uint16
}

type Appender[
	TGenericTask GenericTask,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
	THead Head[TGenericTask, TDataStorage, TLSS, TShard],
] struct {
	head THead
}

// Append incoming data to head.
func (a *Appender[TGenericTask, TDataStorage, TLSS, TShard, THead]) Append(
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

	shardedInnerSeries := NewShardedInnerSeries(a.head.NumberOfShards())
	shardedRelabeledSeries := NewShardedRelabeledSeries(a.head.NumberOfShards())

	stats, err := a.inputRelabelingStage(
		ctx,
		state,
		rd,
		NewDestructibleIncomingData(incomingData, int(a.head.NumberOfShards())),
		shardedInnerSeries,
		shardedRelabeledSeries,
	)
	if err != nil {
		// reset msr.rotateWG on error
		return nil, stats, fmt.Errorf("failed input relabeling stage: %w", err)
	}

	if !shardedRelabeledSeries.IsEmpty() {
		shardedStateUpdates := NewShardedStateUpdates(a.head.NumberOfShards())
		if err = h.appendRelabelerSeriesStage(
			ctx,
			rd,
			shardedInnerSeries,
			shardedRelabeledSeries,
			shardedStateUpdates,
		); err != nil {
			return nil, stats, fmt.Errorf("failed append relabeler series stage: %w", err)
		}

		if err = h.updateRelabelerStateStage(
			ctx,
			state,
			rd,
			shardedStateUpdates,
		); err != nil {
			return nil, stats, fmt.Errorf("failed update relabeler stage: %w", err)
		}
	}

	tw := relabeler.NewTaskWaiter(2)

	tAppend := h.CreateTask(
		DSAppendInnerSeries,
		func(shard relabeler.Shard) error {
			shard.DataStorageLock()
			shard.DataStorage().AppendInnerSeriesSlice(shardedInnerSeries.DataByShard(shard.ShardID()))
			shard.DataStorageUnlock()

			return nil
		},
		relabeler.ForDataStorageTask,
	)
	h.Enqueue(tAppend)

	var atomiclimitExhausted uint32
	tWalWrite := h.CreateTask(
		WalWrite,
		func(shard relabeler.Shard) error {
			shard.LSSLock()
			limitExhausted, errWrite := shard.Wal().Write(shardedInnerSeries.DataByShard(shard.ShardID()))
			shard.LSSUnlock()
			if errWrite != nil {
				return fmt.Errorf("shard %d: %w", shard.ShardID(), errWrite)
			}

			if limitExhausted {
				atomic.AddUint32(&atomiclimitExhausted, 1)
			}

			return nil
		},
		relabeler.ForLSSTask,
	)
	h.Enqueue(tWalWrite)

	tw.Add(tAppend)
	tw.Add(tWalWrite)

	if err := tw.Wait(); err != nil {
		logger.Errorf("failed to write wal: %v", err)
	}

	if commitToWal || atomiclimitExhausted > 0 {
		t := h.CreateTask(
			WalCommit,
			func(shard relabeler.Shard) error {
				shard.LSSLock()
				defer shard.LSSUnlock()

				return shard.Wal().Commit()
			},
			relabeler.ForLSSTask,
		)
		h.Enqueue(t)

		if err := t.Wait(); err != nil {
			logger.Errorf("failed to commit wal: %v", err)
		}
	}

	return shardedInnerSeries.Data(), stats, nil
}

// inputRelabelingStage first stage - relabeling.
func (a *Appender[TGenericTask, TDataStorage, TLSS, TShard, THead]) inputRelabelingStage(
	ctx context.Context,
	state *cppbridge.State,
	rd *RelabelerData,
	incomingData *DestructibleIncomingData,
	shardedInnerSeries *ShardedInnerSeries,
	shardedRelabeledSeries *ShardedRelabeledSeries,
) (cppbridge.RelabelerStats, error) {
	stats := make([]cppbridge.RelabelerStats, a.head.NumberOfShards())
	t := a.head.CreateTask(
		LSSInputRelabeling,
		func(shard TShard) error {
			var (
				lss              = shard.LSS()
				shardID          = shard.ShardID()
				hasReallocations bool
				ok               bool
			)

			if err := lss.WithRLock(func(target, input *cppbridge.LabelSetStorage) error {
				var rErr error
				if state.TrackStaleness() {
					stats[shardID], ok, rErr = rd.InputRelabelerByShard(
						shardID,
					).InputRelabelingWithStalenansFromCache(
						ctx,
						input,
						target,
						state.CacheByShard(shardID),
						state.RelabelerOptions(),
						state.StaleNansStateByShard(shardID),
						state.DefTimestamp(),
						incomingData.ShardedData(),
						shardedInnerSeries.DataBySourceShard(shardID),
					)
				} else {
					stats[shardID], ok, rErr = rd.InputRelabelerByShard(shardID).InputRelabelingFromCache(
						ctx,
						input,
						target,
						state.CacheByShard(shardID),
						state.RelabelerOptions(),
						incomingData.ShardedData(),
						shardedInnerSeries.DataBySourceShard(shardID),
					)
				}

				return rErr
			}); err != nil {
				incomingData.Destroy()
				return fmt.Errorf("shard %d: %w", shardID, err)
			}

			if ok {
				incomingData.Destroy()
				return nil
			}

			shard.LSSLock()
			defer shard.LSSUnlock()
			rstats := cppbridge.RelabelerStats{}

			if state.TrackStaleness() {
				rstats, hasReallocations, err = rd.InputRelabelerByShard(shardID).InputRelabelingWithStalenans(
					ctx,
					shard.LSS().Input(),
					shard.LSS().Target(),
					state.CacheByShard(shardID),
					state.RelabelerOptions(),
					state.StaleNansStateByShard(shardID),
					state.DefTimestamp(),
					incomingData.ShardedData(),
					shardedInnerSeries.DataBySourceShard(shardID),
					shardedRelabeledSeries.DataByShard(shardID),
				)
			} else {
				rstats, hasReallocations, err = rd.InputRelabelerByShard(shardID).InputRelabeling(
					ctx,
					shard.LSS().Input(),
					shard.LSS().Target(),
					state.CacheByShard(shardID),
					state.RelabelerOptions(),
					incomingData.ShardedData(),
					shardedInnerSeries.DataBySourceShard(shardID),
					shardedRelabeledSeries.DataByShard(shardID),
				)
			}

			incomingData.Destroy()
			if err != nil {
				return fmt.Errorf("shard %d: %w", shardID, err)
			}

			stats[shardID].SamplesAdded += rstats.SamplesAdded
			stats[shardID].SeriesAdded += rstats.SeriesAdded
			stats[shardID].SeriesDrop += rstats.SeriesDrop

			if hasReallocations {
				shard.LSS().ResetSnapshot()
			}

			return nil
		},
	)
	a.head.Enqueue(t)

	resStats := cppbridge.RelabelerStats{}
	if err := t.Wait(); err != nil {
		return resStats, err
	}

	for _, s := range stats {
		resStats.SamplesAdded += s.SamplesAdded
		resStats.SeriesAdded += s.SeriesAdded
		resStats.SeriesDrop += s.SeriesDrop
	}

	return resStats, nil
}
