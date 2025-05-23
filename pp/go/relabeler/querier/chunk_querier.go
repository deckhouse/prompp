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
	return labelValues(ctx, name, q.head, q.deduplicatorFactory, nil, matchers...)
}

func (q *ChunkQuerier) LabelNames(ctx context.Context, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	return labelNames(ctx, q.head, q.deduplicatorFactory, nil, matchers...)
}

func (q *ChunkQuerier) Select(
	ctx context.Context,
	sortSeries bool,
	hints *storage.SelectHints,
	matchers ...*labels.Matcher,
) storage.ChunkSeriesSet {
	chunkSeriesSets := make([]storage.ChunkSeriesSet, q.head.NumberOfShards())
	convertedMatchers := convertPrometheusMatchersToOpcoreMatchers(matchers...)
	callerID := cppbridge.GetCaller(ctx)

	err := q.head.ReadEachShard(func(shard relabeler.Shard) error {
		lssQueryResult := shard.LSS().Query(convertedMatchers, callerID)

		if lssQueryResult.Status() != cppbridge.LSSQueryStatusMatch {
			chunkSeriesSets[shard.ShardID()] = EmptyChunkSeriesSet{}
			if lssQueryResult.Status() == cppbridge.LSSQueryStatusNoMatch {
				return nil
			}
			return fmt.Errorf(
				"failed to query from shard: %d, query status: %d",
				shard.ShardID(),
				lssQueryResult.Status(),
			)
		}

		serializedChunks := shard.DataStorage().Query(cppbridge.HeadDataStorageQuery{
			StartTimestampMs: q.mint,
			EndTimestampMs:   q.maxt,
			LabelSetIDs:      lssQueryResult.IDs(),
		})

		if serializedChunks.NumberOfChunks() == 0 {
			chunkSeriesSets[shard.ShardID()] = EmptyChunkSeriesSet{}
			return nil
		}

		chunkRecoder := cppbridge.NewSerializedChunkRecoder(serializedChunks, cppbridge.TimeInterval{
			MinT: q.mint,
			MaxT: q.maxt,
		})

		chunkSeriesSets[shard.ShardID()] = NewChunkSeriesSet(lssQueryResult, chunkRecoder)

		return nil
	})
	if err != nil {
		logger.Warnf("QUERIER: Select failed: %s", err)
		return storage.ErrChunkSeriesSet(err)
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
