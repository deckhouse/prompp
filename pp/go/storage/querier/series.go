package querier

import (
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
)

// SampleProvider create chunk iterator over samples.
type SampleProvider interface {
	Samples(minT, maxtT int64) chunkenc.Iterator
}

//
// SeriesSet
//

// SeriesSet contains a set of series, allows to iterate over sorted, populated series.
type SeriesSet struct {
	mint             int64
	maxt             int64
	deserializer     *cppbridge.HeadDataStorageDeserializer
	chunksIndex      cppbridge.HeadDataStorageSerializedChunkIndex
	serializedChunks *cppbridge.HeadDataStorageSerializedChunks
	lssQueryResult   *cppbridge.LSSQueryResult
	labelSetSnapshot *cppbridge.LabelSetSnapshot

	index         int
	currentSeries *Series
}

// At returns full series. Returned series should be iterable even after Next is called.
func (ss *SeriesSet) At() storage.Series {
	return ss.currentSeries
}

// Err the error that iteration as failed with.
func (*SeriesSet) Err() error {
	return nil
}

// Next return true if exist there is a next series and false otherwise.
func (ss *SeriesSet) Next() bool {
	if ss.lssQueryResult == nil {
		return false
	}

	var (
		lsID           uint32
		lsLength       uint16
		chunksMetadata []cppbridge.HeadDataStorageSerializedChunkMetadata
	)

	for {
		if ss.index >= ss.lssQueryResult.Len() {
			return false
		}

		lsID, lsLength = ss.lssQueryResult.GetByIndex(ss.index)

		chunksMetadata = ss.chunksIndex.Chunks(ss.serializedChunks, lsID)
		ss.index++
		if len(chunksMetadata) != 0 {
			break
		}
	}

	ss.currentSeries = &Series{
		mint:     ss.mint,
		maxt:     ss.maxt,
		labelSet: labels.NewLabelsWithLSS(ss.labelSetSnapshot, lsID, lsLength),
		sampleProvider: &DefaultSampleProvider{
			deserializer:   ss.deserializer,
			chunksMetadata: chunksMetadata,
		},
	}

	return true
}

// Warnings a collection of warnings for the whole set.
func (*SeriesSet) Warnings() annotations.Annotations {
	return nil
}

//
// Series
//

// Series is a stream of data points belonging to a metric.
type Series struct {
	mint, maxt     int64
	labelSet       labels.Labels
	sampleProvider SampleProvider
}

// Iterator returns an iterator of the data of the series.
func (s *Series) Iterator(_ chunkenc.Iterator) chunkenc.Iterator {
	return s.sampleProvider.Samples(s.mint, s.maxt)
}

// Labels returns the complete set of labels.
func (s *Series) Labels() labels.Labels {
	return s.labelSet
}

//
// DefaultSampleProvider
//

// DefaultSampleProvider create default chunk iterator over samples.
type DefaultSampleProvider struct {
	deserializer   *cppbridge.HeadDataStorageDeserializer
	chunksMetadata []cppbridge.HeadDataStorageSerializedChunkMetadata
}

// Samples reurns chunk iterator over samples.
func (sp *DefaultSampleProvider) Samples(mint, maxt int64) chunkenc.Iterator {
	return NewLimitedChunkIterator(
		NewChunkIterator(sp.deserializer, sp.chunksMetadata),
		mint,
		maxt,
	)
}

//
// ChunkIterator
//

// ChunkIterator iterates over the samples of a time series, that can only get the next value.
type ChunkIterator struct {
	deserializer   *cppbridge.HeadDataStorageDeserializer
	chunksMetadata []cppbridge.HeadDataStorageSerializedChunkMetadata
	decodeIterator *cppbridge.HeadDataStorageDecodeIterator
	ts             int64
	v              float64
}

// NewChunkIterator init new [ChunkIterator].
func NewChunkIterator(
	deserializer *cppbridge.HeadDataStorageDeserializer,
	chunksMetadata []cppbridge.HeadDataStorageSerializedChunkMetadata,
) *ChunkIterator {
	return &ChunkIterator{
		deserializer:   deserializer,
		chunksMetadata: chunksMetadata,
	}
}

// At returns the current timestamp/value pair if the value is a float.
//
//nolint:gocritic // unnamedResult not need
func (i *ChunkIterator) At() (int64, float64) {
	return i.ts, i.v
}

// AtFloatHistogram returns the current timestamp/value pair if the value is a histogram with floating-point counts.
func (*ChunkIterator) AtFloatHistogram(_ *histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	return 0, nil
}

// AtHistogram returns the current timestamp/value pair if the value is a histogram with integer counts.
func (*ChunkIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	return 0, nil
}

// AtT returns the current timestamp.
func (i *ChunkIterator) AtT() int64 {
	return i.ts
}

// Err returns the current error.
func (*ChunkIterator) Err() error {
	return nil
}

// Next advances the iterator by one and returns the type of the value.
func (i *ChunkIterator) Next() chunkenc.ValueType {
	if i.decodeIterator == nil {
		if len(i.chunksMetadata) == 0 {
			return chunkenc.ValNone
		}

		i.decodeIterator = i.deserializer.CreateDecodeIterator(i.chunksMetadata[0])
		i.chunksMetadata = i.chunksMetadata[1:]
	}

	if !i.decodeIterator.Next() {
		i.decodeIterator = nil
		return i.Next()
	}

	i.ts, i.v = i.decodeIterator.Sample()
	return chunkenc.ValFloat
}

// Seek advances the iterator forward to the first sample with a timestamp equal or greater than t.
func (i *ChunkIterator) Seek(t int64) chunkenc.ValueType {
	if i.decodeIterator == nil {
		if i.Next() == chunkenc.ValNone {
			return chunkenc.ValNone
		}
	}

	for i.ts < t {
		if i.Next() == chunkenc.ValNone {
			return chunkenc.ValNone
		}
	}

	return chunkenc.ValFloat
}

//
// LimitedChunkIterator
//

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
