package querier

import (
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
)

type DefaultSampleProvider struct {
	deserializer   *cppbridge.HeadDataStorageDeserializer
	chunksMetadata []cppbridge.HeadDataStorageSerializedChunkMetadata
}

func (sp *DefaultSampleProvider) Samples(_ uint32, mint, maxt int64) chunkenc.Iterator {
	return NewLimitedChunkIterator(
		NewChunkIterator(sp.deserializer, sp.chunksMetadata),
		mint,
		maxt,
	)
}

type ChunkIterator struct {
	deserializer   *cppbridge.HeadDataStorageDeserializer
	chunksMetadata []cppbridge.HeadDataStorageSerializedChunkMetadata
	decodeIterator *cppbridge.HeadDataStorageDecodeIterator
	ts             int64
	v              float64
}

func NewChunkIterator(deserializer *cppbridge.HeadDataStorageDeserializer, chunksMetadata []cppbridge.HeadDataStorageSerializedChunkMetadata) *ChunkIterator {
	return &ChunkIterator{
		deserializer:   deserializer,
		chunksMetadata: chunksMetadata,
	}
}

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

func (i *ChunkIterator) At() (int64, float64) {
	return i.ts, i.v
}

func (i *ChunkIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	return 0, nil
}

func (i *ChunkIterator) AtFloatHistogram(floatHistogram *histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	return 0, nil
}

func (i *ChunkIterator) AtT() int64 {
	return i.ts
}

func (i *ChunkIterator) Err() error {
	return nil
}

type LimitedChunkIterator struct {
	chunkIterator chunkenc.Iterator
	mint          int64
	maxt          int64
}

func NewLimitedChunkIterator(iterator chunkenc.Iterator, mint, maxt int64) *LimitedChunkIterator {
	return &LimitedChunkIterator{
		chunkIterator: iterator,
		mint:          mint,
		maxt:          maxt,
	}
}

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

func (i *LimitedChunkIterator) At() (int64, float64) {
	return i.chunkIterator.At()
}

func (i *LimitedChunkIterator) AtHistogram(h *histogram.Histogram) (int64, *histogram.Histogram) {
	return i.chunkIterator.AtHistogram(h)
}

func (i *LimitedChunkIterator) AtFloatHistogram(h *histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	return i.chunkIterator.AtFloatHistogram(h)
}

func (i *LimitedChunkIterator) AtT() int64 {
	return i.chunkIterator.AtT()
}

func (i *LimitedChunkIterator) Err() error {
	return i.chunkIterator.Err()
}

type SampleProvider interface {
	Samples(seriesID uint32, minT, maxtT int64) chunkenc.Iterator
}

type Series struct {
	seriesID       uint32
	mint, maxt     int64
	labelSet       *cppbridge.LabelsCpp
	sampleProvider SampleProvider
}

func (s *Series) Labels() labels.Labels {
	return s.labelSet.Labels()
}

func (s *Series) Iterator(_ chunkenc.Iterator) chunkenc.Iterator {
	return s.sampleProvider.Samples(s.seriesID, s.mint, s.maxt)
}

type SeriesSet struct {
	index         int
	seriesSet     []*Series
	currentSeries *Series
}

func NewSeriesSet(seriesSet []*Series) *SeriesSet {
	return &SeriesSet{
		seriesSet: seriesSet,
	}
}

func (ss *SeriesSet) Next() bool {
	if ss.index >= len(ss.seriesSet) {
		return false
	}

	ss.currentSeries = ss.seriesSet[ss.index]
	ss.index++
	return true
}

func (ss *SeriesSet) At() storage.Series {
	return ss.currentSeries
}

func (ss *SeriesSet) Err() error {
	return nil
}

func (ss *SeriesSet) Warnings() annotations.Annotations {
	return nil
}

const (
	DefaultInstantQueryValueNotFoundTimestampValue int64 = 0
)

type InstantSeriesSet struct {
	index                       int
	valueNotFoundTimestampValue int64
	labelSets                   []*cppbridge.LabelsCpp
	samples                     []cppbridge.Sample
}

func NewInstantSeriesSet(valueNotFoundTimestampValue int64, labelSets []*cppbridge.LabelsCpp, samples []cppbridge.Sample) *InstantSeriesSet {
	return &InstantSeriesSet{
		index:                       -1,
		valueNotFoundTimestampValue: valueNotFoundTimestampValue,
		labelSets:                   labelSets,
		samples:                     samples,
	}
}

func (ss *InstantSeriesSet) Next() bool {
	if ss.index >= len(ss.labelSets) {
		return false
	}

	ss.index++

	if ss.samples[ss.index].Timestamp == ss.valueNotFoundTimestampValue {
		return ss.Next()
	}

	return true
}

func (ss *InstantSeriesSet) At() storage.Series {
	return InstantSeries{
		labelSet: ss.labelSets[ss.index],
		sample:   ss.samples[ss.index],
	}
}

func (ss *InstantSeriesSet) Err() error {
	return nil
}

func (ss *InstantSeriesSet) Warnings() annotations.Annotations {
	return nil
}

type InstantSeries struct {
	labelSet *cppbridge.LabelsCpp
	sample   cppbridge.Sample
}

// Labels is storage.Series interface implementation.
func (s InstantSeries) Labels() labels.Labels {
	return s.labelSet.Labels()
}

// Iterator is storage.Series interface implementation.
func (s InstantSeries) Iterator(iterator chunkenc.Iterator) chunkenc.Iterator {
	if i, ok := iterator.(*InstantSeriesChunkIterator); ok {
		i.ResetTo(s.sample.Timestamp, s.sample.Value)
		return i
	}
	return NewInstantSeriesChunkIterator(s.sample.Timestamp, s.sample.Value)
}

type InstantSeriesChunkIterator struct {
	i int
	t int64
	v float64
}

func NewInstantSeriesChunkIterator(t int64, v float64) *InstantSeriesChunkIterator {
	return &InstantSeriesChunkIterator{
		t: t,
		v: v,
	}
}

func (i *InstantSeriesChunkIterator) ResetTo(t int64, v float64) {
	i.i = 0
	i.t = t
	i.v = v
}

// Next is chunkenc.Iterator interface implementation.
func (i *InstantSeriesChunkIterator) Next() chunkenc.ValueType {
	if i.i > 0 {
		return chunkenc.ValNone
	}

	i.i++
	return chunkenc.ValFloat
}

// Seek is chunkenc.Iterator interface implementation.
func (i *InstantSeriesChunkIterator) Seek(t int64) chunkenc.ValueType {
	if i.i > 0 {
		return chunkenc.ValNone
	}

	if i.t == t {
		return chunkenc.ValFloat
	}

	i.i++
	return chunkenc.ValNone
}

// At is chunkenc.Iterator interface implementation.
func (i *InstantSeriesChunkIterator) At() (int64, float64) {
	return i.t, i.v
}

// AtHistogram is chunkenc.Iterator interface implementation.
func (i *InstantSeriesChunkIterator) AtHistogram(h *histogram.Histogram) (int64, *histogram.Histogram) {
	return 0, nil
}

// AtFloatHistogram is chunkenc.Iterator interface implementation.
func (i *InstantSeriesChunkIterator) AtFloatHistogram(floatHistogram *histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	return 0, nil
}

// AtT is chunkenc.Iterator interface implementation.
func (i *InstantSeriesChunkIterator) AtT() int64 {
	return i.t
}

// Err is chunkenc.Iterator interface implementation.
func (i *InstantSeriesChunkIterator) Err() error {
	return nil
}
