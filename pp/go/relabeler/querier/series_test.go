package querier

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

type InstantSeriesSetTestSuite struct {
	suite.Suite

	valueNotFoundTimestampValue int64
	lssQueryResult              *cppbridge.LSSQueryResult
	labelSetSnapshot            *cppbridge.LabelSetSnapshot
	samples                     []cppbridge.Sample
}

func TestInstantSeriesSetTestSuite(t *testing.T) {
	suite.Run(t, new(InstantSeriesSetTestSuite))
}

func (s *InstantSeriesSetTestSuite) SetupTest() {
	lss := cppbridge.NewQueryableLssStorage()
	lss.FindOrEmplace(model.LabelSetFromPairs("job", "test", "__name__", "testmetric0"))
	lss.FindOrEmplace(model.LabelSetFromPairs("job", "test", "__name__", "testmetric1"))
	lss.FindOrEmplace(model.LabelSetFromPairs("job", "test", "__name__", "testmetric2"))
	lss.FindOrEmplace(model.LabelSetFromPairs("job", "test", "__name__", "testmetric3"))

	s.labelSetSnapshot = lss.CreateLabelSetSnapshot()

	selector, status := lss.QuerySelector([]model.LabelMatcher{{Name: "job", Value: "test"}})
	s.Require().Equal(cppbridge.LSSQueryStatusMatch, status)

	s.lssQueryResult = s.labelSetSnapshot.Query(selector)
	s.Require().Equal(4, len(s.lssQueryResult.IDs()))

	s.valueNotFoundTimestampValue = 0
	s.samples = []cppbridge.Sample{
		{Timestamp: 1, Value: 1},
		{Timestamp: s.valueNotFoundTimestampValue, Value: 0},
		{Timestamp: 3, Value: 3},
		{Timestamp: s.valueNotFoundTimestampValue, Value: 0},
	}
}

func (s *InstantSeriesSetTestSuite) TestNext() {
	iss := NewInstantSeriesSet(s.lssQueryResult, s.labelSetSnapshot, s.valueNotFoundTimestampValue, s.samples)

	expected := make([]InstantSeries, 0, 2)
	for _, idx := range []int{0, 2} {
		lsID, lsLength := s.lssQueryResult.GetByIndex(idx)

		expected = append(expected, InstantSeries{
			labelSet: labels.NewLabelsWithLSS(s.labelSetSnapshot, lsID, lsLength),
			sample:   s.samples[idx],
		})
	}

	index := 0
	for iss.Next() {
		is := iss.At()
		s.Require().Equal(expected[index].Labels().String(), is.Labels().String())
		ci := is.Iterator(nil)
		s.Require().Equal(chunkenc.ValFloat, ci.Next())
		timestamp, value := ci.At()
		s.Require().Equal(expected[index].sample.Timestamp, timestamp)
		s.Require().Equal(expected[index].sample.Value, value)
		index++
	}
}
