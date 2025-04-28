package querier

import (
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"testing"
)

type InstantSeriesSetTestSuite struct {
	suite.Suite

	valueNotFoundTimestampValue int64
	labelSets                   []*cppbridge.LabelsCpp
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

	lqr := lss.Query([]model.LabelMatcher{{Name: "job", Value: "test"}}, cppbridge.LSSQuerySourceOther)
	require.Equal(s.T(), cppbridge.LSSQueryStatusMatch, lqr.Status())
	require.Equal(s.T(), 4, len(lqr.IDs()))

	labelSets := make([]*cppbridge.LabelsCpp, len(lqr.IDs()))
	lqr.MatchesIndexRange(func(lss *cppbridge.LabelSetStorage, index int, lsid uint32, length uint16) {
		labelSets[index] = cppbridge.NewLabelsCpp(lss, lsid, length)
	})

	for _, ls := range labelSets {
		s.T().Log(ls.Labels().String())
	}

	s.valueNotFoundTimestampValue = 0
	s.labelSets = labelSets
	s.samples = []cppbridge.Sample{
		{Timestamp: 1, Value: 1},
		{Timestamp: s.valueNotFoundTimestampValue, Value: 0},
		{Timestamp: 3, Value: 3},
		{Timestamp: s.valueNotFoundTimestampValue, Value: 0},
	}
}

func (s *InstantSeriesSetTestSuite) TestNext() {
	iss := NewInstantSeriesSet(s.valueNotFoundTimestampValue, s.labelSets, s.samples)

	expected := []InstantSeries{
		{labelSet: s.labelSets[0], sample: s.samples[0]},
		{labelSet: s.labelSets[2], sample: s.samples[2]},
	}

	index := 0
	for iss.Next() {
		is := iss.At()
		require.Equal(s.T(), expected[index].Labels().String(), is.Labels().String())
		ci := is.Iterator(nil)
		require.Equal(s.T(), chunkenc.ValFloat, ci.Next())
		timestamp, value := ci.At()
		require.Equal(s.T(), expected[index].sample.Timestamp, timestamp)
		require.Equal(s.T(), expected[index].sample.Value, value)
		index++
	}
}
