package querysplit

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/parser/posrange"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/stats"
)

// stubEngine is a promql.QueryEngine that creates stubQuery objects with the exec function.
type stubEngine struct {
	exec func(ctx context.Context, q *stubQuery) *promql.Result

	mtx     sync.Mutex
	queries []*stubQuery
}

func (e *stubEngine) NewInstantQuery(_ context.Context, _ storage.Queryable, _ promql.QueryOpts, qs string, ts time.Time) (promql.Query, error) {
	return e.add(qs, ts, ts), nil
}

func (e *stubEngine) NewRangeQuery(_ context.Context, _ storage.Queryable, _ promql.QueryOpts, qs string, start, end time.Time, _ time.Duration) (promql.Query, error) {
	return e.add(qs, start, end), nil
}

func (e *stubEngine) add(qs string, start, end time.Time) *stubQuery {
	e.mtx.Lock()
	defer e.mtx.Unlock()
	q := &stubQuery{engine: e, qs: qs, start: start.UnixMilli(), end: end.UnixMilli(), samples: stats.NewQuerySamples(false)}
	e.queries = append(e.queries, q)
	return q
}

type stubQuery struct {
	engine     *stubEngine
	qs         string
	start, end int64
	samples    *stats.QuerySamples
	closed     atomic.Bool
	canceled   atomic.Bool
}

func (q *stubQuery) Exec(ctx context.Context) *promql.Result { return q.engine.exec(ctx, q) }
func (q *stubQuery) Close()                                  { q.closed.Store(true) }
func (*stubQuery) Statement() parser.Statement               { return nil }
func (q *stubQuery) Stats() *stats.Statistics {
	return &stats.Statistics{Timers: stats.NewQueryTimers(), Samples: q.samples}
}
func (q *stubQuery) Cancel()        { q.canceled.Store(true) }
func (q *stubQuery) String() string { return q.qs }

// newStubSplitEngine returns a splitting engine with bounds every 2h over a stub engine. The merge
// factor is 4, so that the bound at 0h does not give a sub-range of one look-back window.
func newStubSplitEngine(inner *stubEngine, parallelism int, reg prometheus.Registerer) *Engine {
	bounds := everyBound(0, 10*24*time.Hour, 2*time.Hour)
	e := New(inner, func() []int64 { return bounds }, Options{Registerer: reg})
	e.defaults.Parallelism = parallelism
	e.defaults.MergeFactor = 4
	return e
}

// withHeader returns a copy of ctx of a request with an empty [Header].
func withHeader(ctx context.Context) context.Context {
	return WithSettings(ctx, Settings{})
}

// headerEngine is a splitting engine for the requests with an empty [Header].
type headerEngine struct {
	*Engine
}

// NewRangeQuery implements [promql.QueryEngine].
func (e headerEngine) NewRangeQuery(
	ctx context.Context,
	q storage.Queryable,
	opts promql.QueryOpts,
	qs string,
	start, end time.Time,
	interval time.Duration,
) (promql.Query, error) {
	return e.Engine.NewRangeQuery(withHeader(ctx), q, opts, qs, start, end, interval)
}

// newRangeQuery creates a 1m-step range query from 0 to 12h, which the engine splits into 6 sub-queries.
func newRangeQuery(t *testing.T, e *Engine, qs string) promql.Query {
	q, err := e.NewRangeQuery(withHeader(context.Background()), nil, nil, qs, time.UnixMilli(0), time.UnixMilli(ms(12*time.Hour)), time.Minute)
	require.NoError(t, err)
	return q
}

func emptyResult(context.Context, *stubQuery) *promql.Result {
	return &promql.Result{Value: promql.Matrix{}}
}

func TestEngineCreatesSubQueries(t *testing.T) {
	// Arrange
	inner := &stubEngine{exec: emptyResult}
	engine := newStubSplitEngine(inner, 4, nil)

	// Act
	q := newRangeQuery(t, engine, `rate(x[5m]) + y @ end()`)

	// Assert
	require.Len(t, inner.queries, 6)
	require.Equal(t, `rate(x[5m]) + y @ 43200.000`, inner.queries[0].qs)
	require.Equal(t, `rate(x[5m]) + y @ end()`, q.String())
	require.Equal(t, int64(0), inner.queries[0].start)
	require.Equal(t, ms(12*time.Hour), inner.queries[5].end)
}

func TestEngineDoesNotSplit(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query string
		start time.Duration
		end   time.Duration
	}{
		{name: "one sub-range", query: `rate(x[5m])`, start: 10 * time.Minute, end: time.Hour},
		{name: "few blocks", query: `rate(x[5m])`, start: time.Hour, end: 5 * time.Hour},
		{name: "window longer than the range", query: `rate(x[8d])`, start: 0, end: 7 * 24 * time.Hour},
		{name: "parse error", query: `rate(x[5m]`, start: 0, end: 12 * time.Hour},
		{name: "matrix type", query: `x[5m]`, start: 0, end: 12 * time.Hour},
		{name: "varying aggregation parameter", query: `topk(scalar(y), x)`, start: 0, end: 12 * time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			inner := &stubEngine{exec: emptyResult}
			engine := newStubSplitEngine(inner, 4, nil)

			// Act
			q, err := engine.NewRangeQuery(withHeader(context.Background()), nil, nil, tc.query,
				time.UnixMilli(ms(tc.start)), time.UnixMilli(ms(tc.end)), time.Minute)

			// Assert
			require.NoError(t, err)
			require.Len(t, inner.queries, 1)
			require.Same(t, inner.queries[0], q)
		})
	}
}

func TestEngineMinSeries(t *testing.T) {
	errCount := errors.New("count")
	for _, tc := range []struct {
		name          string
		query         string
		counts        map[string]int
		err           error
		expectedSplit bool
		expectedCalls int
	}{
		{name: "enough series", query: `sum(rate(x[5m]))`, counts: map[string]int{"x": 100}, expectedSplit: true, expectedCalls: 1},
		{name: "few series", query: `sum(rate(x[5m]))`, counts: map[string]int{"x": 99}, expectedCalls: 1},
		{name: "sum of selectors", query: `rate(x[5m]) + y`, counts: map[string]int{"x": 60, "y": 40}, expectedSplit: true, expectedCalls: 2},
		{name: "stop at the threshold", query: `rate(x[5m]) + y`, counts: map[string]int{"x": 100, "y": 1}, expectedSplit: true, expectedCalls: 1},
		{name: "count error", query: `rate(x[5m])`, err: errCount, expectedCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			inner := &stubEngine{exec: emptyResult}
			bounds := everyBound(0, 10*24*time.Hour, 2*time.Hour)
			calls := 0
			engine := New(inner, func() []int64 { return bounds }, Options{
				SeriesCount: func(_ context.Context, matchers []*labels.Matcher) (int, error) {
					calls++
					for _, m := range matchers {
						if m.Name == labels.MetricName {
							return tc.counts[m.Value], tc.err
						}
					}
					return 0, tc.err
				},
			})

			// Act
			q := newRangeQuery(t, engine, tc.query)

			// Assert
			_, split := q.(*splitQuery)
			require.Equal(t, tc.expectedSplit, split)
			require.Equal(t, tc.expectedCalls, calls)
		})
	}
}

func TestEngineInstantQuery(t *testing.T) {
	// Arrange
	inner := &stubEngine{exec: emptyResult}
	engine := newStubSplitEngine(inner, 4, nil)

	// Act
	q, err := engine.NewInstantQuery(context.Background(), nil, nil, `rate(x[5m])`, time.UnixMilli(ms(12*time.Hour)))

	// Assert
	require.NoError(t, err)
	require.Len(t, inner.queries, 1)
	require.Same(t, inner.queries[0], q)
}

func TestEngineParallelismLimit(t *testing.T) {
	// Arrange
	var running, peak atomic.Int32
	inner := &stubEngine{exec: func(context.Context, *stubQuery) *promql.Result {
		n := running.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		running.Add(-1)
		return &promql.Result{Value: promql.Matrix{}}
	}}
	engine := newStubSplitEngine(inner, 2, nil)
	q := newRangeQuery(t, engine, `rate(x[5m])`)

	// Act
	res := q.Exec(context.Background())

	// Assert
	require.NoError(t, res.Err)
	require.Equal(t, int32(2), peak.Load())
}

func TestEngineFirstErrorCancelsOtherSubQueries(t *testing.T) {
	// Arrange
	errLimit := promql.ErrTooManySamples("query execution")
	inner := &stubEngine{exec: func(ctx context.Context, q *stubQuery) *promql.Result {
		if q.start == 0 {
			return &promql.Result{Err: errLimit}
		}
		<-ctx.Done()
		return &promql.Result{Err: promql.ErrQueryCanceled("expression evaluation")}
	}}
	engine := newStubSplitEngine(inner, 6, nil)
	q := newRangeQuery(t, engine, `rate(x[5m])`)

	// Act
	res := q.Exec(context.Background())

	// Assert
	require.Equal(t, errLimit, res.Err)
}

func TestEngineTimeoutStopsSubQueries(t *testing.T) {
	// Arrange
	inner := &stubEngine{exec: func(ctx context.Context, _ *stubQuery) *promql.Result {
		<-ctx.Done()
		return &promql.Result{Err: promql.ErrQueryTimeout("expression evaluation")}
	}}
	engine := newStubSplitEngine(inner, 2, nil)
	q := newRangeQuery(t, engine, `rate(x[5m])`)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	// Act
	res := q.Exec(ctx)

	// Assert
	var errTimeout promql.ErrQueryTimeout
	require.ErrorAs(t, res.Err, &errTimeout)
}

func TestEngineCloseAndCancel(t *testing.T) {
	// Arrange
	inner := &stubEngine{exec: emptyResult}
	engine := newStubSplitEngine(inner, 4, nil)
	q := newRangeQuery(t, engine, `rate(x[5m])`)

	// Act
	q.Cancel()
	q.Close()

	// Assert
	for _, sub := range inner.queries {
		require.True(t, sub.canceled.Load())
		require.True(t, sub.closed.Load())
	}
}

func TestEngineMergesResults(t *testing.T) {
	// Arrange
	a, b := labels.FromStrings("s", "a"), labels.FromStrings("s", "b")
	h := &histogram.FloatHistogram{Count: 3, Sum: 1.5}
	warning := annotations.NewPossibleNonCounterInfo("x", posrange.PositionRange{})
	inner := &stubEngine{exec: func(_ context.Context, q *stubQuery) *promql.Result {
		q.samples.TotalSamples = 10
		q.samples.PeakSamples = 3
		mat := promql.Matrix{{Metric: b, Floats: []promql.FPoint{{T: q.start, F: float64(q.start)}}}}
		// Series a is only in the first and in the last sub-range.
		if q.start == 0 || q.end == ms(12*time.Hour) {
			mat = append(mat, promql.Series{Metric: a, Histograms: []promql.HPoint{{T: q.end, H: h}}})
		}
		var warnings annotations.Annotations
		warnings.Add(warning)
		return &promql.Result{Value: mat, Warnings: warnings}
	}}
	engine := newStubSplitEngine(inner, 4, nil)
	q := newRangeQuery(t, engine, `rate(x[5m])`)

	// Act
	res := q.Exec(context.Background())
	q.Close()

	// Assert
	require.NoError(t, res.Err)
	mat := res.Value.(promql.Matrix)
	require.Len(t, mat, 2)
	require.Equal(t, a, mat[0].Metric)
	require.Equal(t, []int64{ms(2*time.Hour + 4*time.Minute), ms(12 * time.Hour)},
		[]int64{mat[0].Histograms[0].T, mat[0].Histograms[1].T})
	require.Equal(t, h, mat[0].Histograms[0].H)
	require.NotSame(t, h, mat[0].Histograms[0].H)
	require.Equal(t, b, mat[1].Metric)
	require.Len(t, mat[1].Floats, 6)
	for i, sub := range inner.queries {
		require.Equal(t, sub.start, mat[1].Floats[i].T)
	}
	require.Len(t, res.Warnings, 1)
	samples := q.Stats().Samples
	require.Equal(t, int64(60), samples.TotalSamples)
	require.Equal(t, 18, samples.PeakSamples)
}

func TestEngineMergesPerStepStats(t *testing.T) {
	// Arrange
	inner := &stubEngine{exec: func(_ context.Context, q *stubQuery) *promql.Result {
		q.samples = stats.NewQuerySamples(true)
		q.samples.InitStepTracking(q.start, q.end, ms(time.Minute))
		for i := range q.samples.TotalSamplesPerStep {
			q.samples.IncrementSamplesAtStep(i, 1)
		}
		return &promql.Result{Value: promql.Matrix{}}
	}}
	engine := newStubSplitEngine(inner, 4, nil)
	q := newRangeQuery(t, engine, `rate(x[5m])`)

	// Act
	res := q.Exec(context.Background())

	// Assert
	require.NoError(t, res.Err)
	perStep := *q.Stats().Samples.TotalSamplesPerStepMap()
	require.Len(t, perStep, 12*60+1)
	for ts, v := range perStep {
		require.Equal(t, 1, v, "step %d", ts)
	}
}

func TestEngineMetrics(t *testing.T) {
	// Arrange
	reg := prometheus.NewRegistry()
	inner := &stubEngine{exec: emptyResult}
	engine := newStubSplitEngine(inner, 4, reg)

	// Act
	newRangeQuery(t, engine, `rate(x[5m])`)
	newRangeQuery(t, engine, `rate(x[1d])`)

	// Assert
	require.InDelta(t, 1, testutil.ToFloat64(engine.metrics.splitTotal), 0)
	require.InDelta(t, 1, testutil.ToFloat64(engine.metrics.notSplitTotal), 0)
	require.Equal(t, 1, testutil.CollectAndCount(reg, "prompp_query_range_split_subqueries"))
}

func TestEngineSubQueryCreationError(t *testing.T) {
	// Arrange
	errCreate := errors.New("create")
	inner := &failingEngine{stubEngine: stubEngine{exec: emptyResult}, failAt: 3, err: errCreate}
	bounds := everyBound(0, 10*24*time.Hour, 2*time.Hour)
	engine := New(inner, func() []int64 { return bounds }, Options{})

	// Act
	_, err := engine.NewRangeQuery(withHeader(context.Background()), nil, nil, `rate(x[5m])`,
		time.UnixMilli(0), time.UnixMilli(ms(12*time.Hour)), time.Minute)

	// Assert
	require.ErrorIs(t, err, errCreate)
	for _, sub := range inner.queries {
		require.True(t, sub.closed.Load())
	}
}

// failingEngine fails the range query with the number failAt.
type failingEngine struct {
	stubEngine
	failAt int
	err    error
}

func (e *failingEngine) NewRangeQuery(ctx context.Context, s storage.Queryable, o promql.QueryOpts, qs string, start, end time.Time, step time.Duration) (promql.Query, error) {
	if len(e.queries) == e.failAt {
		return nil, e.err
	}
	return e.stubEngine.NewRangeQuery(ctx, s, o, qs, start, end, step)
}
