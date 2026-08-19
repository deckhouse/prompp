// Package upsampler provides a storage.Querier wrapper that interpolates
// synthetic samples into results when querying downsampled or sparse data
// for specific range-vector functions (rate, increase, delta, deriv).
package upsampler

import (
	"context"

	"github.com/prometheus/prometheus/model/labels"
	headupsampler "github.com/prometheus/prometheus/pp/go/storage/upsampler"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

// Querier wraps a [storage.Querier] and conditionally injects interpolation logic
// into Select() results when hints match the allow-list and gaps exceed half of
// hints.Range.
type Querier struct {
	base         storage.Querier
	resolutionMS int64 // nominal resolution of the sparsest underlying data source
}

// NewQuerier wraps a base Querier. resolutionMS is used as a coarse filter
// (optimization only): when resolutionMS*2 < hints.Range, the data is denser than
// the function window requires, so we skip the Upsampler wrapper. It is also the
// amount by which the left border of the query is extended back, so that the first
// sample of the window has a predecessor to interpolate from, and the bound on the
// gap width the [Iterator] still interpolates (resolutionMS*2).
func NewQuerier(base storage.Querier, resolutionMS int64) storage.Querier {
	return &Querier{
		base:         base,
		resolutionMS: resolutionMS,
	}
}

// Select returns a [storage.SeriesSet], optionally wrapped to inject synthetic samples.
func (q *Querier) Select(
	ctx context.Context,
	sortSeries bool,
	hints *storage.SelectHints,
	matchers ...*labels.Matcher,
) storage.SeriesSet {
	hints, shouldWrap := q.shouldWrap(hints)
	base := q.base.Select(ctx, sortSeries, hints, matchers...)

	if shouldWrap {
		return NewSeriesSet(base, hints.Range, q.resolutionMS, headupsampler.IsCounterFunc(hints))
	}

	return base
}

// shouldWrap decides whether to wrap the SeriesSet based on hints and
// the coarse resolution filter, and returns the hints to query base with.
// It wraps only when:
//  1. The function is in the allow-list (checked via headupsampler.NeedsUpsampling).
//  2. resolutionMS*2 >= hints.Range — a window of hints.Range wide is not
//     guaranteed to hold two real samples, so gaps have to be filled.
func (q *Querier) shouldWrap(hints *storage.SelectHints) (*storage.SelectHints, bool) {
	if !headupsampler.NeedsUpsampling(hints) {
		return hints, false
	}

	//revive:disable-next-line:add-constant // resolutionMS must be x2 so that there are 2 points for the query
	shouldWrap := q.resolutionMS*2 >= hints.Range
	if shouldWrap {
		// Extend the left border back by one resolution so that the first sample
		// of the window has a predecessor to interpolate from.
		extended := *hints
		extended.Start -= q.resolutionMS
		hints = &extended
	}

	return hints, shouldWrap
}

// LabelValues returns label values, delegating to base.
func (q *Querier) LabelValues(
	ctx context.Context,
	name string,
	hints *storage.LabelHints,
	matchers ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return q.base.LabelValues(ctx, name, hints, matchers...)
}

// LabelNames returns label names, delegating to base.
func (q *Querier) LabelNames(
	ctx context.Context,
	hints *storage.LabelHints,
	matchers ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return q.base.LabelNames(ctx, hints, matchers...)
}

// Close closes the underlying querier.
func (q *Querier) Close() error {
	return q.base.Close()
}

//
// ResolutionQuerier
//

// ResolutionQuerier wrappes a [storage.Querier] with a resolution.
type ResolutionQuerier struct {
	storage.Querier
	resolutionMS int64 // nominal resolution of the sparsest underlying data source
}

// NewResolutionQuerier wraps a base [storage.Querier] with a resolution.
func NewResolutionQuerier(q storage.Querier, resolutionMS int64) storage.Querier {
	return &ResolutionQuerier{
		Querier:      q,
		resolutionMS: resolutionMS,
	}
}

// Resolution returns the nominal resolution of the underlying data source.
func (q *ResolutionQuerier) Resolution() int64 {
	return q.resolutionMS
}
