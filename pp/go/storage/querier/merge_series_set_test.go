package querier_test

import (
	"fmt"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
	"github.com/prometheus/prometheus/storage"
)

// mergeSeriesSetMatrix is exercised for happy-path, empty-set, and across-shard order scenarios.
// A single makeHead per row avoids tripling heavy C++/DataStorage setup (same bug class as slow CI).
func mergeSeriesSetMatrix() []struct{ numShards, numSeries, numSamples int } {
	cases := []struct {
		numShards, numSeries, numSamples int
	}{
		{1, 100, 5},
		{2, 100, 5},
		{4, 100, 5},
		{6, 100, 5},
		{8, 100, 5},
		{10, 100, 5},
	}
	if testing.Short() {
		return []struct {
			numShards, numSeries, numSamples int
		}{
			{1, 100, 5},
			{10, 100, 5},
		}
	}
	return cases
}

func assertMergeShardSeriesSetsEqual(s *MergeShardSeriesSetSuite, esets, asets []storage.SeriesSet) {
	expected := storage.NewMergeSeriesSet(esets, storage.ChainedSeriesMerge)
	actual := querier.NewMergeShardSeriesSet(asets)
	// groupSamples: true keeps one row per series; false expands every sample to its own row and
	// makes testify deep-equal quadratic on large inputs (very slow under ASAN on ARM CI).
	s.Require().Equal(
		storagetest.TimeSeriesFromSeriesSet(expected, true),
		storagetest.TimeSeriesFromSeriesSet(actual, true),
	)
}

type MergeShardSeriesSetSuite struct {
	suite.Suite
}

func TestMergeShardSeriesSetSuite(t *testing.T) {
	suite.Run(t, new(MergeShardSeriesSetSuite))
}

//revive:disable-next-line:cognitive-complexity // this is a test.
func (s *MergeShardSeriesSetSuite) TestMergeShardSeriesSetScenarios() {
	var start int64
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	for _, bm := range mergeSeriesSetMatrix() {
		bm := bm
		s.Run(fmt.Sprintf("%d_%d_%d", bm.numShards, bm.numSeries, bm.numSamples), func() {
			// One head for all scenarios: each scenario runs fresh SeriesSet queries on the same data.
			head := makeHead(bm.numShards, bm.numSeries, bm.numSamples)
			end := int64(bm.numSamples)

			s.Run("happy_path", func() {
				esets := make([]storage.SeriesSet, 0, bm.numShards)
				asets := make([]storage.SeriesSet, 0, bm.numShards)
				for i := 0; i < bm.numShards; i++ {
					esets = append(esets, queryOpt(
						s.T(),
						head.lsses[i],
						head.dss[i],
						start,
						end,
						cppbridge.NoDownsampling,
						matcher,
					))
					asets = append(asets, queryOpt(
						s.T(),
						head.lsses[i],
						head.dss[i],
						start,
						end,
						cppbridge.NoDownsampling,
						matcher,
					))
				}
				assertMergeShardSeriesSetsEqual(s, esets, asets)
			})

			s.Run("empty_series_sets_interleaved", func() {
				esets := make([]storage.SeriesSet, 0, bm.numShards*2)
				asets := make([]storage.SeriesSet, 0, bm.numShards*2)
				for i := 0; i < bm.numShards; i++ {
					if i%2 == 0 {
						esets = append(esets, &querier.SeriesSet{})
						asets = append(asets, &querier.SeriesSet{})
					}
					esets = append(esets, queryOpt(
						s.T(),
						head.lsses[i],
						head.dss[i],
						start,
						end,
						cppbridge.NoDownsampling,
						matcher,
					))
					asets = append(asets, queryOpt(
						s.T(),
						head.lsses[i],
						head.dss[i],
						start,
						end,
						cppbridge.NoDownsampling,
						matcher,
					))
				}
				assertMergeShardSeriesSetsEqual(s, esets, asets)
			})

			s.Run("across_shards_reverse_order", func() {
				esets := make([]storage.SeriesSet, 0, bm.numShards)
				asets := make([]storage.SeriesSet, 0, bm.numShards)
				for i := 0; i < bm.numShards; i++ {
					esets = append(esets, queryOpt(
						s.T(),
						head.lsses[i],
						head.dss[i],
						start,
						end,
						cppbridge.NoDownsampling,
						matcher,
					))
				}
				for i := bm.numShards - 1; i >= 0; i-- {
					asets = append(asets, queryOpt(
						s.T(),
						head.lsses[i],
						head.dss[i],
						start,
						end,
						cppbridge.NoDownsampling,
						matcher,
					))
				}
				assertMergeShardSeriesSetsEqual(s, esets, asets)
			})
		})
	}
}

// assertConcatShardSeriesSetsEqual checks that the concatenation of asets yields the sets one after another,
// in their own order (csets, equal to asets), and the same series as the sorted merge of esets.
func assertConcatShardSeriesSetsEqual(s *MergeShardSeriesSetSuite, esets, csets, asets []storage.SeriesSet) {
	expectedSorted := storagetest.TimeSeriesFromSeriesSet(querier.NewMergeShardSeriesSet(esets), true)
	expectedConcat := make([]storagetest.TimeSeries, 0, len(expectedSorted))
	for _, set := range csets {
		expectedConcat = append(expectedConcat, storagetest.TimeSeriesFromSeriesSet(set, true)...)
	}
	actual := storagetest.TimeSeriesFromSeriesSet(querier.NewConcatShardSeriesSet(asets), true)
	s.Require().True(slices.IsSortedFunc(expectedSorted, func(a, b storagetest.TimeSeries) int {
		return labels.Compare(a.Labels, b.Labels)
	}))
	s.Require().ElementsMatch(expectedSorted, actual)
	s.Require().Equal(expectedConcat, actual)
}

func (s *MergeShardSeriesSetSuite) TestConcatShardSeriesSetScenarios() {
	var start int64
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	for _, bm := range mergeSeriesSetMatrix() {
		s.Run(fmt.Sprintf("%d_%d_%d", bm.numShards, bm.numSeries, bm.numSamples), func() {
			head := makeHead(bm.numShards, bm.numSeries, bm.numSamples)
			end := int64(bm.numSamples)

			s.Run("happy_path", func() {
				esets := make([]storage.SeriesSet, 0, bm.numShards)
				csets := make([]storage.SeriesSet, 0, bm.numShards)
				asets := make([]storage.SeriesSet, 0, bm.numShards)
				for i := range bm.numShards {
					esets = append(esets, queryOpt(s.T(), head.lsses[i], head.dss[i], start, end, cppbridge.NoDownsampling, matcher))
					csets = append(csets, queryOpt(s.T(), head.lsses[i], head.dss[i], start, end, cppbridge.NoDownsampling, matcher))
					asets = append(asets, queryOpt(s.T(), head.lsses[i], head.dss[i], start, end, cppbridge.NoDownsampling, matcher))
				}
				assertConcatShardSeriesSetsEqual(s, esets, csets, asets)
			})

			s.Run("empty_series_sets_interleaved", func() {
				esets := make([]storage.SeriesSet, 0, bm.numShards)
				csets := make([]storage.SeriesSet, 0, bm.numShards)
				asets := make([]storage.SeriesSet, 0, bm.numShards*2+1)
				for i := range bm.numShards {
					if i%2 == 0 {
						asets = append(asets, &querier.SeriesSet{})
					}
					esets = append(esets, queryOpt(s.T(), head.lsses[i], head.dss[i], start, end, cppbridge.NoDownsampling, matcher))
					csets = append(csets, queryOpt(s.T(), head.lsses[i], head.dss[i], start, end, cppbridge.NoDownsampling, matcher))
					asets = append(asets, queryOpt(s.T(), head.lsses[i], head.dss[i], start, end, cppbridge.NoDownsampling, matcher))
				}
				asets = append(asets, &querier.SeriesSet{})
				assertConcatShardSeriesSetsEqual(s, esets, csets, asets)
			})
		})
	}
}

func (s *MergeShardSeriesSetSuite) TestConcatShardSeriesSetPooledBufferReuse() {
	// Arrange
	head := makeHead(2, 10, 1)
	matcher := model.LabelMatcher{Name: "__name__", Value: "metric", MatcherType: model.MatcherTypeExactMatch}
	sets := make([]storage.SeriesSet, 0, 2)
	for i := range 2 {
		sets = append(sets, queryOpt(s.T(), head.lsses[i], head.dss[i], 0, 1, cppbridge.NoDownsampling, matcher))
	}

	// Act
	concat := querier.NewConcatShardSeriesSet(sets)
	clear(sets)
	actual := storagetest.TimeSeriesFromSeriesSet(concat, true)

	// Assert
	s.Require().Len(actual, 20)
	s.Require().False(concat.Next())
}

//
// BenchmarkMergeSeriesSet
//

func BenchmarkMergeVsConcatShardSeriesSet(b *testing.B) {
	var start int64
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	for _, bm := range []struct {
		numShards, numSeries int
	}{
		{2, 1000},
		{8, 1000},
		{32, 1000},
		{64, 1000},
	} {
		head := makeHead(bm.numShards, bm.numSeries, 1)
		seriesSets := make([]storage.SeriesSet, 0, bm.numShards)
		for _, ctor := range []struct {
			name string
			fn   func([]storage.SeriesSet) storage.SeriesSet
		}{
			{"merge", querier.NewMergeShardSeriesSet},
			{"concat", querier.NewConcatShardSeriesSet},
		} {
			b.Run(fmt.Sprintf("%s/shards_%d/series_%d", ctor.name, bm.numShards, bm.numSeries), func(b *testing.B) {
				for b.Loop() {
					b.StopTimer()
					seriesSets = seriesSets[:0]
					for i := range bm.numShards {
						seriesSets = append(seriesSets, queryOpt(b, head.lsses[i], head.dss[i], start, 1, cppbridge.NoDownsampling, matcher))
					}
					b.StartTimer()

					seriesSet := ctor.fn(seriesSets)
					for seriesSet.Next() {
						_ = seriesSet.At().Labels()
					}
				}
			})
		}
	}
}

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
		{2, 10, 5},
		{4, 10, 5},
		{6, 10, 5},
		{8, 10, 5},
		{10, 10, 5},
	} {
		end := int64(bm.numSamples)
		head := makeHead(bm.numShards, bm.numSeries, bm.numSamples)
		seriesSets := make([]storage.SeriesSet, 0, bm.numShards)

		b.Run(fmt.Sprintf("%d_%d_%d", bm.numShards, bm.numSeries, bm.numSamples), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				b.StopTimer()
				seriesSets = seriesSets[:0]
				for i := 0; i < bm.numShards; i++ {
					seriesSets = append(seriesSets, queryOpt(
						b,
						head.lsses[i],
						head.dss[i],
						start,
						end,
						cppbridge.NoDownsampling,
						matcher,
					))
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
		ds := shard.NewDataStorage(false, false)
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
		evenNumbered := j%2 == 0
		ls := labels.FromStrings(
			"__name__", "metric",
			"even_numbered", strconv.FormatBool(evenNumbered),
			"foo", fmt.Sprintf("bar%d", j),
			"shard_id", fmt.Sprintf("id_%d", shardID),
		)

		samples := make([]cppbridge.Sample, 0, numSamples)
		for k := range numSamples {
			samples = append(samples, cppbridge.Sample{Timestamp: int64(k + 1), Value: float64(k)})
		}

		timeSeries = append(timeSeries, storagetest.TimeSeries{Labels: ls, Samples: samples})
	}

	return timeSeries
}
