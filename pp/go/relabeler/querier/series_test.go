package querier

import (
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type InstantSeriesSetTestSuite struct {
	suite.Suite

	valueNotFoundTimestampValue int64
	lssQueryResult              *cppbridge.LSSQueryResult
	labelSetSnapshot            *cppbridge.LabelSetSnapshot

	samples []cppbridge.Sample
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

	s.lssQueryResult = lss.Query([]model.LabelMatcher{{Name: "job", Value: "test"}}, cppbridge.LSSQuerySourceOther)
	require.Equal(s.T(), cppbridge.LSSQueryStatusMatch, s.lssQueryResult.Status())
	require.Equal(s.T(), 4, len(s.lssQueryResult.IDs()))

	s.labelSetSnapshot = cppbridge.NewLSSWithSnapshotWithoutBitset(lss).Snapshot()

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
		require.Equal(s.T(), expected[index].Labels().String(), is.Labels().String())
		ci := is.Iterator(nil)
		require.Equal(s.T(), chunkenc.ValFloat, ci.Next())
		timestamp, value := ci.At()
		require.Equal(s.T(), expected[index].sample.Timestamp, timestamp)
		require.Equal(s.T(), expected[index].sample.Value, value)
		index++
	}
}
