package querier_test

import (
	"fmt"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/stretchr/testify/require"
	"testing"
)

func prepareInstantData(lss *shard.LSS, ds *shard.DataStorage, timeStamps []int64, size int) {
	timeSeries := make([]storagetest.TimeSeries, 0, size)
	for _, ts := range timeStamps {
		for i := 0; i < size; i++ {
			label := fmt.Sprintf("index_%d", i)
			timeSeries = append(timeSeries, storagetest.TimeSeries{
				Labels: labels.FromStrings("__name__", "metric", "job", label, "container", "", "id", "/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod37ce076d8d523c8b0c8c0b6191d927f6.slice/cri-containerd-bdd69edcd2fb187baa3381810051e8cc7b8a0d0368e168040f93adb3260582b2.scope", "image", "registry.k8s.io/pause:3.8", "name", "bdd69edcd2fb187baa3381810051e8cc7b8a0d0368e168040f93adb3260582b2", "namespace", "kube-system", "pod", "kube-scheduler-m1.k8s.lan"),
				Samples: []cppbridge.Sample{
					{Timestamp: ts, Value: float64(i)},
				},
			})
		}

	}
	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(lss, ds, timeSeries...)
}

func BenchmarkInstantSeriesSet(b *testing.B) {
	size := 500000
	matcher := model.LabelMatcher{
		Name:        "__name__",
		Value:       "metric",
		MatcherType: model.MatcherTypeExactMatch,
	}

	lss := shard.NewLSS()
	ds := shard.NewDataStorage()
	timestamps := []int64{0, 1, 2}
	valueNotFoundTimestampValue := timestamps[0] - 1
	prepareInstantData(lss, ds, timestamps, size)
	b.ResetTimer()
	b.ReportAllocs()
	var iterator chunkenc.Iterator
	for i := 0; i < b.N; i++ {
		seriesSet, err := storagetest.InstantQuery(lss, ds, timestamps[1], valueNotFoundTimestampValue, matcher)
		require.NoError(b, err)
		b.StartTimer()
		iterator = storagetest.IterateSeriesSet(seriesSet, iterator)
		b.StopTimer()
	}
}
