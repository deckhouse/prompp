package promql

import (
	"context"
	"strings"
	"time"

	"github.com/go-kit/log/level"
	"go.opentelemetry.io/otel/trace"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/util/stats"
)

//
// Engine
//

func (ng *Engine) logExpensiveQuery(ctx context.Context, q *query, result *Result) {
	cost := newQueryCost(q, result)
	reasons := cost.reasons(ng.expensiveQueryLogging)

	if len(reasons) == 0 {
		return
	}

	fields := []any{
		"msg", "Expensive PromQL query", "query", q.q, "reasons", strings.Join(reasons, ","),
		"total_samples", cost.totalSamples, "peak_samples", cost.peakSamples,
		"returned_series", cost.returnedSeries, "returned_samples", cost.returnedSamples,
		"returned_sample_weight", cost.returnedSampleWeight, "sample_amplification", cost.sampleAmplification,
		"eval_seconds", cost.evalSeconds, "range_seconds", cost.rangeSeconds, "eval_range_ratio", cost.evalRangeRatio,
		"exec_seconds", q.stats.GetTimer(stats.ExecTotalTime).Duration(),
		"queue_seconds", q.stats.GetTimer(stats.ExecQueueTime).Duration(),
		"prepare_seconds", q.stats.GetTimer(stats.QueryPreparationTime).Duration(),
		"inner_eval_seconds", q.stats.GetTimer(stats.InnerEvalTime).Duration(),
	}

	if stmt, ok := q.stmt.(*parser.EvalStmt); ok {
		queryType := "instant"

		if stmt.Interval > 0 {
			queryType = "range"
		}

		fields = append(fields, "query_type", queryType, "start", formatDate(stmt.Start), "end", formatDate(stmt.End),
			"step_seconds", stmt.Interval.Seconds())

		// Evaluation replaces subqueries in the statement AST, so classify the original expression.
		if expr, err := parser.ParseExpr(q.q); err == nil {
			fields = append(fields, "patterns", expensiveQueryPatterns(expr))
		} else {
			fields = append(fields, "patterns", "", "pattern_error", err)
		}
	}

	if result.Err != nil {
		fields = append(fields, "error", result.Err)
	}

	if origin, ok := ctx.Value(QueryOrigin{}).(map[string]any); ok {
		for key, value := range origin {
			fields = append(fields, key, value)
		}
	}

	if span := trace.SpanContextFromContext(ctx); span.IsValid() {
		fields = append(fields, "traceID", span.TraceID(), "spanID", span.SpanID())
	}

	_ = level.Info(ng.logger).Log(fields...)
}

//
// queryCost
//

// queryCost summarizes completed engine work and returned points before [Query.Close] can recycle them.
type queryCost struct {
	totalSamples         int64
	peakSamples          int
	returnedSamples      int64
	returnedSampleWeight int64
	returnedSeries       int
	evalSeconds          float64
	rangeSeconds         float64
	sampleAmplification  float64
	evalRangeRatio       float64
}

func newQueryCost(q *query, result *Result) queryCost {
	cost := queryCost{
		totalSamples: q.sampleStats.TotalSamples,
		peakSamples:  q.sampleStats.PeakSamples,
		evalSeconds:  q.stats.GetTimer(stats.EvalTotalTime).Duration(),
	}

	if stmt, ok := q.stmt.(*parser.EvalStmt); ok {
		cost.rangeSeconds = stmt.End.Sub(stmt.Start).Seconds()
	}

	if cost.rangeSeconds > 0 {
		cost.evalRangeRatio = cost.evalSeconds / cost.rangeSeconds
	}

	if result.Err != nil {
		return cost
	}

	switch value := result.Value.(type) {
	case Vector:
		cost.returnedSeries = len(value)
		cost.returnedSamples = int64(len(value))
		cost.returnedSampleWeight = int64(value.TotalSamples())
	case Matrix:
		cost.returnedSeries = len(value)
		cost.returnedSampleWeight = int64(value.TotalSamples())

		for _, series := range value {
			cost.returnedSamples += int64(len(series.Floats) + len(series.Histograms))
		}
	case Scalar:
		cost.returnedSamples = 1
		cost.returnedSampleWeight = 1
	}

	cost.sampleAmplification = float64(cost.totalSamples) / float64(max(cost.returnedSampleWeight, 1))

	return cost
}

func (c queryCost) reasons(cfg *ExpensiveQueryLoggingConfig) []string {
	var reasons []string

	if cfg.HighSamples != nil && c.totalSamples >= int64(*cfg.HighSamples) {
		reasons = append(reasons, "high_samples")
	}

	if cfg.HighPeakSamples != nil && int64(c.peakSamples) >= int64(*cfg.HighPeakSamples) {
		reasons = append(reasons, "high_peak_samples")
	}

	if t := cfg.SampleAmplification; t != nil {
		if c.totalSamples >= int64(t.MinSamples) && c.sampleAmplification >= t.Ratio {
			reasons = append(reasons, "sample_amplification")
		}
	}

	if cfg.SlowEvaluation != nil && c.evalSeconds >= time.Duration(*cfg.SlowEvaluation).Seconds() {
		reasons = append(reasons, "slow_evaluation")
	}

	if t := cfg.HighRangeTimeRatio; t != nil {
		if c.rangeSeconds > 0 && c.evalSeconds >= time.Duration(t.MinEvaluation).Seconds() && c.evalRangeRatio >= t.Ratio {
			reasons = append(reasons, "high_range_time_ratio")
		}
	}

	return reasons
}

func expensiveQueryPatterns(expr parser.Expr) string {
	var aggregation, comparison, regex, rangeSelector, subquery, vectorMatching, labelReplace bool

	parser.Inspect(expr, func(node parser.Node, _ []parser.Node) error {
		switch node := node.(type) {
		case *parser.AggregateExpr:
			aggregation = true
		case *parser.BinaryExpr:
			comparison = comparison || node.Op.IsComparisonOperator() && !node.ReturnBool
			vectorMatching = vectorMatching ||
				node.LHS.Type() == parser.ValueTypeVector && node.RHS.Type() == parser.ValueTypeVector
		case *parser.VectorSelector:
			for _, matcher := range node.LabelMatchers {
				regex = regex || matcher.Type == labels.MatchRegexp || matcher.Type == labels.MatchNotRegexp
			}
		case *parser.MatrixSelector:
			rangeSelector = true
		case *parser.SubqueryExpr:
			subquery = true
		case *parser.Call:
			labelReplace = labelReplace || node.Func.Name == "label_replace"
		}

		return nil
	})

	var patterns []string

	for _, pattern := range []struct {
		name    string
		present bool
	}{
		{"aggregation", aggregation},
		{"comparison_filter", comparison},
		{"regex_matcher", regex},
		{"range_selector", rangeSelector},
		{"subquery", subquery},
		{"vector_matching", vectorMatching},
		{"label_replace", labelReplace},
	} {
		if pattern.present {
			patterns = append(patterns, pattern.name)
		}
	}

	return strings.Join(patterns, ",")
}
