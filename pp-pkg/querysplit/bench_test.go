package querysplit

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"
	"sync"
	"syscall"
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

// Benchmark data: benchSeries counters and benchSeries gauges, scraped every benchScrape,
// in 2h blocks for benchDuration.
const (
	benchSeries   = 2000
	benchScrape   = 15 * time.Second
	benchBlock    = 2 * time.Hour
	benchDuration = 24 * time.Hour
)

var (
	benchOnce   sync.Once
	benchBlocks []*tsdb.Block
	benchErr    error
)

// openBenchBlocks writes the benchmark blocks to a temporary directory one time and opens them.
// QUERYSPLIT_BENCH_DIR selects a directory to keep the blocks between runs.
func openBenchBlocks(b *testing.B) []*tsdb.Block {
	benchOnce.Do(func() {
		dir := os.Getenv("QUERYSPLIT_BENCH_DIR")
		if dir == "" {
			dir, benchErr = os.MkdirTemp("", "querysplit-bench")
			if benchErr != nil {
				return
			}
		}

		entries, _ := os.ReadDir(dir)
		if len(entries) == 0 {
			if benchErr = writeBenchBlocks(dir); benchErr != nil {
				return
			}
		}

		benchBlocks, _, benchErr = tsdb.OpenBlocks(log.NewNopLogger(), dir, nil, chunkenc.NewPool())
	})
	require.NoError(b, benchErr)
	return benchBlocks
}

func writeBenchBlocks(dir string) error {
	rnd := rand.New(rand.NewPCG(1, 2))
	counters := make([]float64, benchSeries)
	lsets := make([]labels.Labels, 0, 2*benchSeries)
	for i := range benchSeries {
		common := []string{
			"job", "job-" + strconv.Itoa(i%10),
			"instance", "instance-" + strconv.Itoa(i%200),
			"pod", "pod-" + strconv.Itoa(i),
		}
		lsets = append(lsets,
			labels.FromStrings(append([]string{"__name__", "requests_total"}, common...)...),
			labels.FromStrings(append([]string{"__name__", "memory_bytes"}, common...)...),
		)
	}

	ctx := context.Background()
	for blockStart := time.Duration(0); blockStart < benchDuration; blockStart += benchBlock {
		w, err := tsdb.NewBlockWriter(log.NewNopLogger(), dir, benchBlock.Milliseconds())
		if err != nil {
			return err
		}

		app := w.Appender(ctx)
		refs := make([]storage.SeriesRef, len(lsets))
		for ts := blockStart; ts < blockStart+benchBlock; ts += benchScrape {
			for i := range benchSeries {
				counters[i] += float64(rnd.IntN(100))
				if refs[2*i], err = app.Append(refs[2*i], lsets[2*i], ts.Milliseconds(), counters[i]); err != nil {
					return err
				}
				gauge := 1e9 + float64(rnd.IntN(1e6))
				if refs[2*i+1], err = app.Append(refs[2*i+1], lsets[2*i+1], ts.Milliseconds(), gauge); err != nil {
					return err
				}
			}
		}
		if err = app.Commit(); err != nil {
			return err
		}
		if _, err = w.Flush(ctx); err != nil {
			return err
		}
		if err = w.Close(); err != nil {
			return err
		}
	}

	return nil
}

// blocksQueryable reads the blocks like manager.Manager.Querier.
type blocksQueryable []*tsdb.Block

func (bq blocksQueryable) Querier(mint, maxt int64) (storage.Querier, error) {
	queriers := make([]storage.Querier, 0, len(bq))
	for _, b := range bq {
		if !b.OverlapsClosedInterval(mint, maxt) {
			continue
		}
		q, err := tsdb.NewBlockQuerier(b, mint, maxt)
		if err != nil {
			return nil, err
		}
		queriers = append(queriers, q)
	}
	return storage.NewMergeQuerier(queriers, nil, storage.ChainedSeriesMerge), nil
}

func (bq blocksQueryable) bounds() []int64 {
	bounds := make([]int64, 0, 2*len(bq))
	for _, b := range bq {
		bounds = append(bounds, b.Meta().MinTime, b.Meta().MaxTime)
	}
	return SortBounds(bounds)
}

// cpuTime returns the user and system CPU time of the process.
func cpuTime() time.Duration {
	var ru syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}

// BenchmarkRangeQuery compares range queries over 24h of 2h blocks without a split (p=0) and
// with a split at different parallelism (p) and merge factors (k). cpu-ms/op is the CPU time
// of the process for one query.
func BenchmarkRangeQuery(b *testing.B) {
	blocks := blocksQueryable(openBenchBlocks(b))
	bounds := blocks.bounds()
	inner := promql.NewEngine(promql.EngineOpts{
		MaxSamples:           1e9,
		Timeout:              time.Hour,
		EnableAtModifier:     true,
		EnableNegativeOffset: true,
	})
	b.Cleanup(func() { _ = inner.Close() })

	start := time.UnixMilli(0).Add(time.Hour)
	end := time.UnixMilli(0).Add(benchDuration)

	type config struct{ parallelism, mergeFactor int }
	configs := []config{{0, 0}, {1, 4}, {2, 4}, {4, 4}, {8, 4}, {8, 1}, {8, 2}}

	for _, query := range []struct {
		name string
		qs   string
		step time.Duration
	}{
		{name: "sum_rate_5m", qs: `sum(rate(requests_total[5m]))`, step: time.Minute},
		{name: "sum_by_job_rate_5m", qs: `sum by (job) (rate(requests_total[5m]))`, step: time.Minute},
		{name: "rate_5m_all_series", qs: `rate(requests_total[5m])`, step: 5 * time.Minute},
		{name: "one_series_rate_5m", qs: `rate(requests_total{pod="pod-7"}[5m])`, step: time.Minute},
		{name: "avg_over_time_1h", qs: `sum(avg_over_time(memory_bytes[1h]))`, step: time.Minute},
		{name: "max_over_time_3h", qs: `max(max_over_time(memory_bytes[3h]))`, step: 5 * time.Minute},
		{name: "sum_rate_5m_step_15m", qs: `sum(rate(requests_total[5m]))`, step: 15 * time.Minute},
	} {
		for _, cfg := range configs {
			var engine promql.QueryEngine = inner
			if cfg.parallelism > 0 {
				e := New(inner, func() []int64 { return bounds }, Options{})
				e.defaults.Parallelism = cfg.parallelism
				e.defaults.MergeFactor = cfg.mergeFactor
				engine = e
			}

			b.Run(fmt.Sprintf("%s/p=%d/k=%d", query.name, cfg.parallelism, cfg.mergeFactor), func(b *testing.B) {
				ctx := withHeader(context.Background())
				var subQueries int
				cpuStart := cpuTime()
				b.ResetTimer()
				for range b.N {
					q, err := engine.NewRangeQuery(ctx, blocks, nil, query.qs, start, end, query.step)
					if err != nil {
						b.Fatal(err)
					}
					subQueries = 1
					if sq, ok := q.(*splitQuery); ok {
						subQueries = len(sq.subs)
					}
					if res := q.Exec(ctx); res.Err != nil {
						b.Fatal(res.Err)
					}
					q.Close()
				}
				b.StopTimer()
				b.ReportMetric(float64((cpuTime()-cpuStart).Milliseconds())/float64(b.N), "cpu-ms/op")
				b.ReportMetric(float64(subQueries), "subqueries")
			})
		}
	}
}
