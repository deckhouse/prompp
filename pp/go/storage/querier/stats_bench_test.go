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

// stageStatsMetric is the metric in both the regular and the heavy Select fixtures.
const stageStatsMetric = "stage_metric"

// heavySelectMatches is the number of series that match both filters on the heavy Select path.
const heavySelectMatches = 200_000

// BenchmarkQuerierHeavySelect measures the complete Select call, without iterating the result.
func BenchmarkQuerierHeavySelect(b *testing.B) {
	previous := querier.QueryStageMetricsEnabled
	querier.QueryStageMetricsEnabled = true
	b.Cleanup(func() { querier.QueryStageMetricsEnabled = previous })

	for _, shardCount := range []uint16{1, 4} {
		b.Run(fmt.Sprintf("shards=%d", shardCount), func(b *testing.B) {
			const excludedPerFilter = heavySelectMatches / 8
			h := newStageStatsHeadWithLabels(b, heavySelectMatches+2*excludedPerFilter, shardCount, func(i int) labels.Labels {
				zone, tier := "west", "primary"
				switch {
				case i >= heavySelectMatches+excludedPerFilter:
					tier = "secondary"
				case i >= heavySelectMatches:
					zone = "east"
				}

				return labels.FromStrings(
					labels.MetricName, stageStatsMetric,
					"series", fmt.Sprintf("%07d", heavySelectMatches+2*excludedPerFilter-i),
					"tier", tier,
					"zone", zone,
				)
			})

			for _, stats := range []string{"off", "on"} {
				b.Run("stats="+stats, func(b *testing.B) {
					var metrics *querier.Metrics
					if stats == "on" {
						metrics = querier.NewMetrics(prometheus.NewRegistry(), querier.QueryableAppenderSource)
					}

					for _, sortSeries := range []bool{false, true} {
						b.Run(fmt.Sprintf("sort_%t", sortSeries), func(b *testing.B) {
							query := heavySelectQueries(h, metrics, sortSeries)
							for _, operation := range []string{"instant", "range", "chunks", "parallel_range"} {
								if operation == "chunks" && !sortSeries {
									continue // ChunkQuerier always sorts.
								}

								b.Run(operation, func(b *testing.B) {
									benchmarkHeavySelect(b, query[operation], operation == "parallel_range")
								})
							}
						})
					}
				})
			}
		})
	}
}

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

// benchmarkHeavySelect validates the fixture before timing each Select variant.
func benchmarkHeavySelect(b *testing.B, query func() selectResult, parallel bool) {
	set := query()
	var matched int
	for set.Next() {
		matched++
	}
	require.NoError(b, set.Err())
	require.Equal(b, heavySelectMatches, matched)

	b.ReportAllocs()
	b.ResetTimer()

	if parallel {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				runtime.KeepAlive(query())
			}
		})

		return
	}

	for b.Loop() {
		runtime.KeepAlive(query())
	}
}

func benchmarkQuerierStageStats(
	b *testing.B, h *ppstorage.Head, metrics *querier.Metrics, operation string, parallel bool,
) {
	q := querier.NewQuerier(h, querier.NewNoOpShardedDeduplicator, int64(0), int64(10), 1, 0, metrics)
	cq := querier.NewChunkQuerier(h, querier.NewNoOpShardedDeduplicator, int64(0), int64(10), nil, metrics)
	matcher := labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, stageStatsMetric)
	matchers := []*labels.Matcher{matcher}
	hints := &storage.SelectHints{Start: 0, End: 10, Step: 1}
	ctx := context.Background()
	query := func() {
		switch operation {
		case "instant":
			runtime.KeepAlive(q.Select(ctx, false, &storage.SelectHints{}, matchers...))
		case "range":
			runtime.KeepAlive(q.Select(ctx, false, hints, matchers...))
		case "chunks":
			runtime.KeepAlive(cq.Select(ctx, false, hints, matchers...))
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
	query()
	b.ReportAllocs()
	b.ResetTimer()

	if parallel {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				query()
			}
		})

		return
	}

	for b.Loop() {
		query()
	}
}

// selectResult is the part of the series and chunk sets needed to validate a benchmark fixture.
type selectResult interface {
	Next() bool
	Err() error
}

// heavySelectQueries returns the query variants over the same series distribution.
func heavySelectQueries(h *ppstorage.Head, metrics *querier.Metrics, sortSeries bool) map[string]func() selectResult {
	q := querier.NewQuerier(h, querier.NewNoOpShardedDeduplicator, int64(0), int64(10), 1, 0, metrics)
	cq := querier.NewChunkQuerier(h, querier.NewNoOpShardedDeduplicator, int64(0), int64(10), nil, metrics)
	matchers := []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, stageStatsMetric),
		labels.MustNewMatcher(labels.MatchEqual, "zone", "west"),
		labels.MustNewMatcher(labels.MatchEqual, "tier", "primary"),
	}
	hints := &storage.SelectHints{Start: 0, End: 10, Step: 1}
	instantHints := &storage.SelectHints{}
	ctx := context.Background()
	selectRange := func() selectResult { return q.Select(ctx, sortSeries, hints, matchers...) }

	return map[string]func() selectResult{
		"instant":        func() selectResult { return q.Select(ctx, sortSeries, instantHints, matchers...) },
		"range":          selectRange,
		"chunks":         func() selectResult { return cq.Select(ctx, sortSeries, hints, matchers...) },
		"parallel_range": selectRange,
	}
}

func newStageStatsHead(tb testing.TB, size int, shardCount uint16) *ppstorage.Head {
	tb.Helper()

	return newStageStatsHeadWithLabels(tb, size, shardCount, func(i int) labels.Labels {
		return labels.FromStrings(labels.MetricName, stageStatsMetric, "series", strconv.Itoa(i))
	})
}

// newStageStatsHeadWithLabels distributes the same input series over the requested shard count.
func newStageStatsHeadWithLabels(
	tb testing.TB, size int, shardCount uint16, makeLabels func(int) labels.Labels,
) *ppstorage.Head {
	tb.Helper()

	shards := make([]*shard.Shard, shardCount)

	for id := range shardCount {
		lss := shard.NewLSS()
		ds := shard.NewDataStorage(false, false)
		series := make([]storagetest.TimeSeries, 0, size/int(shardCount)+1)

		for i := int(id); i < size; i += int(shardCount) {
			series = append(series, storagetest.TimeSeries{
				Labels: makeLabels(i),
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
