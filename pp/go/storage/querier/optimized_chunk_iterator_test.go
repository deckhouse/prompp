package querier_test

import (
	"math"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// OptimizedChunkIteratorSuite checks the iterator contract against real serialized C++ data.
type OptimizedChunkIteratorSuite struct {
	suite.Suite

	function  string
	rangeMS   int64
	expected  []cppbridge.Sample
	newSeries func(*cppbridge.DataStorageSerializedData, uint32, int64, int64) storage.Series

	ds             *cppbridge.DataStorage
	serializedData *cppbridge.DataStorageSerializedData
	chunkRef       uint32
	series         storage.Series
}

func TestOptimizedChunkIteratorSuite(t *testing.T) {
	t.Run("aggregation", func(t *testing.T) {
		suite.Run(t, &OptimizedChunkIteratorSuite{
			function: "max_over_time",
			rangeMS:  100,
			expected: []cppbridge.Sample{
				{Timestamp: 50, Value: 10},
				{Timestamp: 150, Value: 20},
				{Timestamp: 250, Value: 30},
				{Timestamp: 350, Value: 40},
			},
			newSeries: func(data *cppbridge.DataStorageSerializedData, ref uint32, mint, maxt int64) storage.Series {
				series := querier.NewAggrSeries(labels.EmptyLabels(), data, mint, maxt, ref)

				return &series
			},
		})
	})
	t.Run("cross", func(t *testing.T) {
		suite.Run(t, &OptimizedChunkIteratorSuite{
			function: "sum",
			expected: []cppbridge.Sample{
				{Timestamp: 100, Value: 10},
				{Timestamp: 200, Value: 20},
				{Timestamp: 300, Value: 30},
				{Timestamp: 400, Value: 40},
			},
			newSeries: func(data *cppbridge.DataStorageSerializedData, _ uint32, mint, maxt int64) storage.Series {
				series := querier.NewCrossSeries(
					labels.EmptyLabels(), data, &cppbridge.SeriesGroups{Groups: [][]uint32{{0}}}, 0, mint, maxt,
				)

				return &series
			},
		})
	})
}

func (s *OptimizedChunkIteratorSuite) SetupTest() {
	s.ds = cppbridge.NewDataStorage(false, false)
	s.ds.Encode(0, 50, 10)
	s.ds.Encode(0, 150, 20)
	s.ds.Encode(0, 250, 30)
	s.ds.Encode(0, 350, 40)
	s.serializedData, s.chunkRef = s.querySerializedData(1, 1)
	s.series = s.newSeries(s.serializedData, s.chunkRef, 1, 400)
}

func (s *OptimizedChunkIteratorSuite) TestIteratorReuseAfterExhaustion() {
	it := s.series.Iterator(nil)
	s.Require().Equal(chunkenc.ValNone, it.Seek(s.expected[3].Timestamp+1))
	mint := s.expected[1].Timestamp + 1
	data, ref := s.querySerializedData(mint, mint)
	series := s.newSeries(data, ref, mint, s.expected[3].Timestamp)

	reused := series.Iterator(it)

	s.Require().Same(it, reused)
	s.Require().Equal(chunkenc.ValFloat, reused.Seek(math.MinInt64))
	s.requireSample(reused, s.expected[2])
	s.Require().Equal(chunkenc.ValFloat, reused.Next())
	s.requireSample(reused, s.expected[3])
	s.Equal(chunkenc.ValNone, reused.Next())
}

func (s *OptimizedChunkIteratorSuite) TestNextHonorsLowerBound() {
	mint := s.expected[1].Timestamp + 1
	data, ref := s.querySerializedData(mint, mint)
	series := s.newSeries(data, ref, mint, s.expected[3].Timestamp)
	it := series.Iterator(nil)

	s.Require().Equal(chunkenc.ValFloat, it.Next())

	s.requireSample(it, s.expected[2])
}

func (s *OptimizedChunkIteratorSuite) TestSeekAfterNextExhaustsIterator() {
	it := s.series.Iterator(nil)
	for it.Next() != chunkenc.ValNone {
	}

	s.Equal(chunkenc.ValNone, it.Seek(math.MinInt64))
	s.Equal(chunkenc.ValNone, it.Next())
}

func (s *OptimizedChunkIteratorSuite) TestSeekAtUpperBound() {
	series := s.newSeries(s.serializedData, s.chunkRef, 1, s.expected[2].Timestamp)
	it := series.Iterator(nil)

	s.Require().Equal(chunkenc.ValFloat, it.Seek(s.expected[2].Timestamp))

	s.requireSample(it, s.expected[2])
	s.Equal(chunkenc.ValNone, it.Next())
	s.Equal(chunkenc.ValNone, it.Seek(math.MinInt64))
	s.Equal(chunkenc.ValNone, it.Next())
}

func (s *OptimizedChunkIteratorSuite) TestSeekBeforeFirstSample() {
	it := s.series.Iterator(nil)

	s.Require().Equal(chunkenc.ValFloat, it.Seek(math.MinInt64))

	s.requireSample(it, s.expected[0])
	s.Require().Equal(chunkenc.ValFloat, it.Next())
	s.requireSample(it, s.expected[1])
}

func (s *OptimizedChunkIteratorSuite) TestSeekBetweenSamples() {
	it := s.series.Iterator(nil)

	s.Require().Equal(chunkenc.ValFloat, it.Seek(s.expected[1].Timestamp+1))

	s.requireSample(it, s.expected[2])
	s.Require().Equal(chunkenc.ValFloat, it.Next())
	s.requireSample(it, s.expected[3])
}

func (s *OptimizedChunkIteratorSuite) TestSeekDoesNotMoveBackwards() {
	it := s.series.Iterator(nil)
	s.Require().Equal(chunkenc.ValFloat, it.Seek(s.expected[1].Timestamp))
	s.requireSample(it, s.expected[1])

	s.Require().Equal(chunkenc.ValFloat, it.Seek(s.expected[0].Timestamp))
	s.requireSample(it, s.expected[1])
	s.Require().Equal(chunkenc.ValFloat, it.Seek(s.expected[1].Timestamp))
	s.requireSample(it, s.expected[1])
	s.Require().Equal(chunkenc.ValFloat, it.Next())
	s.requireSample(it, s.expected[2])
}

func (s *OptimizedChunkIteratorSuite) TestSeekExtremeTimestamp() {
	it := s.series.Iterator(nil)

	s.Require().Equal(chunkenc.ValNone, it.Seek(math.MaxInt64))

	s.Equal(chunkenc.ValNone, it.Seek(math.MinInt64))
	s.Equal(chunkenc.ValNone, it.Next())
}

func (s *OptimizedChunkIteratorSuite) TestSeekHonorsLowerBound() {
	mint := s.expected[1].Timestamp + 1
	data, ref := s.querySerializedData(mint, mint)
	series := s.newSeries(data, ref, mint, s.expected[3].Timestamp)
	it := series.Iterator(nil)

	s.Require().Equal(chunkenc.ValFloat, it.Seek(math.MinInt64))

	s.requireSample(it, s.expected[2])
}

func (s *OptimizedChunkIteratorSuite) TestSeekOnEmptyIterator() {
	data, ref := s.querySerializedData(1, 1_000)
	it := s.newSeries(data, ref, 1, 1_400).Iterator(nil)
	s.Require().Equal(chunkenc.ValNone, it.Next())

	s.Equal(chunkenc.ValNone, it.Seek(math.MinInt64))
	s.Equal(chunkenc.ValNone, it.Seek(1_000))
	s.Equal(chunkenc.ValNone, it.Next())
}

func (s *OptimizedChunkIteratorSuite) TestSeekOnEmptyIteratorWithoutNext() {
	data, ref := s.querySerializedData(1, 1_000)
	it := s.newSeries(data, ref, 1, 1_400).Iterator(nil)

	s.Equal(chunkenc.ValNone, it.Seek(math.MinInt64))
	s.Equal(chunkenc.ValNone, it.Next())
}

func (s *OptimizedChunkIteratorSuite) TestSeekPastLastSample() {
	it := s.series.Iterator(nil)

	s.Require().Equal(chunkenc.ValNone, it.Seek(s.expected[3].Timestamp+1))

	s.Equal(chunkenc.ValNone, it.Seek(math.MinInt64))
	s.Equal(chunkenc.ValNone, it.Next())
}

func (s *OptimizedChunkIteratorSuite) TestSeekPastUpperBound() {
	series := s.newSeries(s.serializedData, s.chunkRef, 1, s.expected[1].Timestamp)
	it := series.Iterator(nil)

	s.Require().Equal(chunkenc.ValNone, it.Seek(s.expected[2].Timestamp))

	s.Equal(chunkenc.ValNone, it.Seek(math.MinInt64))
	s.Equal(chunkenc.ValNone, it.Next())
}

func (s *OptimizedChunkIteratorSuite) TestSeekSkipsMultipleSamples() {
	it := s.series.Iterator(nil)
	s.Require().Equal(chunkenc.ValFloat, it.Next())
	s.requireSample(it, s.expected[0])

	s.Require().Equal(chunkenc.ValFloat, it.Seek(s.expected[3].Timestamp))

	s.requireSample(it, s.expected[3])
	s.Equal(chunkenc.ValNone, it.Next())
}

func (s *OptimizedChunkIteratorSuite) TestSeekWithoutNext() {
	it := s.series.Iterator(nil)

	s.Require().Equal(chunkenc.ValFloat, it.Seek(s.expected[3].Timestamp))

	s.requireSample(it, s.expected[3])
	s.Equal(chunkenc.ValNone, it.Next())
}

func (s *OptimizedChunkIteratorSuite) querySerializedData(start, hintsStart int64) (*cppbridge.DataStorageSerializedData, uint32) {
	hints := &storage.SelectHints{
		Start: hintsStart, End: hintsStart + 399, Step: 100, Range: s.rangeMS, LookbackDelta: 100, Func: s.function,
	}
	result := s.ds.Query(cppbridge.DataStorageQuery{
		StartTimestampMs: start,
		EndTimestampMs:   start + 399,
		LabelSetIDs:      []uint32{0},
	}, cppbridge.NoDownsampling, unsafe.Pointer(hints))
	s.Require().Equal(cppbridge.DataStorageQueryStatusSuccess, result.Status)
	seriesID, ref := result.SerializedData.Next()
	s.Require().NotEqual(uint32(math.MaxUint32), seriesID)

	return result.SerializedData, ref
}

func (s *OptimizedChunkIteratorSuite) requireSample(it chunkenc.Iterator, expected cppbridge.Sample) {
	timestamp, value := it.At()
	s.Equal(expected, cppbridge.Sample{Timestamp: timestamp, Value: value})
	s.Require().NoError(it.Err())
}
