package querier

import (
	"math"
	"runtime"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
)

// ChunkIterator iterates over the samples of a time series, that can only get the next value with limit.
type ChunkIterator struct {
	serializedData  *cppbridge.DataStorageSerializedData
	chunkIterator   cppbridge.DataStorageSerializedDataIterator
	iterationResult cppbridge.SerializedDataIteratorIterationResult
	mint            int64
	maxt            int64
}

// NewChunkIterator init new [ChunkIterator].
func NewChunkIterator(serializedData *cppbridge.DataStorageSerializedData, chunkRef uint32, mint, maxt int64) *ChunkIterator {
	it := &ChunkIterator{
		serializedData:  serializedData,
		chunkIterator:   cppbridge.NewDataStorageSerializedDataIterator(serializedData, chunkRef),
		iterationResult: cppbridge.NewSerializedDataIteratorIterationResult(),
		mint:            mint,
		maxt:            maxt,
	}

	runtime.SetFinalizer(it, func(it *ChunkIterator) {
		it.chunkIterator.Destroy()
	})

	return it
}

func (it *ChunkIterator) Reset(serializedData *cppbridge.DataStorageSerializedData, chunkRef uint32, mint, maxt int64) {
	it.serializedData = serializedData
	it.mint = mint
	it.maxt = maxt
	it.chunkIterator.Reset(serializedData, chunkRef)
	it.iterationResult = cppbridge.NewSerializedDataIteratorIterationResult()
}

// At returns the current timestamp/value pair if the value is a float.
//
//nolint:gocritic // unnamedResult not need
func (it *ChunkIterator) At() (int64, float64) {
	return it.iterationResult.Timestamp, it.iterationResult.Value
}

// AtFloatHistogram returns the current timestamp/value pair if the value is a histogram with floating-point counts.
func (it *ChunkIterator) AtFloatHistogram(h *histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	return 0, nil
}

// AtHistogram returns the current timestamp/value pair if the value is a histogram with integer counts.
func (it *ChunkIterator) AtHistogram(h *histogram.Histogram) (int64, *histogram.Histogram) {
	return 0, nil
}

// AtT returns the current timestamp.
func (it *ChunkIterator) AtT() int64 {
	return it.iterationResult.Timestamp
}

// Err returns the current error.
func (it *ChunkIterator) Err() error {
	return nil
}

func (it *ChunkIterator) next() chunkenc.ValueType {
	it.chunkIterator.Next(&it.iterationResult)
	if it.iterationResult.HasValue {
		return chunkenc.ValFloat
	}
	return chunkenc.ValNone
}

// Next advances the iterator by one and returns the type of the value.
func (it *ChunkIterator) Next() chunkenc.ValueType {
	if it.next() == chunkenc.ValNone {
		return chunkenc.ValNone
	}

	ts := it.AtT()
	if ts < it.mint {
		if it.Seek(it.mint) == chunkenc.ValNone {
			return chunkenc.ValNone
		}
		ts = it.AtT()
	}

	if ts > it.maxt {
		return chunkenc.ValNone
	}

	return chunkenc.ValFloat
}

func (it *ChunkIterator) seek(t int64) chunkenc.ValueType {
	ts := it.AtT()
	// check if iterator is not initialized or is not reached t.
	if ts == math.MinInt64 || ts < t {
		it.chunkIterator.Seek(t, &it.iterationResult)
	}

	if it.iterationResult.HasValue {
		return chunkenc.ValFloat
	}
	return chunkenc.ValNone
}

// Seek advances the iterator forward to the first sample with a timestamp equal or greater than t.
func (it *ChunkIterator) Seek(t int64) chunkenc.ValueType {
	// adjust lower limit.
	if t < it.mint {
		t = it.mint
	}

	ts := it.AtT()
	if ts == math.MinInt64 || ts < t {
		if it.seek(t) == chunkenc.ValNone {
			return chunkenc.ValNone
		}
		ts = it.AtT()
	}

	if ts > it.maxt {
		return chunkenc.ValNone
	}

	return chunkenc.ValFloat
}

type Series struct {
	mint, maxt     int64
	labelSet       labels.Labels
	serializedData *cppbridge.DataStorageSerializedData
	chunkRef       uint32
}

func NewSeries(mint, maxt int64, labelSet labels.Labels, serializedData *cppbridge.DataStorageSerializedData, chunkRef uint32) Series {
	return Series{
		mint:           mint,
		maxt:           maxt,
		labelSet:       labelSet,
		serializedData: serializedData,
		chunkRef:       chunkRef,
	}
}

func (s *Series) Labels() labels.Labels {
	return s.labelSet
}

func (s *Series) Iterator(it chunkenc.Iterator) chunkenc.Iterator {
	chunkIterator, ok := it.(*ChunkIterator)
	if !ok {
		return NewChunkIterator(
			s.serializedData,
			s.chunkRef,
			s.mint,
			s.maxt,
		)
	}

	chunkIterator.Reset(s.serializedData, s.chunkRef, s.mint, s.maxt)
	return chunkIterator
}

type SeriesSet struct {
	mint, maxt       int64
	lssQueryResult   *cppbridge.LSSQueryResult
	labelSetSnapshot *cppbridge.LabelSetSnapshot
	serializedData   *cppbridge.DataStorageSerializedData

	lastIndexFromLSSQueryResult int
	series                      []Series
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
		series:           make([]Series, 0, lssQueryResult.Len()),
	}
}

func (s *SeriesSet) Next() bool {
	if s.serializedData == nil {
		return false
	}

	seriesID, chunkRef := s.serializedData.Next()
	if seriesID == math.MaxUint32 {
		return false
	}

	var lsLength uint16
	lsLength, s.lastIndexFromLSSQueryResult = s.lssQueryResult.LengthBySeriesID(seriesID, s.lastIndexFromLSSQueryResult)
	if s.lastIndexFromLSSQueryResult < 0 {
		logger.Errorf("not found label set for series id: %d", seriesID)
		return false
	}

	s.series = append(s.series, NewSeries(
		s.mint,
		s.maxt,
		cppbridge.NewLabelsWithLSS(s.labelSetSnapshot, seriesID, lsLength),
		s.serializedData,
		chunkRef,
	))

	return true
}

func (s *SeriesSet) At() storage.Series {
	return &s.series[len(s.series)-1]
}

func (s *SeriesSet) Err() error {
	return nil
}

func (s *SeriesSet) Warnings() annotations.Annotations {
	return nil
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
	nextSampleIndex             int
	series                      []InstantSeries
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
		series:                      make([]InstantSeries, 0, len(samples)),
	}
}

// At returns full series. Returned series should be iterable even after Next is called.
func (ss *InstantSeriesSet) At() storage.Series {
	return &ss.series[len(ss.series)-1]
}

// Err the error that iteration as failed with.
func (*InstantSeriesSet) Err() error {
	return nil
}

// Next return true if exist there is a next series and false otherwise.
func (ss *InstantSeriesSet) Next() bool {
	for {
		if ss.nextSampleIndex >= ss.lssQueryResult.Len() {
			return false
		}

		if ss.samples[ss.nextSampleIndex].Timestamp != ss.valueNotFoundTimestampValue {
			break
		}
		ss.nextSampleIndex++
	}

	lsID, lsLength := ss.lssQueryResult.GetByIndex(ss.nextSampleIndex)
	ss.series = append(ss.series, InstantSeries{
		labelSet: cppbridge.NewLabelsWithLSS(
			ss.labelSetSnapshot,
			lsID,
			lsLength,
		),
		sample: &ss.samples[ss.nextSampleIndex],
	})

	ss.nextSampleIndex++
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
	sample   *cppbridge.Sample
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
