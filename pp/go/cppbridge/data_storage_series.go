package cppbridge

import (
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
)

//
// InstantSeriesSet
//

// InstantSeriesSet contains a instatnt set of series, allows to iterate over sorted, populated series.
type InstantSeriesSet struct {
	lssQueryResult              *LSSQueryResult
	labelSetSnapshot            *LabelSetSnapshot
	valueNotFoundTimestampValue int64
	nextSeriesIndex             int
	series                      []InstantSeries
}

// NewInstantSeriesSet init new [InstantSeriesSet].
func NewInstantSeriesSet(
	lssQueryResult *LSSQueryResult,
	labelSetSnapshot *LabelSetSnapshot,
	valueNotFoundTimestampValue int64,
	samples []InstantSeries,
) *InstantSeriesSet {
	return &InstantSeriesSet{
		lssQueryResult:              lssQueryResult,
		labelSetSnapshot:            labelSetSnapshot,
		valueNotFoundTimestampValue: valueNotFoundTimestampValue,
		series:                      make([]InstantSeries, 0, len(samples)),
	}
}

// At returns full series. Returned series should be iterable even after Next is called.
func (ss *InstantSeriesSet) At() storage.Series {
	return &ss.series[ss.nextSeriesIndex-1]
}

// Err the error that iteration as failed with.
func (*InstantSeriesSet) Err() error {
	return nil
}

// Next return true if exist there is a next series and false otherwise.
func (ss *InstantSeriesSet) Next() bool {
	for {
		if ss.nextSeriesIndex >= len(ss.series) {
			return false
		}

		if ss.series[ss.nextSeriesIndex].Timestamp != ss.valueNotFoundTimestampValue {
			break
		}

		ss.nextSeriesIndex++
	}

	lsID, lsLength := ss.lssQueryResult.GetByIndex(ss.nextSeriesIndex)
	ss.series[ss.nextSeriesIndex].LabelSet = NewLabelsWithLSS(
		ss.labelSetSnapshot,
		lsID,
		lsLength,
	)

	ss.nextSeriesIndex++
	return true
}

// Warnings a collection of warnings for the whole set.
func (*InstantSeriesSet) Warnings() annotations.Annotations {
	return nil
}

//
// InstantSeries
//

// InstantSeries is a instant stream of data points belonging to a metric.
type InstantSeries struct {
	Timestamp int64
	Value     float64
	LabelSet  labels.Labels
}

// Iterator is storage.Series interface implementation.
func (s *InstantSeries) Iterator(iterator chunkenc.Iterator) chunkenc.Iterator {
	if i, ok := iterator.(*InstantSeriesChunkIterator); ok {
		i.ResetTo(s.Timestamp, s.Value)
		return i
	}
	return NewInstantSeriesChunkIterator(s.Timestamp, s.Value)
}

// Labels is storage.Series interface implementation.
func (s *InstantSeries) Labels() labels.Labels {
	return s.LabelSet
}

//
// InstantSeriesChunkIterator
//

// InstantSeriesChunkIterator  iterates over the samples of a instant time series, that can only get the next value.
type InstantSeriesChunkIterator struct {
	i int
	t int64
	v float64
}

// NewInstantSeriesChunkIterator init new [InstantSeriesChunkIterator].
func NewInstantSeriesChunkIterator(t int64, v float64) *InstantSeriesChunkIterator {
	return &InstantSeriesChunkIterator{
		i: -1,
		t: t,
		v: v,
	}
}

// At returns the current timestamp/value pair if the value is a float.
//
//nolint:gocritic // unnamedResult not need
func (i *InstantSeriesChunkIterator) At() (int64, float64) {
	return i.t, i.v
}

// AtFloatHistogram returns the current timestamp/value pair if the value is a histogram with floating-point counts.
func (*InstantSeriesChunkIterator) AtFloatHistogram(_ *histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	return 0, nil
}

// AtHistogram returns the current timestamp/value pair if the value is a histogram with integer counts.
func (*InstantSeriesChunkIterator) AtHistogram(_ *histogram.Histogram) (int64, *histogram.Histogram) {
	return 0, nil
}

// AtT returns the current timestamp.
func (i *InstantSeriesChunkIterator) AtT() int64 {
	return i.t
}

// Err returns the current error.
func (*InstantSeriesChunkIterator) Err() error {
	return nil
}

// Next advances the iterator by one and returns the type of the value.
func (i *InstantSeriesChunkIterator) Next() chunkenc.ValueType {
	if i.i < 1 {
		i.i++
	}
	return i.valueType()
}

// ResetTo reset state to timestamp and value.
func (i *InstantSeriesChunkIterator) ResetTo(t int64, v float64) {
	i.i = -1
	i.t = t
	i.v = v
}

// Seek advances the iterator forward to the first sample with a timestamp equal or greater than t.
func (i *InstantSeriesChunkIterator) Seek(t int64) chunkenc.ValueType {
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

func (i *InstantSeriesChunkIterator) valueType() chunkenc.ValueType {
	if i.i == 0 {
		return chunkenc.ValFloat
	}

	return chunkenc.ValNone
}
