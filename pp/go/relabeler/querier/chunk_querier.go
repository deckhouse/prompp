package querier

import (
	"context"
	"errors"
	"fmt"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

type ChunkQuerier struct {
	head                relabeler.Head
	deduplicatorFactory DeduplicatorFactory
	mint                int64
	maxt                int64
	closer              func() error
}

func NewChunkQuerier(head relabeler.Head, deduplicatorFactory DeduplicatorFactory, mint, maxt int64, closer func() error) *ChunkQuerier {
	return &ChunkQuerier{
		head:                head,
		deduplicatorFactory: deduplicatorFactory,
		mint:                mint,
		maxt:                maxt,
		closer:              closer,
	}
}

func (q *ChunkQuerier) LabelValues(ctx context.Context, name string, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return labelValues(ctx, name, q.head, q.deduplicatorFactory, nil, relabeler.LSSLabelValuesChunkQuerier, matchers...)
}

func (q *ChunkQuerier) LabelNames(ctx context.Context, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return labelNames(ctx, q.head, q.deduplicatorFactory, nil, relabeler.LSSLabelNamesChunkQuerier, matchers...)
}

func (q *ChunkQuerier) Select(
	ctx context.Context,
	sortSeries bool,
	hints *storage.SelectHints,
	matchers ...*labels.Matcher,
) storage.ChunkSeriesSet {
	runlock, err := q.head.RLockQuery(ctx)
	if err != nil {
		logger.Warnf("[ChunkQuerier]: Select failed: %s", err)
		return storage.ErrChunkSeriesSet(err)
	}
	defer runlock()

	numberOfShards := q.head.NumberOfShards()
	snapshots := make([]*cppbridge.LabelSetSnapshot, numberOfShards)
	lssSelectors := make([]uintptr, numberOfShards)

	convertedMatchers := convertPrometheusMatchersToOpcoreMatchers(matchers...)
	// callerID := cppbridge.GetCaller(ctx)

	tLSSQuerySelector := q.head.CreateTask(
		relabeler.LSSQueryRangeQuerySelector,
		func(shard relabeler.Shard) error {
			selector, status := shard.LSS().QuerySelector(convertedMatchers)
			switch status {
			case cppbridge.LSSQueryStatusMatch:
				lssSelectors[shard.ShardID()] = selector
				snapshots[shard.ShardID()] = shard.LSS().GetSnapshot()
			case cppbridge.LSSQueryStatusNoMatch:
			default:
				return fmt.Errorf(
					"failed to query selector from shard: %d, query status: %d",
					shard.ShardID(),
					status,
				)
			}

			return nil
		},
		relabeler.ForLSSTask,
		relabeler.NonExclusiveTask,
	)
	q.head.Enqueue(tLSSQuerySelector)
	if err := tLSSQuerySelector.Wait(); err != nil {
		logger.Warnf("[ChunkQuerier]: QuerySelector failed: %s", err)
		return storage.ErrChunkSeriesSet(err)
	}

	lssQueryResults := make([]*cppbridge.LSSQueryResult, numberOfShards)
	errs := make([]error, numberOfShards)
	for shardID, selector := range lssSelectors {
		if selector == 0 {
			continue
		}

		lssQueryResult := snapshots[shardID].Query(selector)
		switch lssQueryResult.Status() {
		case cppbridge.LSSQueryStatusMatch:
			lssQueryResults[shardID] = lssQueryResult
		case cppbridge.LSSQueryStatusNoMatch:
		default:
			errs[shardID] = fmt.Errorf(
				"failed to query from shard: %d, query status: %d", shardID, lssQueryResult.Status(),
			)
		}
	}
	if err := errors.Join(errs...); err != nil {
		logger.Warnf("[ChunkQuerier]: Query failed: %s", err)
		return storage.ErrChunkSeriesSet(err)
	}

	serializedChunksShards := make([]*cppbridge.HeadDataStorageSerializedChunks, numberOfShards)
	tDataStorageQuery := q.head.CreateTask(
		relabeler.DSQueryChunkQuerier,
		func(shard relabeler.Shard) error {
			lssQueryResult := lssQueryResults[shard.ShardID()]
			if lssQueryResult == nil {
				return nil
			}

			serializedChunks := shard.DataStorage().Query(cppbridge.HeadDataStorageQuery{
				StartTimestampMs: q.mint,
				EndTimestampMs:   q.maxt,
				LabelSetIDs:      lssQueryResult.IDs(),
			})

			if serializedChunks.NumberOfChunks() == 0 {
				return nil
			}

			serializedChunksShards[shard.ShardID()] = serializedChunks

			return nil
		},
		relabeler.ForDataStorageTask,
		relabeler.NonExclusiveTask,
	)
	q.head.Enqueue(tDataStorageQuery)
	_ = tDataStorageQuery.Wait()

	chunkSeriesSets := make([]storage.ChunkSeriesSet, numberOfShards)
	for shardID, serializedChunks := range serializedChunksShards {
		if serializedChunks == nil {
			chunkSeriesSets[shardID] = &EmptyChunkSeriesSet{}
			continue
		}

		chunkSeriesSets[shardID] = NewChunkSeriesSet(
			lssQueryResults[shardID],
			snapshots[shardID],
			cppbridge.NewSerializedChunkRecoder(serializedChunks, cppbridge.TimeInterval{MinT: q.mint, MaxT: q.maxt}),
		)
	}

	return storage.NewMergeChunkSeriesSet(chunkSeriesSets, storage.NewConcatenatingChunkSeriesMerger())
}

func (q *ChunkQuerier) Close() error {
	if q.closer != nil {
		err := q.closer()
		q.closer = nil
		return err
	}

	return nil
}
