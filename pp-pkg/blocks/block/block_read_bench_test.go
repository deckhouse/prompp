package block

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// BenchmarkPersistedBlockRead measures sample decoding and PromQL over the actual Prom++ block reader.
// Blocks are created and warmed outside the timer; measured queries include construction and cleanup.
func BenchmarkPersistedBlockRead(b *testing.B) {
	const seriesCount = 1024
	const sampleCount = 601
	const maxTime = int64((sampleCount-1)*10_000 + 1000)
	ctx := context.Background()
	engine := promql.NewEngine(promql.EngineOpts{MaxSamples: 10_000_000, Timeout: time.Minute})
	b.Cleanup(func() { require.NoError(b, engine.Close()) })

	for _, pattern := range []string{"constant", "counter", "gauge", "jitter_counter"} {
		b.Run(pattern, func(b *testing.B) {
			series := make([]storage.Series, seriesCount)

			for i := range series {
				timestamps := make([]int64, sampleCount)
				values := make([]float64, sampleCount)

				for j := range timestamps {
					timestamps[j] = int64(j) * 10_000
					values[j] = float64(i + j)

					switch pattern {
					case "constant":
						values[j] = float64(i)
					case "gauge":
						values[j] = float64(i) + math.Sin(float64(j)*0.1)
					case "jitter_counter":
						timestamps[j] += int64(j%7) * 37
					}
				}

				series[i] = storage.MockSeries(timestamps, values, []string{
					"__name__", "block_metric_total", "instance", fmt.Sprintf("%06d", i),
					"job", fmt.Sprintf("job%02d", i%16), "region", fmt.Sprintf("r%d", i%4),
				})
			}

			dir, err := tsdb.CreateBlock(series, b.TempDir(), 4*maxTime, log.NewNopLogger())
			require.NoError(b, err)
			block, err := OpenBlock(nil, dir, nil)
			require.NoError(b, err)
			b.Cleanup(func() { require.NoError(b, block.Close()) })
			queryable := storage.QueryableFunc(func(mint, maxt int64) (storage.Querier, error) {
				return tsdb.NewBlockQuerier(block, mint, maxt)
			})
			require.Equal(b, seriesCount*sampleCount, readBenchmarkBlock(b, block, maxTime))

			b.Run("scan", func(b *testing.B) {
				var count int
				b.ReportAllocs()

				for b.Loop() {
					count = readBenchmarkBlock(b, block, maxTime)
				}

				b.StopTimer()
				require.Equal(b, seriesCount*sampleCount, count)
				b.ReportMetric(seriesCount*sampleCount, "samples/op")
			})

			for _, tc := range []struct {
				name string
				expr string
			}{
				{"sum_by", "sum by(job)(block_metric_total)"},
				{"filtered_sum_by", "sum by(job)(block_metric_total{region='r0'})"},
				{"sparse_sum_by", "sum by(job)(block_metric_total{region='r0',instance='001020'})"},
				{"sum_rate", "sum by(job)(rate(block_metric_total{region='r0'}[5m]))"},
			} {
				b.Run(tc.name, func(b *testing.B) {
					b.ReportAllocs()

					for b.Loop() {
						query, err := engine.NewRangeQuery(ctx, queryable, nil, tc.expr,
							time.Unix(0, 0), time.UnixMilli(maxTime-1000), time.Minute)
						if err != nil {
							b.Fatal(err)
						}

						result := query.Exec(ctx)
						query.Close()

						if result.Err != nil {
							b.Fatal(result.Err)
						}

						if len(result.Value.(promql.Matrix)) == 0 {
							b.Fatal("empty query result")
						}
					}
				})
			}
		})
	}
}

func readBenchmarkBlock(tb testing.TB, block *Block, maxt int64) int {
	tb.Helper()
	querier, err := tsdb.NewBlockQuerier(block, 0, maxt)
	require.NoError(tb, err)
	matcher := labels.MustNewMatcher(labels.MatchEqual, "__name__", "block_metric_total")
	series := querier.Select(context.Background(), false, nil, matcher)
	var iterator chunkenc.Iterator
	count := 0

	for series.Next() {
		iterator = series.At().Iterator(iterator)

		for iterator.Next() == chunkenc.ValFloat {
			iterator.At()
			count++
		}

		require.NoError(tb, iterator.Err())
	}

	err = series.Err()
	closeErr := querier.Close()
	require.NoError(tb, err)
	require.NoError(tb, closeErr)

	return count
}
