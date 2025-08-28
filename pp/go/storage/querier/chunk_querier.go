package querier

import (
	"context"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/logger"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

const (
	// LSSQueryChunkQuerySelector name of task.
	LSSQueryChunkQuerySelector = "lss_query_chunk_query_selector"
	// LSSLabelValuesChunkQuerier name of task.
	LSSLabelValuesChunkQuerier = "lss_label_values_chunk_querier"
	// LSSLabelNamesChunkQuerier name of task.
	LSSLabelNamesChunkQuerier = "lss_label_names_chunk_querier"

	// DSQueryChunkQuerier name of task.
	DSQueryChunkQuerier = "data_storage_query_chunk_querier"
)

// ChunkQuerier provides querying access over time series data of a fixed time range.
type ChunkQuerier[
	TGenericTask GenericTask,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
	THead Head[TGenericTask, TDataStorage, TLSS, TShard],
] struct {
	head             THead
	deduplicatorCtor deduplicatorCtor
	mint             int64
	maxt             int64
	closer           func() error
}

// NewChunkQuerier init new [ChunkQuerier].
func NewChunkQuerier[
	TGenericTask GenericTask,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
	THead Head[TGenericTask, TDataStorage, TLSS, TShard],
](
	head THead,
	deduplicatorCtor deduplicatorCtor,
	mint, maxt int64,
	closer func() error,
) *ChunkQuerier[TGenericTask, TDataStorage, TLSS, TShard, THead] {
	return &ChunkQuerier[TGenericTask, TDataStorage, TLSS, TShard, THead]{
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
func (q *ChunkQuerier[TGenericTask, TDataStorage, TLSS, TShard, THead]) Close() error {
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
func (q *ChunkQuerier[TGenericTask, TDataStorage, TLSS, TShard, THead]) LabelNames(
	ctx context.Context,
	hints *storage.LabelHints,
	matchers ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return queryLabelNames(
		ctx,
		q.head,
		q.deduplicatorCtor,
		nil,
		LSSLabelNamesChunkQuerier,
		hints,
		matchers...,
	)
}

// LabelValues returns label values present in the head for the specific label name
// that are within the time range mint to maxt. If matchers are specified the returned
// result set is reduced to label values of metrics matching the matchers.
//
//revive:disable:confusing-naming // other type of querier.
func (q *ChunkQuerier[TGenericTask, TDataStorage, TLSS, TShard, THead]) LabelValues(
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
		LSSLabelValuesChunkQuerier,
		hints,
		matchers...,
	)
}

// Select returns a chunk set of series that matches the given label matchers.
//
//revive:disable-next-line:confusing-naming // other type of querier.
func (q *ChunkQuerier[TGenericTask, TDataStorage, TLSS, TShard, THead]) Select(
	ctx context.Context,
	_ bool,
	_ *storage.SelectHints,
	matchers ...*labels.Matcher,
) storage.ChunkSeriesSet {
	release, err := q.head.AcquireQuery(ctx)
	if err != nil {
		logger.Warnf("[ChunkQuerier]: Select failed: %s", err)
		return storage.ErrChunkSeriesSet(err)
	}
	defer release()

	lssQueryResults, snapshots, err := queryLss(LSSQueryChunkQuerySelector, q.head, matchers)
	if err != nil {
		logger.Warnf("[ChunkQuerier]: failed: %s", err)
		return storage.ErrChunkSeriesSet(err)
	}

	serializedChunksShards := queryDataStorage(DSQueryChunkQuerier, q.head, lssQueryResults, q.mint, q.maxt)
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
