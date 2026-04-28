package querier

import (
	"math"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
)

// floatStaleNaN is the float64 representation of the [value.StaleNaN] value.
var floatStaleNaN = math.Float64frombits(value.StaleNaN)

// MakeTimestampsSliceWithDefault creates a slice with the default timestamp.
func MakeTimestampsSliceWithDefault(size int, defaultTimestamp int64) []int64 {
	timestamps := make([]int64, size)
	for i := range timestamps {
		timestamps[i] = defaultTimestamp
	}

	return timestamps
}

// NewStalenanSeriesSliceFromTimestamps creates StalenanSeries slice from timestamps.
func NewStalenanSeriesSliceFromTimestamps(timestamps []int64) []StalenanSeries {
	seriesSlice := make([]StalenanSeries, len(timestamps))
	for i := range seriesSlice {
		seriesSlice[i].timestamp = timestamps[i]
	}

	return seriesSlice
}

//
// StalenanSeriesSet
//

// StalenanSeriesSet contains a set of series that always return the staleNaN value for the specified timestamps.
type StalenanSeriesSet struct {
	series                      []StalenanSeries
	lssQueryResult              *cppbridge.LSSQueryResult
	labelSetSnapshot            *cppbridge.LabelSetSnapshot
	valueNotFoundTimestampValue int64
	nextSeriesIndex             int
}

// NewStalenanSeriesSet creates a new [StalenanSeriesSet].
func NewStalenanSeriesSet(
	series []StalenanSeries,
	lssQueryResult *cppbridge.LSSQueryResult,
	labelSetSnapshot *cppbridge.LabelSetSnapshot,
	valueNotFoundTimestampValue int64,
) *StalenanSeriesSet {
	return &StalenanSeriesSet{
		series:                      series,
		lssQueryResult:              lssQueryResult,
		labelSetSnapshot:            labelSetSnapshot,
		valueNotFoundTimestampValue: valueNotFoundTimestampValue,
	}
}

// At returns the current [StalenanSeries], should be iterable even after Next is called.
// [storage.SeriesSet] interface implementation.
func (ss *StalenanSeriesSet) At() storage.Series {
	return &ss.series[ss.nextSeriesIndex-1]
}

// Err the error that iteration as failed with - always nil.
// [storage.SeriesSet] interface implementation.
func (*StalenanSeriesSet) Err() error {
	return nil
}

// Next advances the iterator by one and returns false if there are no more values.
// [storage.SeriesSet] interface implementation.
func (ss *StalenanSeriesSet) Next() bool {
	for {
		if ss.nextSeriesIndex >= len(ss.series) {
			return false
		}

		if ss.series[ss.nextSeriesIndex].timestamp != ss.valueNotFoundTimestampValue {
			break
		}

		ss.nextSeriesIndex++
	}

	lsID, _ := ss.lssQueryResult.GetByIndex(ss.nextSeriesIndex)
	builder := builderPool.Get().(*labels.ScratchBuilder)
	builder.Reset()
	ss.series[ss.nextSeriesIndex].labelSet = labels.NewLabelsWithLSS(
		ss.labelSetSnapshot,
		lsID,
		builder,
	)
	ss.nextSeriesIndex++
	builderPool.Put(builder)

	return true
}

// Warnings a collection of warnings for the whole set - always nil.
// [storage.SeriesSet] interface implementation.
func (*StalenanSeriesSet) Warnings() annotations.Annotations {
	return nil
}

//
// StalenanSeries
//

// StalenanSeries is a series that always returns the staleNaN value for the specified timestamps.
type StalenanSeries struct {
	timestamp int64
	labelSet  labels.Labels
}

// Iterator returns an iterator that iterates over the samples of the series.
// [storage.Series] interface implementation.
func (s *StalenanSeries) Iterator(iterator chunkenc.Iterator) chunkenc.Iterator {
	if i, ok := iterator.(*StalenanSeriesChunkIterator); ok {
		i.ResetTo(s.timestamp)
		return i
	}

	return NewStalenanSeriesChunkIterator(s.timestamp)
}

// Labels returns the complete set of labels. For series it means all labels identifying the series.
// [storage.Series] interface implementation.
func (s *StalenanSeries) Labels() labels.Labels {
	return s.labelSet
}

//
// StalenanSeriesChunkIterator
//

// StalenanSeriesChunkIterator iterates over the samples time series,
// which always returns the stalenan value for the specified timestamps.
type StalenanSeriesChunkIterator struct {
	i int
	t int64
}

// NewStalenanSeriesChunkIterator init new [StalenanSeriesChunkIterator].
func NewStalenanSeriesChunkIterator(t int64) *StalenanSeriesChunkIterator {
	return &StalenanSeriesChunkIterator{
		i: -1,
		t: t,
	}
}

// At returns the current timestamp/value pair if the value is a float.
// Always returns the staleNaN value for the specified timestamp.
// [chunkenc.Iterator] interface implementation.
//
//nolint:gocritic // unnamedResult not need
func (i *StalenanSeriesChunkIterator) At() (int64, float64) {
	return i.t, floatStaleNaN
}

// AtFloatHistogram returns the current timestamp/value pair if the value is a histogram with floating-point counts.
// [chunkenc.Iterator] interface implementation.
func (*StalenanSeriesChunkIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	return 0, nil
}

// AtHistogram returns the current timestamp/value pair if the value is a histogram with integer counts.
// [chunkenc.Iterator] interface implementation.
func (*StalenanSeriesChunkIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	return 0, nil
}

// AtT returns the current timestamp.
// [chunkenc.Iterator] interface implementation.
func (i *StalenanSeriesChunkIterator) AtT() int64 {
	return i.t
}

// Err returns the current error. Always nil.
// [chunkenc.Iterator] interface implementation.
func (*StalenanSeriesChunkIterator) Err() error {
	return nil
}

// Next advances the iterator by one and returns the type of the value.
// [chunkenc.Iterator] interface implementation.
func (i *StalenanSeriesChunkIterator) Next() chunkenc.ValueType {
	if i.i < 1 {
		i.i++
	}

	return i.valueType()
}

// ResetTo reset state to timestamp.
// [chunkenc.Iterator] interface implementation.
func (i *StalenanSeriesChunkIterator) ResetTo(t int64) {
	i.i = -1
	i.t = t
}

// Seek advances the iterator forward to the first sample with a timestamp equal or greater than t.
// [chunkenc.Iterator] interface implementation.
func (i *StalenanSeriesChunkIterator) Seek(t int64) chunkenc.ValueType {
	if i.valueType() == chunkenc.ValFloat && i.t >= t {
		return chunkenc.ValFloat
	}

	for {
		if i.Next() == chunkenc.ValNone {
			return chunkenc.ValNone
		}

		if i.t >= t {
			return chunkenc.ValFloat
		}
	}
}

// valueType returns the type of the value at the current position.
func (i *StalenanSeriesChunkIterator) valueType() chunkenc.ValueType {
	if i.i == 0 {
		return chunkenc.ValFloat
	}

	return chunkenc.ValNone
}
