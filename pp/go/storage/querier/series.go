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

// ChunkIterator iterates over the samples of a time series, that can only get the next value with limit.
type ChunkIterator struct {
	serializedData *cppbridge.DataStorageSerializedData
	chunkIterator  cppbridge.DataStorageSerializedDataIterator
	mint           int64
	maxt           int64
	isInitialized  bool
}

// NewChunkIterator init new [ChunkIterator].
func NewChunkIterator(serializedData *cppbridge.DataStorageSerializedData, chunkRef uint32, mint, maxt int64) *ChunkIterator {
	it := &ChunkIterator{
		serializedData: serializedData,
		chunkIterator:  cppbridge.NewDataStorageSerializedDataIterator(serializedData, chunkRef),
		mint:           mint,
		maxt:           maxt,
	}

	if it.chunkIterator.Timestamp() < mint {
		it.chunkIterator.Seek(mint)
	}

	return it
}

func (it *ChunkIterator) Reset(serializedData *cppbridge.DataStorageSerializedData, chunkRef uint32, mint, maxt int64) {
	it.serializedData = serializedData
	it.mint = mint
	it.maxt = maxt
	it.isInitialized = false
	it.chunkIterator.Reset(serializedData, chunkRef)

	if it.chunkIterator.Timestamp() < mint {
		it.chunkIterator.Seek(mint)
	}
}

// At returns the current timestamp/value pair if the value is a float.
//
//nolint:gocritic // unnamedResult not need
func (it *ChunkIterator) At() (int64, float64) {
	return it.chunkIterator.Timestamp(), it.chunkIterator.Value()
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
	return it.chunkIterator.Timestamp()
}

// Err returns the current error.
func (it *ChunkIterator) Err() error {
	return nil
}

func (it *ChunkIterator) next() chunkenc.ValueType {
	if !it.isInitialized {
		if !it.chunkIterator.HasData() {
			return chunkenc.ValNone
		}

		it.isInitialized = true
		return chunkenc.ValFloat
	} else {
		it.chunkIterator.Next()

		if !it.chunkIterator.HasData() {
			return chunkenc.ValNone
		}

		return chunkenc.ValFloat
	}
}

// Next advances the iterator by one and returns the type of the value.
func (it *ChunkIterator) Next() chunkenc.ValueType {
	if it.next() == chunkenc.ValNone {
		return chunkenc.ValNone
	}

	if it.AtT() > it.maxt {
		return chunkenc.ValNone
	}

	return chunkenc.ValFloat
}

// Seek advances the iterator forward to the first sample with a timestamp equal or greater than t.
func (it *ChunkIterator) Seek(t int64) chunkenc.ValueType {
	// adjust lower limit.
	if t < it.mint {
		t = it.mint
	}

	ts := it.AtT()
	if !it.isInitialized || ts < t {
		it.chunkIterator.Seek(t)
		it.isInitialized = true
		if !it.chunkIterator.HasData() {
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
	builder                     labels.ScratchBuilder
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

	s.builder.Reset()
	s.series = append(s.series, NewSeries(
		s.mint,
		s.maxt,
		labels.NewLabelsWithLSS(s.labelSetSnapshot, seriesID, &s.builder),
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
