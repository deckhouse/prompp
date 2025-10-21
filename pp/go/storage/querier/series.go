package querier

import (
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
	"math"
)

type Series struct {
	labelSet      labels.Labels
	chunkIterator *cppbridge.DataStorageSerializedDataIterator
}

func (s *Series) Next() chunkenc.ValueType {
	hasValue := s.chunkIterator.Next()
	if !hasValue {
		return chunkenc.ValNone
	}
	return chunkenc.ValFloat
}

func (s *Series) Seek(t int64) chunkenc.ValueType {
	for s.AtT() < t {
		if s.Next() == chunkenc.ValNone {
			return chunkenc.ValNone
		}
	}

	return chunkenc.ValFloat
}

func (s *Series) At() (int64, float64) {
	return s.chunkIterator.At()
}

func (s *Series) AtHistogram(_ *histogram.Histogram) (int64, *histogram.Histogram) {
	return 0, nil
}

func (s *Series) AtFloatHistogram(_ *histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	return 0, nil
}

func (s *Series) AtT() int64 {
	ts, _ := s.chunkIterator.At()
	return ts
}

func (s *Series) Err() error {
	return nil
}

func NewSeries(labelSet labels.Labels, chunkIterator *cppbridge.DataStorageSerializedDataIterator) *Series {
	return &Series{
		labelSet:      labelSet,
		chunkIterator: chunkIterator,
	}
}

func (s *Series) Labels() labels.Labels {
	return s.labelSet
}

func (s *Series) Iterator(_ chunkenc.Iterator) chunkenc.Iterator {
	return s
}

type LimitedSeries struct {
	mint, maxt int64
	series     *Series
}

func (ls *LimitedSeries) Next() chunkenc.ValueType {

}

func (ls *LimitedSeries) Seek(t int64) chunkenc.ValueType {
	//TODO implement me
	panic("implement me")
}

func (ls *LimitedSeries) At() (int64, float64) {
	return ls.series.At()
}

func (ls *LimitedSeries) AtHistogram(h *histogram.Histogram) (int64, *histogram.Histogram) {
	return ls.series.AtHistogram(h)
}

func (ls *LimitedSeries) AtFloatHistogram(floatHistogram *histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	return ls.series.AtFloatHistogram(floatHistogram)
}

func (ls *LimitedSeries) AtT() int64 {
	return ls.series.AtT()
}

func (ls *LimitedSeries) Err() error {
	return ls.series.Err()
}

func (ls *LimitedSeries) Labels() labels.Labels {
	return ls.Labels()
}

func (ls *LimitedSeries) Iterator(_ chunkenc.Iterator) chunkenc.Iterator {
	return ls
}

func NewLimitedSeries(mint, maxt int64, series *Series) *LimitedSeries {
	return &LimitedSeries{
		mint:   mint,
		maxt:   maxt,
		series: series,
	}
}

type SeriesSet struct {
	mint, maxt       int64
	lssQueryResult   *cppbridge.LSSQueryResult
	labelSetSnapshot *cppbridge.LabelSetSnapshot
	serializedData   *cppbridge.DataStorageSerializedData

	series storage.Series
}

func NewSeriesSet(
	mint, maxt int64,
	lssQueryResult *cppbridge.LSSQueryResult,
	labelSetSnapshot *cppbridge.LabelSetSnapshot,
	serializedData *cppbridge.DataStorageSerializedData,
) *SeriesSet {
	return &SeriesSet{
		mint:             mint,
		maxt:             maxt,
		lssQueryResult:   lssQueryResult,
		labelSetSnapshot: labelSetSnapshot,
		serializedData:   serializedData,
	}
}

func (s *SeriesSet) Next() bool {
	if s.serializedData == nil {
		return false
	}

	seriesID := s.serializedData.Next()
	if seriesID == math.MaxUint32 {
		return false
	}

	_, lsLength := s.lssQueryResult.GetByIndex(s.lssQueryResult.IndexOf(seriesID))
	s.series = NewLimitedSeries(
		s.mint,
		s.maxt,
		NewSeries(
			labels.NewLabelsWithLSS(s.labelSetSnapshot, seriesID, lsLength),
			s.serializedData.Iterator(),
		),
	)

	return true
}

func (s *SeriesSet) At() storage.Series {
	return s.series
}

func (s *SeriesSet) Err() error {
	return nil
}

func (s *SeriesSet) Warnings() annotations.Annotations {
	return nil
}

// LimitedChunkIterator iterates over the samples of a time series, that can only get the next value with limit.
type LimitedChunkIterator struct {
	chunkIterator chunkenc.Iterator
	mint          int64
	maxt          int64
}

// NewLimitedChunkIterator init new [LimitedChunkIterator].
func NewLimitedChunkIterator(iterator chunkenc.Iterator, mint, maxt int64) *LimitedChunkIterator {
	return &LimitedChunkIterator{
		chunkIterator: iterator,
		mint:          mint,
		maxt:          maxt,
	}
}

// At returns the current timestamp/value pair if the value is a float.
//
//nolint:gocritic // unnamedResult not need
func (i *LimitedChunkIterator) At() (int64, float64) {
	return i.chunkIterator.At()
}

// AtFloatHistogram returns the current timestamp/value pair if the value is a histogram with floating-point counts.
func (i *LimitedChunkIterator) AtFloatHistogram(h *histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	return i.chunkIterator.AtFloatHistogram(h)
}

// AtHistogram returns the current timestamp/value pair if the value is a histogram with integer counts.
func (i *LimitedChunkIterator) AtHistogram(h *histogram.Histogram) (int64, *histogram.Histogram) {
	return i.chunkIterator.AtHistogram(h)
}

// AtT returns the current timestamp.
func (i *LimitedChunkIterator) AtT() int64 {
	return i.chunkIterator.AtT()
}

// Err returns the current error.
func (i *LimitedChunkIterator) Err() error {
	return i.chunkIterator.Err()
}

// Next advances the iterator by one and returns the type of the value.
func (i *LimitedChunkIterator) Next() chunkenc.ValueType {
	if i.chunkIterator.Next() == chunkenc.ValNone {
		return chunkenc.ValNone
	}

	if i.Seek(i.mint) == chunkenc.ValNone {
		return chunkenc.ValNone
	}

	if i.chunkIterator.AtT() > i.maxt {
		return chunkenc.ValNone
	}

	return chunkenc.ValFloat
}

// Seek advances the iterator forward to the first sample with a timestamp equal or greater than t.
func (i *LimitedChunkIterator) Seek(t int64) chunkenc.ValueType {
	if t < i.mint {
		t = i.mint
	}

	if t > i.maxt {
		t = i.maxt
	}

	if i.chunkIterator.Seek(t) == chunkenc.ValNone {
		return chunkenc.ValNone
	}

	if i.chunkIterator.AtT() > i.maxt {
		return chunkenc.ValNone
	}

	return chunkenc.ValFloat
}

//
// InstantSeriesSet
//

// InstantSeriesSet contains a instatnt set of series, allows to iterate over sorted, populated series.
type InstantSeriesSet struct {
	lssQueryResult              *cppbridge.LSSQueryResult
	labelSetSnapshot            *cppbridge.LabelSetSnapshot
	valueNotFoundTimestampValue int64
	samples                     []cppbridge.Sample

	index         int
	currentSeries *InstantSeries
}

// NewInstantSeriesSet init new [InstantSeriesSet].
func NewInstantSeriesSet(
	lssQueryResult *cppbridge.LSSQueryResult,
	labelSetSnapshot *cppbridge.LabelSetSnapshot,
	valueNotFoundTimestampValue int64,
	samples []cppbridge.Sample,
) *InstantSeriesSet {
	return &InstantSeriesSet{
		lssQueryResult:              lssQueryResult,
		labelSetSnapshot:            labelSetSnapshot,
		valueNotFoundTimestampValue: valueNotFoundTimestampValue,
		samples:                     samples,
		index:                       -1,
	}
}

// At returns full series. Returned series should be iterable even after Next is called.
func (ss *InstantSeriesSet) At() storage.Series {
	return ss.currentSeries
}

// Err the error that iteration as failed with.
func (*InstantSeriesSet) Err() error {
	return nil
}

// Next return true if exist there is a next series and false otherwise.
func (ss *InstantSeriesSet) Next() bool {
	for {
		if ss.index+1 >= ss.lssQueryResult.Len() {
			return false
		}

		ss.index++
		if ss.samples[ss.index].Timestamp != ss.valueNotFoundTimestampValue {
			break
		}
	}

	lsID, lsLength := ss.lssQueryResult.GetByIndex(ss.index)
	ss.currentSeries = &InstantSeries{
		labelSet: labels.NewLabelsWithLSS(
			ss.labelSetSnapshot,
			lsID,
			lsLength,
		),
		sample: ss.samples[ss.index],
	}

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
	labelSet labels.Labels
	sample   cppbridge.Sample
}

// Iterator is storage.Series interface implementation.
func (s *InstantSeries) Iterator(iterator chunkenc.Iterator) chunkenc.Iterator {
	if i, ok := iterator.(*InstantSeriesChunkIterator); ok {
		i.ResetTo(s.sample.Timestamp, s.sample.Value)
		return i
	}
	return NewInstantSeriesChunkIterator(s.sample.Timestamp, s.sample.Value)
}

// Labels is storage.Series interface implementation.
func (s *InstantSeries) Labels() labels.Labels {
	return s.labelSet
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
