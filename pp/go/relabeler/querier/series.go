package querier

import (
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
)

//
// SeriesSet
//

type SeriesSet struct {
	mint             int64
	maxt             int64
	deserializer     *cppbridge.HeadDataStorageDeserializer
	chunksIndex      cppbridge.HeadDataStorageSerializedChunkIndex
	serializedChunks cppbridge.HeadDataStorageSerializedChunks
	lssQueryResult   *cppbridge.LSSQueryResult
	labelSetSnapshot *cppbridge.LabelSetSnapshot

	index         int
	currentSeries *Series
}

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
		seriesID: lsID,
		mint:     ss.mint,
		maxt:     ss.maxt,
		labelSet: labels.NewLabelsWithLSS(
			ss.labelSetSnapshot,
			lsID,
			lsLength,
		),
		sampleProvider: &DefaultSampleProvider{
			deserializer:   ss.deserializer,
			chunksMetadata: chunksMetadata,
		},
	}

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

//
// Series
//

type Series struct {
	mint, maxt     int64
	labelSet       labels.Labels
	sampleProvider SampleProvider
	seriesID       uint32
}

func (s *Series) Labels() labels.Labels {
	return s.labelSet
}

func (s *Series) Iterator(_ chunkenc.Iterator) chunkenc.Iterator {
	return s.sampleProvider.Samples(s.seriesID, s.mint, s.maxt)
}

//
// DefaultSampleProvider
//

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

//
// Instant
//

const (
	DefaultInstantQueryValueNotFoundTimestampValue int64 = 0
)

//
// InstantSeriesSet
//

type InstantSeriesSet struct {
	lssQueryResult              *cppbridge.LSSQueryResult
	labelSetSnapshot            *cppbridge.LabelSetSnapshot
	valueNotFoundTimestampValue int64
	samples                     []cppbridge.Sample

	index         int
	currentSeries *InstantSeries
}

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

func (ss *InstantSeriesSet) At() storage.Series {
	return ss.currentSeries
}

func (ss *InstantSeriesSet) Err() error {
	return nil
}

func (ss *InstantSeriesSet) Warnings() annotations.Annotations {
	return nil
}

//
// InstantSeries
//

type InstantSeries struct {
	labelSet labels.Labels
	sample   cppbridge.Sample
}

// Labels is storage.Series interface implementation.
func (s *InstantSeries) Labels() labels.Labels {
	return s.labelSet
}

// Iterator is storage.Series interface implementation.
func (s *InstantSeries) Iterator(iterator chunkenc.Iterator) chunkenc.Iterator {
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
		i: -1,
		t: t,
		v: v,
	}
}

func (i *InstantSeriesChunkIterator) ResetTo(t int64, v float64) {
	i.i = -1
	i.t = t
	i.v = v
}

// Next is chunkenc.Iterator interface implementation.
func (i *InstantSeriesChunkIterator) Next() chunkenc.ValueType {
	if i.i < 1 {
		i.i++
	}
	return i.valueType()
}

// Seek is chunkenc.Iterator interface implementation.
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
