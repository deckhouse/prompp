package querier

import (
	"context"
	"errors"
	"sort"
	"sync"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

//
// MultiQuerier
//

// MultiQuerier querier which makes requests to all queriers from the created list and merges the received results.
type MultiQuerier struct {
	queriers []storage.Querier
	closer   func() error
}

// NewMultiQuerier init new [MultiQuerier].
func NewMultiQuerier(queriers []storage.Querier, closer func() error) *MultiQuerier {
	qs := make([]storage.Querier, 0, len(queriers))
	for _, q := range queriers {
		if rawQ, ok := q.(*MultiQuerier); ok {
			qs = append(qs, rawQ.queriers...)
			continue
		}

		qs = append(qs, q)
	}

	return &MultiQuerier{
		queriers: qs,
		closer:   closer,
	}
}

// Close closes all [storage.Querier]s if need.
func (q *MultiQuerier) Close() (err error) {
	for _, querier := range q.queriers {
		err = errors.Join(err, querier.Close())
	}

	if q.closer != nil {
		err = errors.Join(err, q.closer())
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
		return q.queriers[0].LabelNames(ctx, hints, matchers...)
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
		return q.queriers[0].LabelValues(ctx, name, hints, matchers...)
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
	if len(q.queriers) == 1 {
		return q.queriers[0].Select(ctx, sortSeries, hints, matchers...)
	}

	seriesSets := make([]storage.SeriesSet, len(q.queriers))
	wg := &sync.WaitGroup{}

	for index, querier := range q.queriers {
		wg.Add(1)
		go func(index int, querier storage.Querier) {
			defer wg.Done()
			seriesSets[index] = querier.Select(ctx, sortSeries, hints, matchers...)
		}(index, querier)
	}

	wg.Wait()

	return storage.NewMergeSeriesSet(seriesSets, storage.ChainedSeriesMerge)
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
