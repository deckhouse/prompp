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
)

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
		// seriesSets := make([]storage.SeriesSet, 0, bm.numShards)
		seriesSets := make([]*querier.SeriesSet, 0, bm.numShards)

		b.Run(fmt.Sprintf("%d_%d_%d", bm.numShards, bm.numSeries, bm.numSamples), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				b.StopTimer()
				seriesSets = seriesSets[:0]
				for i := 0; i < bm.numShards; i++ {
					seriesSets = append(seriesSets, queryOpt(b, head.lsses[i], head.dss[i], start, end, matcher))
				}
				b.StartTimer()

				// seriesSet := storage.NewMergeSeriesSet(seriesSets, storage.ChainedSeriesMerge)
				// seriesSet := querier.NewMergeShardSeriesSet(seriesSets)
				seriesSet := querier.NewMergeShardSeriesSetGeneric(seriesSets)

				seriesSet.Next()

				// for seriesSet.Next() {
				// 	_ = 0
				// }

				// var iter chunkenc.Iterator
				// for seriesSet.Next() {
				// 	iter = seriesSet.At().Iterator(iter)
				// 	for iter.Next() == chunkenc.ValFloat {
				// 		ts, v := iter.At()
				// 		_, _ = ts, v
				// 	}
				// }
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
	for shardID := 0; shardID < numShards; shardID++ {
		lss := shard.NewLSS()
		ds := shard.NewDataStorage()
		appendSeries(lss, ds, numSeries, numSamples, shardID)
		lsses = append(lsses, lss)
		dss = append(dss, ds)
	}

	return &testHead{lsses: lsses, dss: dss}
}

// appendSeries appends a series to the given LSS and DataStorage.
func appendSeries(lss *shard.LSS, ds *shard.DataStorage, numSeries, numSamples, shardID int) {
	timeSeries := make([]storagetest.TimeSeries, 0, numSeries)
	for j := 0; j < numSeries; j++ {
		ls := labels.FromStrings(
			"__name__", "metric",
			"foo", fmt.Sprintf("bar%d", j),
			"shard_id", fmt.Sprintf("id_%d", shardID),
		)
		samples := make([]cppbridge.Sample, 0, numSamples)
		for k := 0; k < numSamples; k++ {
			samples = append(samples, cppbridge.Sample{Timestamp: int64(k), Value: float64(k)})
		}
		timeSeries = append(timeSeries, storagetest.TimeSeries{Labels: ls, Samples: samples})
	}

	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(lss, ds, timeSeries...)
}
