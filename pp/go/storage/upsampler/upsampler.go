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
// treat the two samples nearest the eval time as an instantaneous rate, and
// changes/resets count discrete events between samples — interpolation would
// fabricate information for those instead of merely smoothing it, so they are
// deliberately excluded.
var allowedFuncs = map[string]struct{}{
	"rate":     {},
	"increase": {},
	"delta":    {},
	"deriv":    {},
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
