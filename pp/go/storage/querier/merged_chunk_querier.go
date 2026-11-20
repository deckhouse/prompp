package querier

import (
	"context"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

// mergedChunkQuerier owns the outward selections of the Prom++ head chunk merge.
// Close must not overlap with selection or iteration.
type mergedChunkQuerier struct {
	delegate storage.ChunkQuerier
	results  queryResults[storage.ChunkSeries, struct{}]
}

// NewMergedChunkQuerier takes ownership of the supplied head queriers and concatenates matching chunks.
// As with [storage.NewMergeChunkQuerier] secondaries, issue all selections before iterating the first result.
func NewMergedChunkQuerier(queriers []storage.ChunkQuerier) storage.ChunkQuerier {
	delegate := storage.NewMergeChunkQuerier(nil, queriers, storage.NewConcatenatingChunkSeriesMerger())
	if len(queriers) == 0 {
		return delegate
	}

	return &mergedChunkQuerier{delegate: delegate}
}

// Close invalidates all selections before delegating to the child queriers.
func (q *mergedChunkQuerier) Close() error {
	delegate := q.delegate
	if delegate == nil {
		return nil
	}

	q.delegate = nil
	q.results.releaseAll(nil)

	return delegate.Close()
}

// LabelNames delegates to the open chunk merge.
func (q *mergedChunkQuerier) LabelNames(
	ctx context.Context,
	hints *storage.LabelHints,
	matchers ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	if q.delegate == nil {
		return nil, nil, nil
	}

	return q.delegate.LabelNames(ctx, hints, matchers...)
}

// LabelValues delegates to the open chunk merge.
func (q *mergedChunkQuerier) LabelValues(
	ctx context.Context,
	name string,
	hints *storage.LabelHints,
	matchers ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	if q.delegate == nil {
		return nil, nil, nil
	}

	return q.delegate.LabelValues(ctx, name, hints, matchers...)
}

// Select returns a chunk selection that stops advancing when the querier closes.
func (q *mergedChunkQuerier) Select(
	ctx context.Context,
	sortSeries bool,
	hints *storage.SelectHints,
	matchers ...*labels.Matcher,
) storage.ChunkSeriesSet {
	if q.delegate == nil {
		return storage.EmptyChunkSeriesSet()
	}

	return q.results.register(q.delegate.Select(ctx, sortSeries, hints, matchers...), struct{}{})
}
