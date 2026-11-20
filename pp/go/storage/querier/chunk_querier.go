package querier

import (
	"context"
	"errors"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/util/locker"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

const (
	// lssQueryChunkQuerySelector name of task.
	lssQueryChunkQuerySelector = "lss_query_chunk_query_selector"
	// lssLabelValuesChunkQuerier name of task.
	lssLabelValuesChunkQuerier = "lss_label_values_chunk_querier"
	// lssLabelNamesChunkQuerier name of task.
	lssLabelNamesChunkQuerier = "lss_label_names_chunk_querier"

	// dsQueryChunkQuerier name of task.
	dsQueryChunkQuerier = "data_storage_query_chunk_querier"
)

// ChunkQuerier provides querying access over time series data of a fixed time range.
type ChunkQuerier[
	TTask Task,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
	THead Head[TTask, TDataStorage, TLSS, TShard],
] struct {
	head             THead
	deduplicatorCtor deduplicatorCtor
	mint             int64
	maxt             int64
	metrics          *Metrics
	results          queryResults[storage.ChunkSeries, chunkResultResources]
}

// NewChunkQuerier creates a chunk querier for the given head and time range.
func NewChunkQuerier[
	TTask Task,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
	THead Head[TTask, TDataStorage, TLSS, TShard],
](
	head THead,
	deduplicatorCtor deduplicatorCtor,
	mint, maxt int64,
	metrics *Metrics,
) *ChunkQuerier[TTask, TDataStorage, TLSS, TShard, THead] {
	return &ChunkQuerier[TTask, TDataStorage, TLSS, TShard, THead]{
		head:             head,
		deduplicatorCtor: deduplicatorCtor,
		mint:             mint,
		maxt:             maxt,
		metrics:          metrics,
	}
}

// Close invalidates outstanding chunk selections and releases their native buffers and recoders.
// It must not overlap with use of the querier or its results.
//
//revive:disable-next-line:confusing-naming // other type of querier.
func (q *ChunkQuerier[TTask, TDataStorage, TLSS, TShard, THead]) Close() error {
	q.results.releaseAll(func(resources chunkResultResources) {
		closeChunkSeriesSets(resources.sets)
		closeSerializedData(resources.serializedData)

		pool := q.head.PoolProvider()
		pool.PutChunkSeriesSet(resources.sets)
		pool.PutSerializedData(resources.serializedData)
	})

	return nil
}

// LabelNames returns label values present in the head for the specific label name.
//
//revive:disable-next-line:confusing-naming // other type of querier.
func (q *ChunkQuerier[TTask, TDataStorage, TLSS, TShard, THead]) LabelNames(
	ctx context.Context,
	hints *storage.LabelHints,
	matchers ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return queryLabelNames(
		ctx,
		q.head,
		q.deduplicatorCtor,
		q.metrics.startQuery(operationChunkLabelNames),
		lssLabelNamesChunkQuerier,
		hints,
		matchers...,
	)
}

// LabelValues returns label values present in the head for the specific label name
// that are within the time range mint to maxt. If matchers are specified the returned
// result set is reduced to label values of metrics matching the matchers.
//
//revive:disable:confusing-naming // other type of querier.
func (q *ChunkQuerier[TTask, TDataStorage, TLSS, TShard, THead]) LabelValues(
	ctx context.Context,
	name string,
	hints *storage.LabelHints,
	matchers ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return queryLabelValues(
		ctx,
		name,
		q.head,
		q.deduplicatorCtor,
		q.metrics.startQuery(operationChunkLabelValues),
		lssLabelValuesChunkQuerier,
		hints,
		matchers...,
	)
}

// Select returns a chunk set of series that matches the given label matchers.
// Consume the set before calling [ChunkQuerier.Close]; already emitted chunks own their payloads.
//
//revive:disable-next-line:function-length // this is a complex algorithm.
//revive:disable-next-line:confusing-naming // other type of querier.
func (q *ChunkQuerier[TTask, TDataStorage, TLSS, TShard, THead]) Select(
	ctx context.Context,
	_ bool,
	hints *storage.SelectHints,
	matchers ...*labels.Matcher,
) storage.ChunkSeriesSet {
	if q.results.isClosed {
		return storage.EmptyChunkSeriesSet()
	}

	resources, err := beginSelectQuery(ctx, q.head, q.metrics, operationSelectChunks)
	if err != nil {
		if errors.Is(err, locker.ErrSemaphoreClosed) {
			return &EmptyChunkSeriesSet{}
		}

		logger.Warnf("[ChunkQuerier]: Select failed: %s", err)
		return storage.ErrChunkSeriesSet(err)
	}

	defer resources.releaseTemporary()

	stats := &resources.stats
	poolProvider := resources.pool
	err = queryLss(lssQueryChunkQuerySelector, q.head, matchers, true, &resources)
	if err != nil {
		logger.Warnf("[ChunkQuerier]: failed: %s", err)
		return storage.ErrChunkSeriesSet(err)
	}

	resources.serializedData = poolProvider.GetSerializedData()

	t := queryDataStorage(
		dsQueryChunkQuerier, q.head, resources.lssQueryResults, resources.serializedData, q.mint, q.maxt, hints, stats,
	)
	resources.addTask(t)

	resources.chunkSeriesSets = poolProvider.GetChunkSeriesSet()
	for shardID, serializedData := range resources.serializedData {
		if serializedData == nil {
			resources.chunkSeriesSets[shardID] = &EmptyChunkSeriesSet{}
			continue
		}

		resources.chunkSeriesSets[shardID] = NewChunkSeriesSet(
			resources.snapshots[shardID],
			cppbridge.NewSerializedChunkRecoder(serializedData, cppbridge.TimeInterval{MinT: q.mint, MaxT: q.maxt}),
		)
	}

	result := q.results.register(NewMergeShardChunkSeriesSet(resources.chunkSeriesSets), chunkResultResources{
		sets:           resources.chunkSeriesSets,
		serializedData: resources.serializedData,
	})
	resources.chunkSeriesSets = nil
	resources.serializedData = nil
	stats.lap.Mark(stageBuildResult)

	return result
}
