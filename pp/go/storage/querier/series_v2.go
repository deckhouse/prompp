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

type SeriesV2 struct {
	mint, maxt    int64
	labelSet      labels.Labels
	chunkIterator *cppbridge.DataStorageSerializedDataIterator
}

func (s *SeriesV2) Next() chunkenc.ValueType {
	hasValue := s.chunkIterator.Next()
	if !hasValue {
		return chunkenc.ValNone
	}
	return chunkenc.ValFloat
}

func (s *SeriesV2) Seek(t int64) chunkenc.ValueType {
	for s.AtT() < t {
		if s.Next() == chunkenc.ValNone {
			return chunkenc.ValNone
		}
	}

	return chunkenc.ValFloat
}

func (s *SeriesV2) At() (int64, float64) {
	return s.chunkIterator.At()
}

func (s *SeriesV2) AtHistogram(histogram *histogram.Histogram) (int64, *histogram.Histogram) {
	return 0, nil
}

func (s *SeriesV2) AtFloatHistogram(histogram *histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	return 0, nil
}

func (s *SeriesV2) AtT() int64 {
	ts, _ := s.chunkIterator.At()
	return ts
}

func (s *SeriesV2) Err() error {
	return nil
}

func NewSeriesV2(mint, maxt int64, labelSet labels.Labels, chunkIterator *cppbridge.DataStorageSerializedDataIterator) *SeriesV2 {
	return &SeriesV2{
		mint:          mint,
		maxt:          maxt,
		labelSet:      labelSet,
		chunkIterator: chunkIterator,
	}
}

func (s *SeriesV2) Labels() labels.Labels {
	return s.labelSet
}

func (s *SeriesV2) Iterator(iterator chunkenc.Iterator) chunkenc.Iterator {
	return s
}

type SeriesSetV2 struct {
	mint, maxt       int64
	lssQueryResult   *cppbridge.LSSQueryResult
	labelSetSnapshot *cppbridge.LabelSetSnapshot
	serializedData   *cppbridge.DataStorageSerializedData

	series *SeriesV2
}

func NewSeriesSetV2(
	mint, maxt int64,
	lssQueryResult *cppbridge.LSSQueryResult,
	labelSetSnapshot *cppbridge.LabelSetSnapshot,
	serializedData *cppbridge.DataStorageSerializedData,
) *SeriesSetV2 {
	return &SeriesSetV2{
		mint:             mint,
		maxt:             maxt,
		lssQueryResult:   lssQueryResult,
		labelSetSnapshot: labelSetSnapshot,
		serializedData:   serializedData,
	}
}

func (s *SeriesSetV2) Next() bool {
	seriesID := s.serializedData.Next()
	if seriesID == math.MaxUint32 {
		return false
	}

	_, lsLength := s.lssQueryResult.GetByIndex(s.lssQueryResult.IndexOf(seriesID))
	s.series = NewSeriesV2(
		s.mint,
		s.maxt,
		labels.NewLabelsWithLSS(s.labelSetSnapshot, seriesID, lsLength),
		s.serializedData.Iterator(),
	)

	return true
}

func (s *SeriesSetV2) At() storage.Series {
	return s.series
}

func (s *SeriesSetV2) Err() error {
	return nil
}

func (s *SeriesSetV2) Warnings() annotations.Annotations {
	return nil
}
