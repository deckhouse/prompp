// Package upsampler holds the predicate shared between pp/go/storage/querier
// and pp-pkg/blocks/upsampler for deciding whether a query's SelectHints need
// Upsampler treatment. The actual interpolation (Querier/SeriesSet/Iterator)
// lives entirely in pp-pkg/blocks/upsampler.
package upsampler

import "github.com/prometheus/prometheus/storage"

// allowedFuncs is the allow-list of range-vector functions for which linear
// interpolation of missing samples doesn't change the meaning of the result:
// rate/increase/delta average over the whole range regardless of how many
// real samples fall inside it, and deriv fits the same trend through
// synthesized points as it would through the two real ones. irate/idelta
// report the two samples nearest the eval time as an instantaneous rate — on
// interpolated points that degrades to the average rate of the surrounding
// gap, which is still closer to the truth than the NaN they return on sparse
// data. changes/resets count discrete events between samples: interpolation
// would fabricate information there instead of merely smoothing it, so they
// stay excluded.
var allowedFuncs = map[string]struct{}{
	"rate":     {},
	"increase": {},
	"delta":    {},
	"deriv":    {},
	"irate":    {},
	"idelta":   {},
}

// counterFuncs is the subset of allowedFuncs that reads the series as a counter and
// corrects every value decrease as a counter reset. Interpolating a drop down for them
// would spread the reset over every synthetic sample, so such a drop is held flat
// instead. delta/deriv/idelta describe gauges, where a decrease is ordinary data and
// has to be interpolated like a rise.
var counterFuncs = map[string]struct{}{
	"rate":     {},
	"increase": {},
	"irate":    {},
}

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
// as a counter reset.
func IsCounterFunc(hints *storage.SelectHints) bool {
	if hints == nil {
		return false
	}

	_, ok := counterFuncs[hints.Func]

	return ok
}
