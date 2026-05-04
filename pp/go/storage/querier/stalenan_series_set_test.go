package querier_test

import (
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/stretchr/testify/suite"
)

type StaleNaNSeriesSetTestSuite struct {
	suite.Suite

	lss                         *shard.LSS
	ds                          *shard.DataStorage
	data                        []storagetest.TimeSeries
	valueNotFoundTimestampValue int64
}

func TestStaleNaNSeriesSetTestSuite(t *testing.T) {
	suite.Run(t, new(StaleNaNSeriesSetTestSuite))
}

func (s *StaleNaNSeriesSetTestSuite) SetupTest() {
	s.lss = shard.NewLSS()
	s.ds = shard.NewDataStorage()
	s.valueNotFoundTimestampValue = 0

	s.data = []storagetest.TimeSeries{
		{
			Labels:  labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{{Timestamp: 10, Value: 0}},
		},
		{
			Labels:  labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{{Timestamp: 11, Value: 1}},
		},
		{
			Labels:  labels.FromStrings("__name__", "metric", "job", "test"),
			Samples: []cppbridge.Sample{{Timestamp: 12, Value: 2}},
		},
		{
			Labels:  labels.FromStrings("__name__", "metric", "job", "test_2"),
			Samples: []cppbridge.Sample{{Timestamp: 10, Value: 0}},
		},
		{
			Labels:  labels.FromStrings("__name__", "metric", "job", "test_2"),
			Samples: []cppbridge.Sample{{Timestamp: 11, Value: 1}},
		},
		{
			Labels:  labels.FromStrings("__name__", "metric", "job", "test_2"),
			Samples: []cppbridge.Sample{{Timestamp: 12, Value: 2}},
		},
	}

	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(s.lss, s.ds, s.data...)
}

func (s *StaleNaNSeriesSetTestSuite) TestSuccess() {
	// Arrange
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	expected := []storagetest.TimeSeries{s.data[0], s.data[3]}

	// Act
	seriesSet, err := storagetest.StaleNaNQuery(s.lss, s.ds, s.valueNotFoundTimestampValue, matcher)
	s.Require().NoError(err)

	actual := storagetest.TimeSeriesFromSeriesSet(seriesSet, false)

	// Assert
	s.Require().Equal(len(expected), len(actual))
	for i := range expected {
		s.Require().Equal(expected[i].Labels, actual[i].Labels)

		s.Require().Equal(len(expected[i].Samples), len(actual[i].Samples))
		for j := range expected[i].Samples {
			s.Require().Equal(expected[i].Samples[j].Timestamp, actual[i].Samples[j].Timestamp)
			s.Require().True(value.IsStaleNaN(actual[i].Samples[j].Value))
		}
	}
}

//
// StaleNaNSeriesChunkIteratorSuite
//

type StaleNaNSeriesChunkIteratorSuite struct {
	suite.Suite
}

func TestStaleNaNSeriesChunkIteratorSuite(t *testing.T) {
	suite.Run(t, new(StaleNaNSeriesChunkIteratorSuite))
}

func (s *StaleNaNSeriesChunkIteratorSuite) TestIteratorAt() {
	expectTimestamp := int64(42)
	it := querier.NewStaleNaNSeriesChunkIterator(expectTimestamp)

	t, v := it.At()
	s.Require().Equal(expectTimestamp, t)
	s.Require().Equal(expectTimestamp, it.AtT())
	s.Require().True(value.IsStaleNaN(v))
}

func (s *StaleNaNSeriesChunkIteratorSuite) TestIteratorNext() {
	expectTimestamp := int64(42)
	it := querier.NewStaleNaNSeriesChunkIterator(expectTimestamp)

	vt := it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt)

	t, v := it.At()
	s.Require().Equal(expectTimestamp, t)
	s.Require().Equal(expectTimestamp, it.AtT())
	s.Require().True(value.IsStaleNaN(v))
}

func (s *StaleNaNSeriesChunkIteratorSuite) TestIteratorNext2() {
	expectTimestamp := int64(42)
	it := querier.NewStaleNaNSeriesChunkIterator(expectTimestamp)

	vt := it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt)

	vt = it.Next()
	s.Require().Equal(chunkenc.ValNone, vt)

	t, v := it.At()
	s.Require().Equal(expectTimestamp, t)
	s.Require().Equal(expectTimestamp, it.AtT())
	s.Require().True(value.IsStaleNaN(v))
}

func (s *StaleNaNSeriesChunkIteratorSuite) TestIteratorSeek() {
	expectTimestamp := int64(42)
	it := querier.NewStaleNaNSeriesChunkIterator(expectTimestamp)

	vt := it.Seek(expectTimestamp)
	s.Require().Equal(chunkenc.ValFloat, vt)

	t, v := it.At()
	s.Require().Equal(expectTimestamp, t)
	s.Require().Equal(expectTimestamp, it.AtT())
	s.Require().True(value.IsStaleNaN(v))
}

func (s *StaleNaNSeriesChunkIteratorSuite) TestIteratorSeekNext() {
	expectTimestamp := int64(42)
	it := querier.NewStaleNaNSeriesChunkIterator(expectTimestamp)

	vt := it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt)

	vt = it.Seek(expectTimestamp)
	s.Require().Equal(chunkenc.ValFloat, vt)

	t, v := it.At()
	s.Require().Equal(expectTimestamp, t)
	s.Require().Equal(expectTimestamp, it.AtT())
	s.Require().True(value.IsStaleNaN(v))
}

func (s *StaleNaNSeriesChunkIteratorSuite) TestIteratorSeekGreater() {
	expectTimestamp := int64(42)
	it := querier.NewStaleNaNSeriesChunkIterator(expectTimestamp)

	vt := it.Seek(expectTimestamp + 1)
	s.Require().Equal(chunkenc.ValNone, vt)

	t, v := it.At()
	s.Require().Equal(expectTimestamp, t)
	s.Require().Equal(expectTimestamp, it.AtT())
	s.Require().True(value.IsStaleNaN(v))
}

func (s *StaleNaNSeriesChunkIteratorSuite) TestIteratorSeekGreaterNext() {
	expectTimestamp := int64(42)
	it := querier.NewStaleNaNSeriesChunkIterator(expectTimestamp)

	vt := it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt)

	vt = it.Seek(expectTimestamp + 1)
	s.Require().Equal(chunkenc.ValNone, vt)

	t, v := it.At()
	s.Require().Equal(expectTimestamp, t)
	s.Require().Equal(expectTimestamp, it.AtT())
	s.Require().True(value.IsStaleNaN(v))
}

func (s *StaleNaNSeriesChunkIteratorSuite) TestIteratorSeekLess() {
	expectTimestamp := int64(42)
	it := querier.NewStaleNaNSeriesChunkIterator(expectTimestamp)

	vt := it.Seek(expectTimestamp - 1)
	s.Require().Equal(chunkenc.ValFloat, vt)

	t, v := it.At()
	s.Require().Equal(expectTimestamp, t)
	s.Require().Equal(expectTimestamp, it.AtT())
	s.Require().True(value.IsStaleNaN(v))
}

func (s *StaleNaNSeriesChunkIteratorSuite) TestIteratorSeekLessNext() {
	expectTimestamp := int64(42)
	it := querier.NewStaleNaNSeriesChunkIterator(expectTimestamp)

	vt := it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt)

	vt = it.Seek(expectTimestamp - 1)
	s.Require().Equal(chunkenc.ValFloat, vt)

	t, v := it.At()
	s.Require().Equal(expectTimestamp, t)
	s.Require().Equal(expectTimestamp, it.AtT())
	s.Require().True(value.IsStaleNaN(v))
}
