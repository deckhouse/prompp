package querier_test

import (
	"fmt"
	"testing"

	"github.com/go-faker/faker/v4"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
)

func prepareInstantData(lss *shard.LSS, ds *shard.DataStorage, timeStamps []int64, size int) {
	timeSeries := make([]storagetest.TimeSeries, 0, size)
	for range size {
		name := faker.UUIDDigit() + faker.UUIDDigit()
		ls := labels.FromStrings(
			"__name__", "metric",
			"job", fmt.Sprintf("index_%s", faker.UUIDDigit()),
			"container", faker.Word(),
			"id", fmt.Sprintf("/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod%s.slice/cri-containerd-%s.scope", faker.UUIDDigit(), name),
			"image", fmt.Sprintf("registry.k8s.io/%s:3.8", faker.Word()),
			"name", name,
			"namespace", "kube-system",
			"pod", fmt.Sprintf("kube-scheduler-%s", faker.UUIDDigit()),
		)
		samples := make([]cppbridge.Sample, 0, len(timeStamps))
		for _, ts := range timeStamps {
			samples = append(samples, cppbridge.Sample{Timestamp: ts, Value: faker.Latitude()})
		}
		timeSeries = append(timeSeries, storagetest.TimeSeries{
			Labels:  ls,
			Samples: samples,
		})
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

	seriesSets := make([]*querier.InstantSeriesSet, 0, b.N)
	for range b.N {
		seriesSet, err := storagetest.InstantQuery(lss, ds, timestamps[1], valueNotFoundTimestampValue, matcher)
		require.NoError(b, err)
		seriesSets = append(seriesSets, seriesSet)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		iterateSeriesSet(seriesSets[i])
	}
}
