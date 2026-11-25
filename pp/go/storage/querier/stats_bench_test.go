package querier_test

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	ppstorage "github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
	"github.com/prometheus/prometheus/storage"
)

func BenchmarkQuerierStageStats(b *testing.B) {
	previous := querier.QueryStageMetricsEnabled
	querier.QueryStageMetricsEnabled = true
	b.Cleanup(func() { querier.QueryStageMetricsEnabled = previous })

	for _, size := range []int{0, 16, 1000} {
		b.Run(fmt.Sprintf("series=%d", size), func(b *testing.B) {
			h := newStageStatsHead(b, size, numberOfShards)

			for _, enabled := range []bool{false, true} {
				b.Run(fmt.Sprintf("stats=%s", map[bool]string{false: "off", true: "on"}[enabled]), func(b *testing.B) {
					var metrics *querier.Metrics
					if enabled {
						metrics = querier.NewMetrics(prometheus.NewRegistry(), querier.QueryableAppenderSource)
					}

					for _, operation := range []string{
						"instant", "range", "chunks", "label_names", "label_values",
						"chunk_label_names", "chunk_label_values",
					} {
						b.Run(operation, func(b *testing.B) {
							benchmarkQuerierStageStats(b, h, metrics, operation, false)
						})
					}

					if size == 1000 {
						b.Run("parallel_range", func(b *testing.B) {
							benchmarkQuerierStageStats(b, h, metrics, "range", true)
						})
					}
				})
			}
		})
	}
}

func benchmarkQuerierStageStats(
	b *testing.B, h *ppstorage.Head, metrics *querier.Metrics, operation string, parallel bool,
) {
	q := querier.NewQuerier(h, querier.NewNoOpShardedDeduplicator, int64(0), int64(10), 1, 0, metrics)
	cq := querier.NewChunkQuerier(h, querier.NewNoOpShardedDeduplicator, int64(0), int64(10), metrics)
	defer func() {
		qErr, cqErr := q.Close(), cq.Close()
		require.NoError(b, qErr)
		require.NoError(b, cqErr)
	}()

	matcher := labels.MustNewMatcher(labels.MatchEqual, "__name__", "stage_metric")
	matchers := []*labels.Matcher{matcher}
	hints := &storage.SelectHints{Start: 0, End: 10, Step: 1}
	ctx := context.Background()
	if operation == "instant" {
		hints = &storage.SelectHints{}
	}

	query := func(timeSelect bool) {
		switch operation {
		case "instant", "range":
			selected := querier.NewQuerier(h, querier.NewNoOpShardedDeduplicator, int64(0), int64(10), 1, 0, metrics)

			if timeSelect {
				b.StartTimer()
			}

			runtime.KeepAlive(selected.Select(ctx, false, hints, matchers...))

			if timeSelect {
				b.StopTimer()
			}

			if err := selected.Close(); err != nil {
				b.Error(err)
			}
		case "chunks":
			selected := querier.NewChunkQuerier(h, querier.NewNoOpShardedDeduplicator, int64(0), int64(10), metrics)

			if timeSelect {
				b.StartTimer()
			}

			runtime.KeepAlive(selected.Select(ctx, false, hints, matchers...))

			if timeSelect {
				b.StopTimer()
			}

			if err := selected.Close(); err != nil {
				b.Error(err)
			}
		case "label_names":
			_, _, _ = q.LabelNames(ctx, &storage.LabelHints{}, matchers...)
		case "label_values":
			_, _, _ = q.LabelValues(ctx, "series", &storage.LabelHints{}, matchers...)
		case "chunk_label_names":
			_, _, _ = cq.LabelNames(ctx, &storage.LabelHints{}, matchers...)
		case "chunk_label_values":
			_, _, _ = cq.LabelValues(ctx, "series", &storage.LabelHints{}, matchers...)
		}
	}
	query(false)
	b.ReportAllocs()
	b.ResetTimer()

	if parallel {
		// Parallel workers cannot pause the benchmark timer independently.
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				query(false)
			}
		})

		return
	}

	timeSelect := operation == "instant" || operation == "range" || operation == "chunks"

	for b.Loop() {
		if timeSelect {
			b.StopTimer()
		}

		query(timeSelect)
	}
}

func newStageStatsHead(tb testing.TB, size int, shardCount uint16) *ppstorage.Head {
	tb.Helper()

	shards := make([]*shard.Shard, shardCount)

	for id := range shardCount {
		lss := shard.NewLSS()
		ds := shard.NewDataStorage(false, false)
		series := make([]storagetest.TimeSeries, 0, size/int(shardCount)+1)

		for i := int(id); i < size; i += int(shardCount) {
			series = append(series, storagetest.TimeSeries{
				Labels: labels.FromStrings("__name__", "stage_metric", "series", strconv.Itoa(i)),
				Samples: []cppbridge.Sample{
					{Timestamp: 0, Value: float64(i)},
					{Timestamp: 10, Value: float64(i + 1)},
				},
			})
		}

		storagetest.MustAppendTimeSeriesToLSSAndDataStorage(lss, ds, series...)
		shards[id] = shard.NewShard(lss, ds, nil, nil, wal.NewNoopWal(), id)
	}

	h := head.NewHead("stage-stats", shards, shard.NewPerGoroutineShard[wal.NoopWal], nil, 0, nil)
	tb.Cleanup(func() { require.NoError(tb, h.Close()) })

	return h
}
