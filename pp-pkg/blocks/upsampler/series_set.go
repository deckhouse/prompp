package upsampler

import (
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

// SeriesSet wraps a [storage.SeriesSet] and provides interpolation capability.
type SeriesSet struct {
	base         storage.SeriesSet
	rangeMS      int64
	resolutionMS int64
	counterFunc  bool
}

// NewSeriesSet wraps a base [storage.SeriesSet] for interpolation of gaps between
// rangeMS/2 and resolutionMS*2. counterFunc keeps a value drop inside a gap flat.
func NewSeriesSet(base storage.SeriesSet, rangeMS, resolutionMS int64, counterFunc bool) *SeriesSet {
	return &SeriesSet{
		base:         base,
		rangeMS:      rangeMS,
		resolutionMS: resolutionMS,
		counterFunc:  counterFunc,
	}
}

// Next advances the iterator by one and returns false if there are no more values.
func (ss *SeriesSet) Next() bool {
	return ss.base.Next()
}

// At returns the current [storage.Series].
func (ss *SeriesSet) At() storage.Series {
	return &Series{
		base:         ss.base.At(),
		rangeMS:      ss.rangeMS,
		resolutionMS: ss.resolutionMS,
		counterFunc:  ss.counterFunc,
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
