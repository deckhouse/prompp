package fanout

import (
	"context"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

//
// noopQuerier
//

// noopQuerier is a [storage.Querier] that does nothing.
type noopQuerier struct{}

// NoopQuerier is a [storage.Querier] that does nothing.
func NoopQuerier() storage.Querier {
	return noopQuerier{}
}

// Close returns nil. Implements [storage.Querier].
func (noopQuerier) Close() error {
	return nil
}

// LabelNames returns nil. Implements [storage.Querier].
func (noopQuerier) LabelNames(
	context.Context,
	*storage.LabelHints,
	...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

// LabelValues returns nil. Implements [storage.Querier].
func (noopQuerier) LabelValues(
	context.Context,
	string,
	*storage.LabelHints,
	...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

// Select returns [noopSeriesSet]. Implements [storage.Querier].
func (noopQuerier) Select(context.Context, bool, *storage.SelectHints, ...*labels.Matcher) storage.SeriesSet {
	return NoopSeriesSet()
}

//
// noopSeriesSet
//

// noopSeriesSet is a [storage.SeriesSet] that does nothing.
type noopSeriesSet struct{}

// NoopSeriesSet is a [storage.SeriesSet] that does nothing.
func NoopSeriesSet() storage.SeriesSet {
	return noopSeriesSet{}
}

// At returns nil. Implements [storage.SeriesSet].
func (noopSeriesSet) At() storage.Series { return nil }

// Error returns nil. Implements [storage.SeriesSet].
func (noopSeriesSet) Err() error { return nil }

// Next returns false. Implements [storage.SeriesSet].
func (noopSeriesSet) Next() bool { return false }

// Warnings returns an empty [annotations.Annotations]. Implements [storage.SeriesSet].
func (noopSeriesSet) Warnings() annotations.Annotations { return nil }
