package querier

import (
	"context"

	"github.com/prometheus/prometheus/model/labels"
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

func (q *ChunkQuerier) LabelValues(
	ctx context.Context,
	name string,
	hints *storage.LabelHints,
	matchers ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return labelValues(
		ctx,
		name,
		q.head,
		q.deduplicatorFactory,
		nil,
		relabeler.LSSLabelValuesChunkQuerier,
		hints,
		matchers...,
	)
}

func (q *ChunkQuerier) LabelNames(
	ctx context.Context,
	hints *storage.LabelHints,
	matchers ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return labelNames(
		ctx,
		q.head,
		q.deduplicatorFactory,
		nil,
		relabeler.LSSLabelNamesChunkQuerier,
		hints,
		matchers...,
	)
}

func (q *ChunkQuerier) Select(
	ctx context.Context,
	_ bool,
	_ *storage.SelectHints,
	matchers ...*labels.Matcher,
) storage.ChunkSeriesSet {
	runlock, err := q.head.RLockQuery(ctx)
	if err != nil {
		logger.Warnf("[ChunkQuerier]: Select failed: %s", err)
		return storage.ErrChunkSeriesSet(err)
	}
	defer runlock()

	//lssQueryResults, snapshots, err := lssQuery(relabeler.LSSQueryChunkQuerySelector, q.head, matchers)
	//if err != nil {
	//	logger.Warnf("[ChunkQuerier]: failed: %s", err)
	//	return storage.ErrChunkSeriesSet(err)
	//}
	//
	//queryResults, err := dataStorageQuery(relabeler.DSQueryChunkQuerier, q.head, lssQueryResults, q.mint, q.maxt)
	//if err != nil {
	//	return storage.ErrChunkSeriesSet(err)
	//}

	chunkSeriesSets := make([]storage.ChunkSeriesSet, q.head.NumberOfShards())
	//for shardID, serializedChunks := range queryResults {
	//	if serializedChunks == nil || serializedChunks.NumberOfChunks() == 0 {
	//		chunkSeriesSets[shardID] = &EmptyChunkSeriesSet{}
	//		continue
	//	}
	//
	//chunkSeriesSets[shardID] = NewChunkSeriesSet(
	//	lssQueryResults[shardID],
	//	snapshots[shardID],
	//	cppbridge.NewSerializedChunkRecoder(serializedChunks, cppbridge.TimeInterval{MinT: q.mint, MaxT: q.maxt}),
	//)
	//}

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
