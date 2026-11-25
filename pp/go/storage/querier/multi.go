package querier

import (
	"context"
	"errors"
	"slices"
	"sort"
	"sync"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

// MultiQuerier merges results from child queriers and owns their lifetimes.
// Close must not overlap with selection or iteration.
type MultiQuerier struct {
	queriers []storage.Querier
	results  queryResults[storage.Series, struct{}]
}

// NewMultiQuerier takes ownership of its child queriers, including nested merges.
func NewMultiQuerier(queriers []storage.Querier) *MultiQuerier {
	return &MultiQuerier{
		queriers: slices.Clone(queriers),
	}
}

// Close invalidates all selections and closes every child, joining any errors.
func (q *MultiQuerier) Close() (err error) {
	queriers := q.queriers
	q.queriers = nil
	q.results.releaseAll(nil)

	for _, querier := range queriers {
		if closeErr := querier.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}

	return err
}

// LabelNames returns label values present in the head for the specific label name from all [storage.Querier]s.
func (q *MultiQuerier) LabelNames(
	ctx context.Context,
	hints *storage.LabelHints,
	matchers ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	if len(q.queriers) == 1 {
		labelNames, anns, err := q.queriers[0].LabelNames(ctx, hints, matchers...)
		return DeduplicateAndSortStringSlices(labelNames), anns, err
	}

	labelNamesResults := make([][]string, len(q.queriers))
	annotationResults := make([]annotations.Annotations, len(q.queriers))
	errs := make([]error, len(q.queriers))

	wg := &sync.WaitGroup{}
	for index, querier := range q.queriers {
		wg.Add(1)
		go func(index int, querier storage.Querier) {
			defer wg.Done()
			labelNamesResults[index], annotationResults[index], errs[index] = querier.LabelNames(
				ctx,
				hints,
				matchers...,
			)
		}(index, querier)
	}

	wg.Wait()

	labelNames := DeduplicateAndSortStringSlices(labelNamesResults...)

	return labelNames, nil, errors.Join(errs...)
}

// LabelValues returns label values present in the head for the specific label name
// that are within the time range mint to maxt from all [storage.Querier]s. If matchers are specified the returned
// result set is reduced to label values of metrics matching the matchers.
func (q *MultiQuerier) LabelValues(
	ctx context.Context,
	name string,
	hints *storage.LabelHints,
	matchers ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	if len(q.queriers) == 1 {
		labelValues, anns, err := q.queriers[0].LabelValues(ctx, name, hints, matchers...)
		return DeduplicateAndSortStringSlices(labelValues), anns, err
	}

	labelValuesResults := make([][]string, len(q.queriers))
	annotationResults := make([]annotations.Annotations, len(q.queriers))
	errs := make([]error, len(q.queriers))

	wg := &sync.WaitGroup{}
	for index, querier := range q.queriers {
		wg.Add(1)
		go func(index int, querier storage.Querier) {
			defer wg.Done()
			labelValuesResults[index], annotationResults[index], errs[index] = querier.LabelValues(
				ctx,
				name,
				hints,
				matchers...,
			)
		}(index, querier)
	}

	wg.Wait()

	labelValues := DeduplicateAndSortStringSlices(labelValuesResults...)

	return labelValues, nil, errors.Join(errs...)
}

// Select returns a set of series that matches the given label matchers from all [storage.Querier]s.
func (q *MultiQuerier) Select(
	ctx context.Context,
	sortSeries bool,
	hints *storage.SelectHints,
	matchers ...*labels.Matcher,
) storage.SeriesSet {
	switch len(q.queriers) {
	case 0:
		return storage.EmptySeriesSet()
	case 1:
		return q.results.register(q.queriers[0].Select(ctx, sortSeries, hints, matchers...), struct{}{})
	}

	seriesSets := make([]storage.SeriesSet, len(q.queriers))
	wg := &sync.WaitGroup{}

	for index, querier := range q.queriers {
		wg.Add(1)
		go func(index int, querier storage.Querier) {
			defer wg.Done()
			// storage.NewMergeSeriesSet requires sorted inputs, so the caller's flag is not forwarded.
			seriesSets[index] = querier.Select(ctx, true, hints, matchers...)
		}(index, querier)
	}

	wg.Wait()

	return q.results.register(storage.NewMergeSeriesSet(seriesSets, storage.ChainedSeriesMerge), struct{}{})
}

// DeduplicateAndSortStringSlices merge, deduplicate, and sort rows in slices
// and return a single sorted slice of unique rows.
func DeduplicateAndSortStringSlices(stringSlices ...[]string) []string {
	dedup := make(map[string]struct{})
	for _, stringSlice := range stringSlices {
		for _, value := range stringSlice {
			dedup[value] = struct{}{}
		}
	}

	result := make([]string, 0, len(dedup))
	for value := range dedup {
		result = append(result, value)
	}

	sort.Strings(result)
	return result
}
