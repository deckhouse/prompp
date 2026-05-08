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
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
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
					esets = append(esets, queryOpt(s.T(),
						head.lsses[i],
						head.dss[i],
						start,
						end,
						cppbridge.NoDownsampling,
						matcher,
					))
					asets = append(asets, queryOpt(s.T(),
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
		evenNumbered := j%2 == 0
		ls := labels.FromStrings(
			"__name__", "metric",
			"even_numbered", fmt.Sprintf("%t", evenNumbered),
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

//
//
//

func TestAGGSS(t *testing.T) {
	hints := &storage.SelectHints{
		Start: 0,
		End:   6,
		Step:  1,
		Func:  "sum",
		Range: 1,
		Grouping: []string{
			// "shard_id",
			// "even_numbered",
			"head_id",
		},
		By: true,
	}

	shardID := uint16(0)
	head := makeHead(2, 10, 5)

	selector, snapshot, err := head.lsses[shardID].QuerySelector(
		shardID,
		[]model.LabelMatcher{{
			Name:        "__name__",
			Value:       "metric",
			MatcherType: model.MatcherTypeExactMatch,
		}},
	)
	require.NoError(t, err)

	lssQueryResult := snapshot.Query(selector)
	require.Equal(t, cppbridge.LSSQueryStatusMatch, lssQueryResult.Status())
	seriesIDs := lssQueryResult.IDs()
	t.Log("seriesIDs", seriesIDs)

	nameIDs := make([]uint32, len(hints.Grouping))
	t.Log("nameIDs empty", nameIDs)
	head.lsses[0].LabelNameToIDs(hints.Grouping, nameIDs)
	t.Log("nameIDs filled", nameIDs)

	seriesGroups := head.lsses[0].GroupSeriesByLabelNames(seriesIDs, nameIDs)
	t.Log("seriesGroups", seriesGroups.Groups)

	// t.Log("createLabelSet", aggLabelSetCtor(snapshot, hints.Grouping, "headID", seriesGroups.Groups[0][0], shardID))
	// t.Log("createLabelSet", aggLabelSetCtor(snapshot, hints.Grouping, "headID", seriesGroups.Groups[1][0], shardID))

	result := head.dss[shardID].Query(
		cppbridge.DataStorageQuery{
			StartTimestampMs: hints.Start,
			EndTimestampMs:   hints.End,
			LabelSetIDs:      seriesIDs,
		},
		cppbridge.NoDownsampling,
		hints,
	)

	ss := querier.NewAggSeriesSet(
		result.SerializedData,
		snapshot,
		seriesGroups,
		hints.Start,
		hints.End,
		hints.Grouping,
		"headID",
		shardID,
	)

	var it chunkenc.Iterator
	for ss.Next() {
		s := ss.At()
		t.Log(s.Labels())
		it = s.Iterator(it)
		t.Log(it.Next())
		for it.Next() != chunkenc.ValNone {
			ts, v := it.At()
			t.Log(ts, v)
		}
	}
}

type el struct {
	a int
	b string
}

func (e *el) String() string {
	return fmt.Sprintf("el{a: %d, b: %s}", e.a, e.b)
}

type elem struct {
	value el
}

func (e *elem) Get() *el {
	return &e.value
}

func TestXxx(t *testing.T) {
	e := elem{value: el{a: 1, b: "2"}}
	el1 := e.Get()
	t.Log(el1)

	e.value = el{a: 3, b: "4"}
	el2 := e.Get()
	t.Log(el1, el2)
}
