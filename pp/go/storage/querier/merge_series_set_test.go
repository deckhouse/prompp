package querier_test

import (
	"fmt"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
	"github.com/prometheus/prometheus/storage"
	"github.com/stretchr/testify/suite"
)

type MergeShardSeriesSetSuite struct {
	suite.Suite
}

func TestMergeShardSeriesSetSuite(t *testing.T) {
	suite.Run(t, new(MergeShardSeriesSetSuite))
}

func (s *MergeShardSeriesSetSuite) TestHappyPath() {
	var start int64
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	for _, bm := range []struct {
		numShards, numSeries, numSamples int
	}{
		{1, 100, 5},
		{2, 100, 5},
		{4, 100, 5},
		{6, 100, 5},
		{8, 100, 5},
		{10, 100, 5},
	} {
		end := int64(bm.numSamples)
		head := makeHead(bm.numShards, bm.numSeries, bm.numSamples)
		eseriesSets := make([]storage.SeriesSet, 0, bm.numShards)
		aseriesSets := make([]storage.SeriesSet, 0, bm.numShards)

		s.Run(fmt.Sprintf("%d_%d_%d", bm.numShards, bm.numSeries, bm.numSamples), func() {
			eseriesSets = eseriesSets[:0]
			aseriesSets = aseriesSets[:0]
			for i := 0; i < bm.numShards; i++ {
				eseriesSets = append(eseriesSets, queryOpt(s.T(), head.lsses[i], head.dss[i], start, end, cppbridge.NoDownsampling, matcher))
				aseriesSets = append(aseriesSets, queryOpt(s.T(), head.lsses[i], head.dss[i], start, end, cppbridge.NoDownsampling, matcher))
			}

			expectedSeriesSet := storage.NewMergeSeriesSet(eseriesSets, storage.ChainedSeriesMerge)
			actualSeriesSet := querier.NewMergeShardSeriesSet(aseriesSets)

			s.Require().Equal(
				storagetest.TimeSeriesFromSeriesSet(expectedSeriesSet, false),
				storagetest.TimeSeriesFromSeriesSet(actualSeriesSet, false),
			)
		})
	}
}

func (s *MergeShardSeriesSetSuite) TestEmptySeriesSets() {
	var start int64
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	for _, bm := range []struct {
		numShards, numSeries, numSamples int
	}{
		{1, 100, 5},
		{2, 100, 5},
		{4, 100, 5},
		{6, 100, 5},
		{8, 100, 5},
		{10, 100, 5},
	} {
		end := int64(bm.numSamples)
		head := makeHead(bm.numShards, bm.numSeries, bm.numSamples)
		eseriesSets := make([]storage.SeriesSet, 0, bm.numShards)
		aseriesSets := make([]storage.SeriesSet, 0, bm.numShards)

		s.Run(fmt.Sprintf("%d_%d_%d", bm.numShards, bm.numSeries, bm.numSamples), func() {
			eseriesSets = eseriesSets[:0]
			aseriesSets = aseriesSets[:0]
			for i := 0; i < bm.numShards; i++ {
				if i%2 == 0 {
					eseriesSets = append(eseriesSets, &querier.SeriesSet{})
					aseriesSets = append(aseriesSets, &querier.SeriesSet{})
				}

				eseriesSets = append(eseriesSets, queryOpt(s.T(), head.lsses[i], head.dss[i], start, end, cppbridge.NoDownsampling, matcher))
				aseriesSets = append(aseriesSets, queryOpt(s.T(), head.lsses[i], head.dss[i], start, end, cppbridge.NoDownsampling, matcher))
			}

			expectedSeriesSet := storage.NewMergeSeriesSet(eseriesSets, storage.ChainedSeriesMerge)
			actualSeriesSet := querier.NewMergeShardSeriesSet(aseriesSets)

			s.Require().Equal(
				storagetest.TimeSeriesFromSeriesSet(expectedSeriesSet, false),
				storagetest.TimeSeriesFromSeriesSet(actualSeriesSet, false),
			)
		})
	}
}

func (s *MergeShardSeriesSetSuite) TestAcrossShardsSeriesSets() {
	var start int64
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	for _, bm := range []struct {
		numShards, numSeries, numSamples int
	}{
		{1, 100, 5},
		{2, 100, 5},
		{4, 100, 5},
		{6, 100, 5},
		{8, 100, 5},
		{10, 100, 5},
	} {
		end := int64(bm.numSamples)
		head := makeHead(bm.numShards, bm.numSeries, bm.numSamples)
		eseriesSets := make([]storage.SeriesSet, 0, bm.numShards)
		aseriesSets := make([]storage.SeriesSet, 0, bm.numShards)

		s.Run(fmt.Sprintf("%d_%d_%d", bm.numShards, bm.numSeries, bm.numSamples), func() {
			eseriesSets = eseriesSets[:0]
			aseriesSets = aseriesSets[:0]
			for i := 0; i < bm.numShards; i++ {
				eseriesSets = append(eseriesSets, queryOpt(s.T(), head.lsses[i], head.dss[i], start, end, cppbridge.NoDownsampling, matcher))
			}

			for i := bm.numShards - 1; i >= 0; i-- {
				aseriesSets = append(aseriesSets, queryOpt(s.T(), head.lsses[i], head.dss[i], start, end, cppbridge.NoDownsampling, matcher))
			}

			expectedSeriesSet := storage.NewMergeSeriesSet(eseriesSets, storage.ChainedSeriesMerge)
			actualSeriesSet := querier.NewMergeShardSeriesSet(aseriesSets)

			s.Require().Equal(
				storagetest.TimeSeriesFromSeriesSet(expectedSeriesSet, false),
				storagetest.TimeSeriesFromSeriesSet(actualSeriesSet, false),
			)
		})
	}
}

//
// BenchmarkMergeSeriesSet
//

func BenchmarkMergeSeriesSet(b *testing.B) {
	var start int64
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	for _, bm := range []struct {
		numShards, numSeries, numSamples int
	}{
		{2, 100, 5},
		{4, 100, 5},
		{6, 100, 5},
		{8, 100, 5},
		{10, 100, 5},
	} {
		end := int64(bm.numSamples)
		head := makeHead(bm.numShards, bm.numSeries, bm.numSamples)
		seriesSets := make([]storage.SeriesSet, 0, bm.numShards)

		b.Run(fmt.Sprintf("%d_%d_%d", bm.numShards, bm.numSeries, bm.numSamples), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				b.StopTimer()
				seriesSets = seriesSets[:0]
				for i := 0; i < bm.numShards; i++ {
					seriesSets = append(seriesSets, queryOpt(b, head.lsses[i], head.dss[i], start, end, cppbridge.NoDownsampling, matcher))
				}
				b.StartTimer()

				seriesSet := querier.NewMergeShardSeriesSet(seriesSets)

				for seriesSet.Next() {
					_ = 0
				}
			}
		})
	}
}

//
// testHead
//

// testHead is a test head that contains a set of LSS and DataStorage.
type testHead struct {
	lsses []*shard.LSS
	dss   []*shard.DataStorage
}

// makeHead creates a new test head with the given number of shards, series, and samples.
func makeHead(numShards, numSeries, numSamples int) *testHead {
	lsses := make([]*shard.LSS, 0, numShards)
	dss := make([]*shard.DataStorage, 0, numShards)
	for shardID := range numShards {
		lss := shard.NewLSS()
		ds := shard.NewDataStorage()
		timeSeries := makeTimeSeries(numSeries, numSamples, shardID)
		storagetest.MustAppendTimeSeriesToLSSAndDataStorage(lss, ds, timeSeries...)
		lsses = append(lsses, lss)
		dss = append(dss, ds)
	}

	return &testHead{lsses: lsses, dss: dss}
}

// makeTimeSeries creates a new time series with the given number of series, samples, and shard ID.
func makeTimeSeries(numSeries, numSamples, shardID int) []storagetest.TimeSeries {
	timeSeries := make([]storagetest.TimeSeries, 0, numSeries)
	for j := range numSeries {
		ls := labels.FromStrings(
			"__name__", "metric",
			"foo", fmt.Sprintf("bar%d", j),
			"shard_id", fmt.Sprintf("id_%d", shardID),
		)

		samples := make([]cppbridge.Sample, 0, numSamples)
		for k := range numSamples {
			samples = append(samples, cppbridge.Sample{Timestamp: int64(k), Value: float64(k)})
		}

		timeSeries = append(timeSeries, storagetest.TimeSeries{Labels: ls, Samples: samples})
	}

	return timeSeries
}
