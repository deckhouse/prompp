package upsampler

import (
	"math"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

// SeriesSet wraps a [storage.SeriesSet] and provides interpolation capability.
type SeriesSet struct {
	base       storage.SeriesSet
	stepMS     uint32
	maxGapMS   uint32
	typeOfFunc FuncKind
}

// NewSeriesSet wraps a base [storage.SeriesSet] for interpolation of gaps between
// rangeMS/2 and resolutionMS*2. The thresholds are derived once here and carried down as
// durations, so that every per-series [Series] stays small. typeOfFunc says how a gap is
// filled: interpolated, or held at the last known value.
func NewSeriesSet(base storage.SeriesSet, rangeMS, resolutionMS int64, typeOfFunc FuncKind) *SeriesSet {
	stepMS, maxGapMS := gapThresholds(rangeMS, resolutionMS)

	return &SeriesSet{
		base:       base,
		stepMS:     stepMS,
		maxGapMS:   maxGapMS,
		typeOfFunc: typeOfFunc,
	}
}

// Next advances the iterator by one and returns false if there are no more values.
func (ss *SeriesSet) Next() bool {
	return ss.base.Next()
}

// At returns the current [storage.Series].
func (ss *SeriesSet) At() storage.Series {
	return &Series{
		base:       ss.base.At(),
		stepMS:     ss.stepMS,
		maxGapMS:   ss.maxGapMS,
		typeOfFunc: ss.typeOfFunc,
	}
}

// Err returns any accumulated error.
func (ss *SeriesSet) Err() error {
	return ss.base.Err()
}

// Warnings returns annotations from the base SeriesSet.
func (ss *SeriesSet) Warnings() annotations.Annotations {
	return ss.base.Warnings()
}

// gapThresholds converts the millisecond query parameters into the pair of gap thresholds
// the iterator works with: the synthesis step and the widest gap still interpolated. They
// are 32-bit because both are durations, not timestamps. Parameters that don't fit — a range
// above ~99 days or a resolution above ~24 days — yield zeros, which disarms synthesis
// instead of wrapping around into a step that means nothing.
func gapThresholds(rangeMS, resolutionMS int64) (stepMS, maxGapMS uint32) {
	step := rangeMS / synthesisStepDivisor
	maxGap := resolutionMS * maxGapResolutions
	if step < 0 || step > math.MaxUint32 || maxGap < 0 || maxGap > math.MaxUint32 {
		return 0, 0
	}

	return uint32(step), uint32(maxGap) // #nosec G115 // both values are range-checked right above
}
