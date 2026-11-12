// Package upsampler holds the predicate shared between pp/go/storage/querier
// and pp-pkg/blocks/upsampler for deciding whether a query's SelectHints need
// Upsampler treatment. The actual interpolation (Querier/SeriesSet/Iterator)
// lives entirely in pp-pkg/blocks/upsampler.
package upsampler

import (
	"maps"

	"github.com/prometheus/prometheus/storage"
)

// counterFuncs read the series as a counter and correct every value decrease as a counter
// reset. Interpolating a drop down for them would spread the reset over every synthetic
// sample, so such a drop is held flat instead.
var counterFuncs = map[string]struct{}{
	"rate":     {},
	"increase": {},
	"irate":    {},
}

// gaugeFuncs describe the trend of a gauge over the window, where a decrease is ordinary
// data: for them a gap is interpolated in both directions, exactly like a rise.
var gaugeFuncs = map[string]struct{}{
	"delta":  {},
	"deriv":  {},
	"idelta": {},
}

// overTimeFuncs aggregate the values inside the window rather than their trend. A straight
// line through a gap would feed them values that were never measured, so synthetic samples
// hold the last known value instead; only the emptiness of the window is what they fix.
var overTimeFuncs = map[string]struct{}{
	"avg_over_time":      {},
	"min_over_time":      {},
	"max_over_time":      {},
	"sum_over_time":      {},
	"count_over_time":    {},
	"quantile_over_time": {},
	"stddev_over_time":   {},
	"stdvar_over_time":   {},
	"mad_over_time":      {},
	"last_over_time":     {},
	"present_over_time":  {},
}

// allowedFuncs is the union of the three groups above — the range-vector functions for
// which synthetic samples don't change the meaning of the result, only whether there is
// one at all. It is derived rather than written out, so a function added to a group can
// never be classified without being allowed, or allowed without a filling rule.
// changes/resets stay out of every group: they count discrete events between samples, and
// a synthetic sample would fabricate information there instead of merely smoothing it.
var allowedFuncs = func() map[string]struct{} {
	funcs := make(map[string]struct{}, len(counterFuncs)+len(gaugeFuncs)+len(overTimeFuncs))
	maps.Copy(funcs, counterFuncs)
	maps.Copy(funcs, gaugeFuncs)
	maps.Copy(funcs, overTimeFuncs)

	return funcs
}()

// NeedsUpsampling reports whether hints describe a query for which gaps wider
// than the function's range may need synthetic samples inserted to avoid a
// spurious NaN/empty result.
func NeedsUpsampling(hints *storage.SelectHints) bool {
	if hints == nil || hints.Range <= 0 {
		return false
	}

	_, ok := allowedFuncs[hints.Func]

	return ok
}

// IsCounterFunc reports whether hints describe a function that treats a value decrease
// as a counter reset, so that a drop inside a gap has to be held flat.
func IsCounterFunc(hints *storage.SelectHints) bool {
	if hints == nil {
		return false
	}

	_, ok := counterFuncs[hints.Func]

	return ok
}

// IsOverTimeFunc reports whether hints describe an _over_time function, for which a gap is
// filled by holding the last known value instead of interpolating towards the next sample.
func IsOverTimeFunc(hints *storage.SelectHints) bool {
	if hints == nil {
		return false
	}

	_, ok := overTimeFuncs[hints.Func]

	return ok
}
