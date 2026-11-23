package block

import (
	"context"
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/util/annotations"
)

// blockBenchmarkConfig defines the source series and query sampling for one synthetic workload.
type blockBenchmarkConfig struct {
	seriesCount int
	sampleCount int
	interval    int64
	jitter      int64
	step        time.Duration
	patterns    []string
}

// blockBenchmarkSample holds an unencoded float sample for the query result oracle.
type blockBenchmarkSample struct {
	t int64
	v float64
}

func (s blockBenchmarkSample) F() float64                    { return s.v }
func (s blockBenchmarkSample) FH() *histogram.FloatHistogram { return nil }
func (s blockBenchmarkSample) H() *histogram.Histogram       { return nil }
func (s blockBenchmarkSample) T() int64                      { return s.t }
func (s blockBenchmarkSample) Type() chunkenc.ValueType      { return chunkenc.ValFloat }

// blockBenchmarkSeriesSet iterates over filtered, sorted source series without chunk encoding.
type blockBenchmarkSeriesSet struct {
	series []storage.Series
	index  int
}

func (s *blockBenchmarkSeriesSet) At() storage.Series { return s.series[s.index] }
func (*blockBenchmarkSeriesSet) Err() error           { return nil }
func (s *blockBenchmarkSeriesSet) Next() bool {
	s.index++

	return s.index < len(s.series)
}
func (*blockBenchmarkSeriesSet) Warnings() annotations.Annotations { return nil }

// BenchmarkPersistedBlockRead measures sample decoding and PromQL over the actual Prom++ block reader.
// Blocks are created and warmed outside the timer; measured queries include construction and cleanup.
func BenchmarkPersistedBlockRead(b *testing.B) {
	benchmarkPersistedBlockRead(b, blockBenchmarkConfig{
		seriesCount: 1024, sampleCount: 601, interval: 10_000, jitter: 37, step: time.Minute,
		patterns: []string{"constant", "counter", "gauge", "jitter_counter"},
	})
}

// BenchmarkPersistedBlockReadVariants varies cardinality, query step, scrape interval, and value distribution.
func BenchmarkPersistedBlockReadVariants(b *testing.B) {
	for _, config := range []blockBenchmarkConfig{
		{seriesCount: 128, sampleCount: 121, interval: 15_000, jitter: 113, step: 15 * time.Second},
		{seriesCount: 2048, sampleCount: 121, interval: 30_000, step: 2 * time.Minute},
	} {
		config.patterns = []string{"staircase", "jitter_constant", "gauge"}
		b.Run(fmt.Sprintf("series=%d/step=%s", config.seriesCount, config.step), func(b *testing.B) {
			benchmarkPersistedBlockRead(b, config)
		})
	}
}

func benchmarkPersistedBlockRead(b *testing.B, config blockBenchmarkConfig) {
	seriesCount, sampleCount := config.seriesCount, config.sampleCount
	maxTime := int64(sampleCount-1)*config.interval + 1000
	ctx := context.Background()
	engine := promql.NewEngine(promql.EngineOpts{MaxSamples: 10_000_000, Timeout: time.Minute})
	b.Cleanup(func() { require.NoError(b, engine.Close()) })

	for _, pattern := range config.patterns {
		b.Run(pattern, func(b *testing.B) {
			series := make([]storage.Series, seriesCount)

			for i := range series {
				samples := make([]chunks.Sample, sampleCount)

				for j := range samples {
					sample := blockBenchmarkSample{t: int64(j) * config.interval, v: float64(i + j)}

					switch pattern {
					case "constant":
						sample.v = float64(i)
					case "gauge":
						sample.v = float64(i) + math.Sin(float64(j)*0.1)
					case "staircase":
						sample.v = float64(i + j/16)
					case "jitter_constant":
						sample.v = float64(i)
						sample.t += int64(j%7) * config.jitter
					case "jitter_counter":
						sample.t += int64(j%7) * config.jitter
					}

					samples[j] = sample
				}

				series[i] = storage.NewListSeries(labels.FromStrings(
					"__name__", "block_metric_total", "instance", fmt.Sprintf("%06d", i),
					"job", fmt.Sprintf("job%02d", i%16), "region", fmt.Sprintf("r%d", i%4),
				), samples)
			}
			reference := benchmarkSourceQueryable(series)

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
				b.ReportMetric(float64(seriesCount*sampleCount), "samples/op")
			})

			for _, tc := range []struct {
				name string
				expr string
			}{
				{"sum_by", "sum by(job)(block_metric_total)"},
				{"filtered_sum_by", "sum by(job)(block_metric_total{region='r0'})"},
				{
					"sparse_sum_by",
					// All configured series counts are multiples of 4, so seriesCount-4 is the last index in region r0.
					fmt.Sprintf("sum by(job)(block_metric_total{region='r0',instance='%06d'})", seriesCount-4),
				},
				{"sum_rate", "sum by(job)(rate(block_metric_total{region='r0'}[5m]))"},
			} {
				b.Run(tc.name, func(b *testing.B) {
					func() {
						expectedQuery, err := engine.NewRangeQuery(ctx, reference, nil, tc.expr,
							time.Unix(0, 0), time.UnixMilli(maxTime-1000), config.step)
						require.NoError(b, err)
						defer expectedQuery.Close()
						expected := expectedQuery.Exec(ctx)
						require.NoError(b, expected.Err)
						require.NotEmpty(b, expected.Value.(promql.Matrix))
						warmQuery, err := engine.NewRangeQuery(ctx, queryable, nil, tc.expr,
							time.Unix(0, 0), time.UnixMilli(maxTime-1000), config.step)
						require.NoError(b, err)
						defer warmQuery.Close()
						warm := warmQuery.Exec(ctx)
						require.NoError(b, warm.Err)
						require.Equal(b, expected.Value, warm.Value)
					}()

					b.ReportAllocs()

					for b.Loop() {
						query, err := engine.NewRangeQuery(ctx, queryable, nil, tc.expr,
							time.Unix(0, 0), time.UnixMilli(maxTime-1000), config.step)
						if err != nil {
							b.Fatal(err)
						}

						result := query.Exec(ctx)
						nonEmpty := result.Err == nil && len(result.Value.(promql.Matrix)) != 0
						query.Close()

						if result.Err != nil {
							b.Fatal(result.Err)
						}

						if !nonEmpty {
							b.Fatal("empty query result")
						}
					}
				})
			}
		})
	}
}

func benchmarkSourceQueryable(series []storage.Series) storage.Queryable {
	return &storage.MockQueryable{MockQuerier: &storage.MockQuerier{
		SelectMockFunction: func(_ bool, _ *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
			selected := make([]storage.Series, 0, len(series))

			for _, candidate := range series {
				matches := true

				for _, matcher := range matchers {
					if !matcher.Matches(candidate.Labels().Get(matcher.Name)) {
						matches = false
						break
					}
				}

				if matches {
					selected = append(selected, candidate)
				}
			}

			slices.SortFunc(selected, func(a, b storage.Series) int { return labels.Compare(a.Labels(), b.Labels()) })

			return &blockBenchmarkSeriesSet{series: selected, index: -1}
		},
	}}
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
