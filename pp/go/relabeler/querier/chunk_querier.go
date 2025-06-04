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
	lssQueryResults := make([]*cppbridge.LSSQueryResult, q.head.NumberOfShards())
	snapshots := make([]*cppbridge.LabelSetSnapshot, q.head.NumberOfShards())
	convertedMatchers := convertPrometheusMatchersToOpcoreMatchers(matchers...)
	callerID := cppbridge.GetCaller(ctx)

	err := q.head.ForEachShard(relabeler.LSSQueryChunkQuerierSelect, func(shard relabeler.Shard) error {
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

		lssQueryResults[shard.ShardID()] = lssQueryResult
		snapshots[shard.ShardID()] = shard.LSS().GetSnapshot()

		return nil
	})
	if err != nil {
		logger.Warnf("ChunkQuerier: Select failed: %s", err)
		return storage.ErrChunkSeriesSet(err)
	}

	serializedChunksShards := make([]*cppbridge.HeadDataStorageSerializedChunks, q.head.NumberOfShards())

	_ = q.head.ForEachShard(relabeler.DataStorageQueryChunkQuerierSelect, func(shard relabeler.Shard) error {
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
	})

	chunkSeriesSets := make([]storage.ChunkSeriesSet, q.head.NumberOfShards())
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
