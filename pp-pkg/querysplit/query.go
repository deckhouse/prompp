package querysplit

import (
	"context"
	"sort"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/stats"
)

// splitQuery is a [promql.Query] that runs one sub-query for each sub-range and merges the results.
type splitQuery struct {
	qs       string
	stmt     *parser.EvalStmt
	ranges   []subRange
	subs     []promql.Query
	modifier func(promql.Matrix) promql.Matrix
	limit    int
	timers   *stats.QueryTimers

	cancelMtx sync.Mutex
	cancel    context.CancelFunc
}

var _ promql.Query = (*splitQuery)(nil)

// Exec implements [promql.Query]. The first error of a sub-query cancels the other sub-queries.
// Exec returns that error without changes, so the API maps it as the error of the unsplit query.
func (q *splitQuery) Exec(ctx context.Context) *promql.Result {
	q.timers = stats.NewQueryTimers()
	execTimer := q.timers.GetTimer(stats.ExecTotalTime).Start()
	defer execTimer.Stop()
	evalTimer := q.timers.GetTimer(stats.EvalTotalTime).Start()
	defer evalTimer.Stop()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	q.setCancel(cancel)
	defer q.setCancel(nil)

	results := make([]*promql.Result, len(q.subs))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(q.limit)
	for i, sub := range q.subs {
		g.Go(func() error {
			// A sub-query checks the context itself and returns the engine error for it.
			results[i] = sub.Exec(gctx)
			return results[i].Err
		})
	}
	err := g.Wait()

	var warnings annotations.Annotations
	for _, res := range results {
		if res != nil {
			warnings.Merge(res.Warnings)
		}
	}

	if err != nil {
		return &promql.Result{Err: err, Warnings: warnings}
	}

	sortTimer := q.timers.GetTimer(stats.ResultSortTime).Start()
	mat := mergeMatrices(results)
	sortTimer.Stop()

	if q.modifier != nil {
		mat = q.modifier(mat)
	}

	return &promql.Result{Value: mat, Warnings: warnings}
}

func (q *splitQuery) setCancel(cancel context.CancelFunc) {
	q.cancelMtx.Lock()
	q.cancel = cancel
	q.cancelMtx.Unlock()
}

// Cancel implements [promql.Query].
func (q *splitQuery) Cancel() {
	q.cancelMtx.Lock()
	if q.cancel != nil {
		q.cancel()
	}
	q.cancelMtx.Unlock()

	for _, sub := range q.subs {
		sub.Cancel()
	}
}

// Close implements [promql.Query]. The merged matrix does not use the memory of the sub-query
// results, so it stays valid.
func (q *splitQuery) Close() {
	for _, sub := range q.subs {
		sub.Close()
	}
}

// Statement implements [promql.Query].
func (q *splitQuery) Statement() parser.Statement {
	return q.stmt
}

// String implements [promql.Query].
func (q *splitQuery) String() string {
	return q.qs
}

// Stats implements [promql.Query]. Timers are for the full query. Sample counts are the sums
// for all sub-queries.
func (q *splitQuery) Stats() *stats.Statistics {
	timers := q.timers
	if timers == nil {
		timers = stats.NewQueryTimers()
	}

	return &stats.Statistics{Timers: timers, Samples: q.mergeSamples()}
}

// mergeSamples merges the sample statistics of the sub-queries. PeakSamples is the sum of the
// peaks, because the sub-queries can run at the same time.
func (q *splitQuery) mergeSamples() *stats.QuerySamples {
	var merged *stats.QuerySamples
	for i, sub := range q.subs {
		s := sub.Stats().Samples
		if s == nil {
			continue
		}

		if merged == nil {
			merged = stats.NewQuerySamples(s.EnablePerStepStats)
			merged.InitStepTracking(q.ranges[0].start, q.ranges[len(q.ranges)-1].end, q.stmt.Interval.Milliseconds())
		}

		merged.TotalSamples += s.TotalSamples
		merged.PeakSamples += s.PeakSamples
		if merged.TotalSamplesPerStep != nil && s.TotalSamplesPerStep != nil {
			offset := int((q.ranges[i].start - q.ranges[0].start) / q.stmt.Interval.Milliseconds())
			copy(merged.TotalSamplesPerStep[offset:], s.TotalSamplesPerStep)
		}
	}

	return merged
}

// mergeMatrices merges the matrices of the sub-queries in time order and sorts the series by
// labels. The engine returns the point slices of a result to a pool on Close and can reuse the
// histograms in them, so the merged matrix gets copies.
func mergeMatrices(results []*promql.Result) promql.Matrix {
	var out promql.Matrix
	index := make(map[uint64][]int)
	for _, res := range results {
		mat, _ := res.Value.(promql.Matrix)
		for _, s := range mat {
			h := s.Metric.Hash()
			i := findSeries(out, index[h], s.Metric)
			if i < 0 {
				i = len(out)
				index[h] = append(index[h], i)
				out = append(out, promql.Series{Metric: s.Metric, DropName: s.DropName})
			}

			out[i].Floats = append(out[i].Floats, s.Floats...)
			for _, p := range s.Histograms {
				out[i].Histograms = append(out[i].Histograms, promql.HPoint{T: p.T, H: p.H.Copy()})
			}
		}
	}

	sort.Sort(out)
	return out
}

// findSeries returns the index of the series with labels ls in out, or -1.
// candidates are the indexes of the series with the same labels hash.
func findSeries(out promql.Matrix, candidates []int, ls labels.Labels) int {
	for _, i := range candidates {
		if labels.Equal(out[i].Metric, ls) {
			return i
		}
	}

	return -1
}
