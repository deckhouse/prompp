package querier_test

import (
	"fmt"
	"runtime"
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
)

func iterateSeriesSet(seriesSet storage.SeriesSet) {
	var iterator chunkenc.Iterator
	var series storage.Series
	for seriesSet.Next() {
		series = seriesSet.At()
		iterator = series.Iterator(iterator)
		for iterator.Next() != chunkenc.ValNone {
			ts, v := iterator.At()
			_, _ = ts, v
		}
	}
}

func queryOpt(t testing.TB, lss *shard.LSS, ds *shard.DataStorage, start, end int64, matchers ...model.LabelMatcher) *querier.SeriesSet {
	selector, snapshot, err := lss.QuerySelector(0, matchers)
	require.NoError(t, err)
	if selector == 0 || snapshot == nil {
		return &querier.SeriesSet{}
	}

	lssQueryResult := snapshot.Query(selector)
	if lssQueryResult.Status() == cppbridge.LSSQueryStatusNoMatch {
		return &querier.SeriesSet{}
	}

	dsQueryResult := ds.Query(cppbridge.DataStorageQuery{
		StartTimestampMs: start,
		EndTimestampMs:   end,
		LabelSetIDs:      lssQueryResult.IDs(),
	})

	require.Equal(t, cppbridge.DataStorageQueryStatusSuccess, dsQueryResult.Status)
	return querier.NewSeriesSet(start, end, lssQueryResult, snapshot, dsQueryResult.SerializedData)
}

func BenchmarkSeriesSetOpt(b *testing.B) {
	size := 500000
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	var start int64 = 0
	var end = int64(size)
	lss := shard.NewLSS()
	ds := shard.NewDataStorage()
	prepareData(lss, ds, size)

	for b.Loop() {
		b.StopTimer()
		runtime.GC()
		runtime.GC()
		seriesSet := queryOpt(b, lss, ds, start, end, matcher)
		b.StartTimer()

		iterateSeriesSet(seriesSet)
	}
}

func prepareData(lss *shard.LSS, ds *shard.DataStorage, size int) {
	timeSeries := make([]storagetest.TimeSeries, 0, size)
	for i := 0; i < size; i++ {
		label := fmt.Sprintf("index_%d", i%10000)
		timeSeries = append(timeSeries, storagetest.TimeSeries{
			Labels: labels.FromStrings("__name__", "metric", "job", label, "container", "", "id", "/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod37ce076d8d523c8b0c8c0b6191d927f6.slice/cri-containerd-bdd69edcd2fb187baa3381810051e8cc7b8a0d0368e168040f93adb3260582b2.scope", "image", "registry.k8s.io/pause:3.8", "name", "bdd69edcd2fb187baa3381810051e8cc7b8a0d0368e168040f93adb3260582b2", "namespace", "kube-system", "pod", "kube-scheduler-m1.k8s.lan"),
			Samples: []cppbridge.Sample{
				{Timestamp: int64(i), Value: float64(i)},
			},
		})
	}
	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(lss, ds, timeSeries...)
}
