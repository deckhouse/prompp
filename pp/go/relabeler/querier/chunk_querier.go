package querier

import (
	"context"
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

	lssQueryResults := make([]*cppbridge.LSSQueryResult, q.head.NumberOfShards())
	snapshots := make([]*cppbridge.LabelSetSnapshot, q.head.NumberOfShards())
	convertedMatchers := convertPrometheusMatchersToOpcoreMatchers(matchers...)
	callerID := cppbridge.GetCaller(ctx)

	tLSSQuery := q.head.CreateTask(
		relabeler.LSSQueryChunkQuerier,
		func(shard relabeler.Shard) error {
			shard.LSSRLock()
			defer shard.LSSRUnlock()

			lssQueryResult := shard.LSS().Query(convertedMatchers, callerID)
			if lssQueryResult.Status() != cppbridge.LSSQueryStatusMatch {
				if lssQueryResult.Status() == cppbridge.LSSQueryStatusNoMatch {
					return nil
				}

				return fmt.Errorf(
					"failed to query from shard: %d, query status: %d",
					shard.ShardID(),
					lssQueryResult.Status(),
				)
			}

			shardID := shard.ShardID()
			snapshots[shardID] = shard.LSS().GetSnapshot()
			lssQueryResults[shardID] = lssQueryResult

			return nil
		},
		relabeler.ForLSSTask,
	)
	q.head.Enqueue(tLSSQuery)
	if err := tLSSQuery.Wait(); err != nil {
		logger.Warnf("ChunkQuerier: Select failed: %s", err)
		return storage.ErrChunkSeriesSet(err)
	}

	queryResults := make([]cppbridge.HeadDataStorageSerializedChunks, q.head.NumberOfShards())
	var dataStorageLoadWaiter relabeler.TaskWaiter
	tDataStorageQuery := q.head.CreateTask(
		relabeler.DSQueryChunkQuerier,
		func(shard relabeler.Shard) error {
			shardID := shard.ShardID()
			lssQueryResult := lssQueryResults[shardID]
			if lssQueryResult == nil {
				return nil
			}

			var result cppbridge.DataStorageQueryResult

			shard.DataStorageRLock()
			queryResults[shardID], result = shard.DataStorage().Query(cppbridge.HeadDataStorageQuery{
				StartTimestampMs: q.mint,
				EndTimestampMs:   q.maxt,
				LabelSetIDs:      lssQueryResult.IDs(),
			})
			if result.Status == cppbridge.DataStorageQueryStatusNeedDataLoad {
				dataStorageLoadWaiter.Add(q.head.CreateDataStorageLoadAndQueryTask(shardID, result.Querier))
			}
			shard.DataStorageRUnlock()

			return nil
		},
		relabeler.ForDataStorageTask,
	)
	q.head.Enqueue(tDataStorageQuery)
	_ = tDataStorageQuery.Wait()

	if err := dataStorageLoadWaiter.Wait(); err != nil {
		logger.Warnf("ChunkQuerier: Select: DataStorage load failed: %s", err)
		return storage.ErrChunkSeriesSet(err)
	}

	chunkSeriesSets := make([]storage.ChunkSeriesSet, q.head.NumberOfShards())
	for shardID, serializedChunks := range queryResults {
		if serializedChunks.CBytes == nil || serializedChunks.NumberOfChunks() == 0 {
			chunkSeriesSets[shardID] = &EmptyChunkSeriesSet{}
			continue
		}

		chunkSeriesSets[shardID] = NewChunkSeriesSet(
			lssQueryResults[shardID],
			snapshots[shardID],
			cppbridge.NewSerializedChunkRecoder(serializedChunks.CBytes, cppbridge.TimeInterval{MinT: q.mint, MaxT: q.maxt}),
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
