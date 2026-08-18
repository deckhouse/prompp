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
// into Select() results when hints match the allow-list and gaps exceed hints.Range.
type Querier struct {
	base         storage.Querier
	resolutionMS int64 // nominal resolution of the underlying data source (0 for head querier)
}

// NewQuerier wraps a base Querier. resolutionMS is used as a coarse filter
// (optimization only): when resolutionMS <= hints.Range, gaps wider than Range
// are unlikely by construction, so we skip the Upsampler wrapper. Pass 0 to skip
// this filter entirely (used for head querier, where there is no nominal resolution).
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
		return NewSeriesSet(base, hints.Range)
	}

	return base
}

// shouldWrap decides whether to wrap the SeriesSet based on hints and
// the coarse resolution filter. It returns true only when:
//  1. The function is in the allow-list (checked via headupsampler.NeedsUpsampling).
//  2. Either resolutionMS <= 0 (no filter, e.g., head querier), or
//     resolutionMS > hints.Range (gaps wider than the range are likely).
func (q *Querier) shouldWrap(hints *storage.SelectHints) (*storage.SelectHints, bool) {
	if !headupsampler.NeedsUpsampling(hints) {
		return hints, false
	}

	//revive:disable-next-line:add-constant // resolutionMS must be x2 so that there are 2 points for the query
	shouldWrap := q.resolutionMS*2 >= hints.Range
	if shouldWrap {
		hints = &storage.SelectHints{
			Start:           hints.Start - q.resolutionMS,
			End:             hints.End,
			Limit:           hints.Limit,
			Step:            hints.Step,
			Func:            hints.Func,
			Grouping:        hints.Grouping,
			Range:           hints.Range,
			ShardCount:      hints.ShardCount,
			ShardIndex:      hints.ShardIndex,
			LookbackDelta:   hints.LookbackDelta,
			DisableTrimming: hints.DisableTrimming,
			By:              hints.By,
			IsSubquery:      hints.IsSubquery,
		}
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
