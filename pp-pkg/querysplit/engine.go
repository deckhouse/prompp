package querysplit

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
)

const (
	// DefaultMergeFactor is the default minimum length of a sub-range in look-back windows.
	DefaultMergeFactor = 1

	// DefaultMinBlocks is the default minimum number of blocks that the data of a split query
	// touches.
	DefaultMinBlocks = 4

	// DefaultMinSeries is the default minimum number of head series that the selectors of a split
	// query match.
	DefaultMinSeries = 100

	// defaultLookbackDelta is the default lookback delta of promql.Engine.
	defaultLookbackDelta = 5 * time.Minute
)

// defaultSettings are the split settings of a request with an empty [Header].
var defaultSettings = Settings{
	Parallelism: DefaultParallelism,
	MergeFactor: DefaultMergeFactor,
	MinBlocks:   DefaultMinBlocks,
	MinSeries:   DefaultMinSeries,
}

// Options configures the splitting [Engine].
type Options struct {
	// SeriesCount returns the number of head series that match a selector. Nil disables the
	// check of the minimum number of series.
	SeriesCount SeriesCountFunc
	// LookbackDelta is the default lookback delta of the inner engine. It is used when the
	// query options do not set a lookback delta.
	LookbackDelta time.Duration
	// Registerer registers the split metrics. Nil disables the metrics.
	Registerer prometheus.Registerer
}

// SeriesCountFunc returns the number of head series that match the label matchers.
type SeriesCountFunc func(ctx context.Context, matchers []*labels.Matcher) (int, error)

// Engine is a [promql.QueryEngine] that splits range queries near the persisted block bounds.
// It splits only the queries of the requests with [Header]. It runs the sub-queries concurrently
// on the inner engine and merges their results. Instant queries go to the inner engine without
// changes.
type Engine struct {
	inner         promql.QueryEngine
	bounds        BoundsFunc
	defaults      Settings
	seriesCount   SeriesCountFunc
	lookbackDelta time.Duration
	metrics       *metrics
}

var _ promql.QueryEngine = (*Engine)(nil)

// New returns a splitting engine over inner. bounds returns the block bounds for each query.
func New(inner promql.QueryEngine, bounds BoundsFunc, opts Options) *Engine {
	lookbackDelta := opts.LookbackDelta
	if lookbackDelta <= 0 {
		lookbackDelta = defaultLookbackDelta
	}

	return &Engine{
		inner:         inner,
		bounds:        bounds,
		defaults:      defaultSettings,
		seriesCount:   opts.SeriesCount,
		lookbackDelta: lookbackDelta,
		metrics:       newMetrics(opts.Registerer),
	}
}

// NewInstantQuery implements [promql.QueryEngine]. It does not split the query.
func (e *Engine) NewInstantQuery(
	ctx context.Context,
	q storage.Queryable,
	opts promql.QueryOpts,
	qs string,
	ts time.Time,
) (promql.Query, error) {
	return e.inner.NewInstantQuery(ctx, q, opts, qs, ts)
}

// NewRangeQuery implements [promql.QueryEngine]. It creates all sub-queries of a split query.
// It sends a query that it does not split, or that does not parse, to the inner engine.
func (e *Engine) NewRangeQuery(
	ctx context.Context,
	q storage.Queryable,
	opts promql.QueryOpts,
	qs string,
	start, end time.Time,
	interval time.Duration,
) (promql.Query, error) {
	s := e.settings(ctx)
	if s.Parallelism < 2 {
		return e.inner.NewRangeQuery(ctx, q, opts, qs, start, end, interval)
	}

	ranges, expr, modifier := e.plan(s, opts, qs, start, end, interval)
	if len(ranges) < 2 || !e.enoughSeries(ctx, expr, s.MinSeries) {
		e.metrics.notSplit()
		return e.inner.NewRangeQuery(ctx, q, opts, qs, start, end, interval)
	}

	subQS := expr.String()
	sq := &splitQuery{
		qs:       qs,
		stmt:     &parser.EvalStmt{Expr: expr, Start: start, End: end, Interval: interval},
		ranges:   ranges,
		subs:     make([]promql.Query, 0, len(ranges)),
		modifier: modifier,
		limit:    s.Parallelism,
	}
	for _, r := range ranges {
		sub, err := e.inner.NewRangeQuery(ctx, q, opts, subQS, time.UnixMilli(r.start), time.UnixMilli(r.end), interval)
		if err != nil {
			sq.Close()
			return nil, err
		}

		sq.subs = append(sq.subs, sub)
	}

	e.metrics.split(len(ranges))
	return sq, nil
}

// settings returns the split settings of the request: the default settings with the settings from
// [Header] over them. Without the header, the parallelism is zero, and the split is off.
func (e *Engine) settings(ctx context.Context) Settings {
	h, ok := settingsFrom(ctx)
	if !ok {
		return Settings{}
	}

	return e.defaults.overlay(h)
}

// enoughSeries reports whether the selectors of expr match at least minSeries head series. A query
// over a few series gets little from the parallel run, but it pays for the extra index lookups of
// each sub-query. It returns false if the count fails.
func (e *Engine) enoughSeries(ctx context.Context, expr parser.Expr, minSeries int) bool {
	if e.seriesCount == nil {
		return true
	}

	total := 0
	for _, matchers := range parser.ExtractSelectors(expr) {
		n, err := e.seriesCount(ctx, matchers)
		if err != nil {
			return false
		}

		total += n
		if total >= minSeries {
			return true
		}
	}

	return false
}

// plan parses qs and returns the sub-ranges, the expression for the sub-queries and the
// op_top result modifier. It returns no sub-ranges if the query must not be split.
func (e *Engine) plan(
	s Settings,
	opts promql.QueryOpts,
	qs string,
	start, end time.Time,
	interval time.Duration,
) ([]subRange, parser.Expr, func(promql.Matrix) promql.Matrix) {
	if e.bounds == nil || interval <= 0 {
		return nil, nil, nil
	}

	expr, err := parser.ParseExpr(qs)
	if err != nil {
		return nil, nil, nil
	}

	if t := expr.Type(); t != parser.ValueTypeVector && t != parser.ValueTypeScalar {
		return nil, nil, nil
	}

	if hasVaryingAggregationParam(expr) {
		return nil, nil, nil
	}

	startMs, endMs, stepMs := start.UnixMilli(), end.UnixMilli(), interval.Milliseconds()

	// op_top selects series by all points of the range, so the merged result gets it.
	expr, modifier, err := promql.ExtractOptTop(expr, startMs, endMs, stepMs)
	if err != nil {
		return nil, nil, nil
	}

	lookbackDelta := e.lookbackDelta
	if opts != nil && opts.LookbackDelta() > 0 {
		lookbackDelta = opts.LookbackDelta()
	}

	window := lookbackWindow(expr, lookbackDelta).Milliseconds()
	bounds := e.bounds()
	// A query over a few blocks gets little from the parallel run, but it pays for the
	// extra index lookups of each sub-query.
	if countBlocks(bounds, startMs-window, endMs) < s.MinBlocks {
		return nil, nil, nil
	}

	ranges := planSubRanges(startMs, endMs, stepMs, bounds, window, s.MergeFactor)
	if len(ranges) < 2 {
		return nil, nil, nil
	}

	resolveStartEnd(expr, startMs, endMs)
	return ranges, expr, modifier
}
