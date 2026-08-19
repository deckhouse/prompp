package upsampler

import (
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

// SeriesSet wraps a [storage.SeriesSet] and provides interpolation capability.
type SeriesSet struct {
	base        storage.SeriesSet
	stepMS      uint32
	maxGapMS    uint32
	counterFunc bool
}

// NewSeriesSet wraps a base [storage.SeriesSet] for interpolation of gaps between
// rangeMS/2 and resolutionMS*2. The thresholds are derived once here and carried down as
// durations, so that every per-series [Series] stays small. counterFunc keeps a value drop
// inside a gap flat.
func NewSeriesSet(base storage.SeriesSet, rangeMS, resolutionMS int64, counterFunc bool) *SeriesSet {
	stepMS, maxGapMS := gapThresholds(rangeMS, resolutionMS)

	return &SeriesSet{
		base:        base,
		stepMS:      stepMS,
		maxGapMS:    maxGapMS,
		counterFunc: counterFunc,
	}
}

// Next advances the iterator by one and returns false if there are no more values.
func (ss *SeriesSet) Next() bool {
	return ss.base.Next()
}

// At returns the current [storage.Series].
func (ss *SeriesSet) At() storage.Series {
	return &Series{
		base:        ss.base.At(),
		stepMS:      ss.stepMS,
		maxGapMS:    ss.maxGapMS,
		counterFunc: ss.counterFunc,
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
