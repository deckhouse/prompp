package querier

import (
	"math"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
)

//
// AggrSeriesSet
//

// AggrSeriesSet contains a set of aggregated series.
// [storage.SeriesSet] interface implementation.
type AggrSeriesSet struct {
	labelSetSnapshot *cppbridge.LabelSetSnapshot
	serializedData   *cppbridge.DataStorageSerializedData
	mint             int64
	maxt             int64

	lastIndexFromLSSQueryResult int
	series                      []AggrSeries
}

// NewAggrSeriesSet init new [AggrSeriesSet].
func NewAggrSeriesSet(
	labelSetSnapshot *cppbridge.LabelSetSnapshot,
	serializedData *cppbridge.DataStorageSerializedData,
	lssQueryResult *cppbridge.LSSQueryResult,
	mint, maxt int64,
) *AggrSeriesSet {
	return &AggrSeriesSet{
		labelSetSnapshot:            labelSetSnapshot,
		serializedData:              serializedData,
		mint:                        mint,
		maxt:                        maxt,
		series:                      make([]AggrSeries, 0, lssQueryResult.Len()),
		lastIndexFromLSSQueryResult: 0,
	}
}

// At returns the current aggregated series.
// [storage.SeriesSet] interface implementation.
func (ss *AggrSeriesSet) At() storage.Series {
	return &ss.series[len(ss.series)-1]
}

// Err returns the error of the [AggrSeriesSet] - always nil.
// [storage.SeriesSet] interface implementation.
func (*AggrSeriesSet) Err() error {
	return nil
}

// Next advances the iterator by one and returns false if there are no more values.
// [storage.SeriesSet] interface implementation.
func (ss *AggrSeriesSet) Next() bool {
	if ss.serializedData == nil {
		return false
	}

	seriesID, chunkRef := ss.serializedData.Next()
	if seriesID == math.MaxUint32 {
		return false
	}

	builder := builderPool.Get().(*labels.ScratchBuilder)
	builder.Reset()
	ss.series = append(ss.series, NewAggrSeries(
		labels.NewLabelsWithLSS(ss.labelSetSnapshot, seriesID, builder),
		ss.serializedData,
		ss.mint,
		ss.maxt,
		chunkRef,
	))
	builderPool.Put(builder)

	return true
}

// Warnings returns the warnings of the [AggrSeriesSet] - always nil.
// [storage.SeriesSet] interface implementation.
func (*AggrSeriesSet) Warnings() annotations.Annotations {
	return nil
}

//
// AggrSeries
//

// AggrSeries represents a time series with aggregated samples.
// [storage.Series] interface implementation.
type AggrSeries struct {
	labelSet       labels.Labels
	serializedData *cppbridge.DataStorageSerializedData
	mint           int64
	maxt           int64
	chunkRef       uint32
}

// NewAggrSeries init new [AggrSeries].
func NewAggrSeries(
	labelSet labels.Labels,
	serializedData *cppbridge.DataStorageSerializedData,
	mint, maxt int64,
	chunkRef uint32,
) AggrSeries {
	return AggrSeries{
		mint:           mint,
		maxt:           maxt,
		labelSet:       labelSet,
		serializedData: serializedData,
		chunkRef:       chunkRef,
	}
}

// Iterator returns an iterator that iterates over the aggregated of the samples of [AggrSeries].
// [storage.Series] interface implementation.
func (s *AggrSeries) Iterator(it chunkenc.Iterator) chunkenc.Iterator {
	aggrChunkIterator, ok := it.(*AggrChunkIterator)
	if !ok {
		return NewAggrChunkIterator(
			s.serializedData,
			s.mint,
			s.maxt,
			s.chunkRef,
		)
	}

	aggrChunkIterator.reset(s.serializedData, s.mint, s.maxt, s.chunkRef)
	return aggrChunkIterator
}

// Labels returns the labels of the [AggrSeries].
// [storage.Series] interface implementation.
func (s *AggrSeries) Labels() labels.Labels {
	return s.labelSet
}

//
// AggrChunkIterator
//

// AggrChunkIterator iterates over the aggregations of a time series, that can only get the next value.
type AggrChunkIterator struct {
	serializedData *cppbridge.DataStorageSerializedData
	chunkIterator  cppbridge.DataStorageSerializedDataAggregationIterator
	mint           int64
	maxt           int64
	isInitialized  bool
}

// NewAggrChunkIterator init new [AggrChunkIterator].
func NewAggrChunkIterator(
	serializedData *cppbridge.DataStorageSerializedData,
	mint, maxt int64,
	chunkRef uint32,
) *AggrChunkIterator {
	return &AggrChunkIterator{
		serializedData: serializedData,
		chunkIterator:  cppbridge.NewDataStorageSerializedDataAggregationIterator(serializedData, chunkRef),
		mint:           mint,
		maxt:           maxt,
	}
}

// At returns the current timestamp/value pair if the value is a float.
// [chunkenc.Iterator] interface implementation.
//
//nolint:gocritic // unnamedResult not need
func (it *AggrChunkIterator) At() (int64, float64) {
	return it.chunkIterator.Timestamp(), it.chunkIterator.Value()
}

// AtFloatHistogram returns the current timestamp/value pair if the value is a histogram with floating-point counts.
// [chunkenc.Iterator] interface implementation.
func (*AggrChunkIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	return 0, nil
}

// AtHistogram returns the current timestamp/value pair if the value is a histogram with integer counts.
// [chunkenc.Iterator] interface implementation.
func (*AggrChunkIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	return 0, nil
}

// AtT returns the current timestamp.
// [chunkenc.Iterator] interface implementation.
func (it *AggrChunkIterator) AtT() int64 {
	return it.chunkIterator.Timestamp()
}

// Err returns the current error - always nil.
// [chunkenc.Iterator] interface implementation.
func (*AggrChunkIterator) Err() error {
	return nil
}

// Next advances the iterator by one and returns the type of the value.
// [chunkenc.Iterator] interface implementation.
func (it *AggrChunkIterator) Next() chunkenc.ValueType {
	if it.nextValue() == chunkenc.ValNone {
		return chunkenc.ValNone
	}

	if it.AtT() > it.maxt {
		return chunkenc.ValNone
	}

	return chunkenc.ValFloat
}

// Seek advances the iterator forward to the first sample with a timestamp equal or greater than t.
// [chunkenc.Iterator] interface implementation.
func (it *AggrChunkIterator) Seek(t int64) chunkenc.ValueType {
	it.isInitialized = true
	if t > it.AtT() {
		return it.Next()
	}

	if it.AtT() > it.maxt {
		return chunkenc.ValNone
	}

	return chunkenc.ValFloat
}

// nextValue advances the iterator by one and returns the type of the value.
func (it *AggrChunkIterator) nextValue() chunkenc.ValueType {
	if !it.isInitialized {
		if !it.chunkIterator.HasData() {
			return chunkenc.ValNone
		}

		it.isInitialized = true
		return chunkenc.ValFloat
	}

	it.chunkIterator.Next()
	if !it.chunkIterator.HasData() {
		return chunkenc.ValNone
	}

	return chunkenc.ValFloat
}

// reset resets the iterator to the beginning of the serialized data.
func (it *AggrChunkIterator) reset(
	serializedData *cppbridge.DataStorageSerializedData,
	mint, maxt int64,
	chunkRef uint32,
) {
	it.serializedData = serializedData
	it.mint = mint
	it.maxt = maxt
	it.isInitialized = false
	it.chunkIterator.Reset(serializedData, chunkRef)
}
