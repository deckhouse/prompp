package querier

import (
	"context"
	"errors"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/util/locker"
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
	closer           func() error
}

// NewChunkQuerier init new [ChunkQuerier].
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
	closer func() error,
) *ChunkQuerier[TTask, TDataStorage, TLSS, TShard, THead] {
	return &ChunkQuerier[TTask, TDataStorage, TLSS, TShard, THead]{
		head:             head,
		deduplicatorCtor: deduplicatorCtor,
		mint:             mint,
		maxt:             maxt,
		closer:           closer,
	}
}

// Close [ChunkQuerier] if need.
//
//revive:disable-next-line:confusing-naming // other type of querier.
func (q *ChunkQuerier[TTask, TDataStorage, TLSS, TShard, THead]) Close() error {
	if q.closer != nil {
		err := q.closer()
		q.closer = nil
		return err
	}

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
		nil,
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
		nil,
		lssLabelValuesChunkQuerier,
		hints,
		matchers...,
	)
}

// Select returns a chunk set of series that matches the given label matchers.
//
//revive:disable-next-line:confusing-naming // other type of querier.
func (q *ChunkQuerier[TTask, TDataStorage, TLSS, TShard, THead]) Select(
	ctx context.Context,
	_ bool,
	_ *storage.SelectHints,
	matchers ...*labels.Matcher,
) storage.ChunkSeriesSet {
	release, err := q.head.AcquireQuery(ctx)
	if err != nil {
		if errors.Is(err, locker.ErrSemaphoreClosed) {
			return &EmptyChunkSeriesSet{}
		}

		logger.Warnf("[ChunkQuerier]: Select failed: %s", err)
		return storage.ErrChunkSeriesSet(err)
	}
	defer release()

	lssQueryResults, snapshots, err := queryLss(lssQueryChunkQuerySelector, q.head, matchers)
	if err != nil {
		logger.Warnf("[ChunkQuerier]: failed: %s", err)
		return storage.ErrChunkSeriesSet(err)
	}

	serializedChunksShards := queryDataStorage(dsQueryChunkQuerier, q.head, lssQueryResults, q.mint, q.maxt)
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
