package promql

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/util/stats"
)

// QueryCostSuite checks completed query costs without relying on wall-clock thresholds.
type QueryCostSuite struct {
	suite.Suite
	query *query
}

func TestQueryCostSuite(t *testing.T) {
	suite.Run(t, new(QueryCostSuite))
}

func (s *QueryCostSuite) SetupTest() {
	s.query = &query{
		stmt:        &parser.EvalStmt{Start: time.Unix(0, 0), End: time.Unix(0, 0)},
		stats:       stats.NewQueryTimers(),
		sampleStats: stats.NewQuerySamples(false),
	}
}

func (s *QueryCostSuite) TestFailedExecutionLogsReachedThresholds() {
	// Given
	var output bytes.Buffer
	engine := NewEngine(EngineOpts{
		Logger: log.NewJSONLogger(&output), Timeout: time.Minute, ExpensiveQueryLogging: expensiveQueryTestConfig(),
		LookbackDelta: 5 * time.Minute,
	})
	defer engine.Close()
	wantErr := errors.New("storage failed after processing samples")
	var q *query
	q = engine.NewTestQuery(func(context.Context) error {
		q.sampleStats.TotalSamples = 10_000_000
		q.sampleStats.PeakSamples = 1_000_000

		return wantErr
	}).(*query)

	// When
	result := q.Exec(context.Background())

	// Then
	s.Require().ErrorIs(result.Err, wantErr)
	var record map[string]interface{}
	s.Require().NoError(json.Unmarshal(output.Bytes(), &record))
	s.Equal(wantErr.Error(), record["error"])
	s.Contains(record["reasons"], "high_samples")
	s.Contains(record["reasons"], "high_peak_samples")
	s.NotContains(record["reasons"], "sample_amplification")
	s.Equal(float64(0), record["sample_amplification"])
}

func (s *QueryCostSuite) TestFailedResultHasNoAmplification() {
	// Given
	s.query.sampleStats.TotalSamples = 100_000

	// When
	cost := newQueryCost(s.query, &Result{Err: errors.New("storage failed")})

	// Then
	s.Zero(cost.sampleAmplification)
	s.Empty(cost.reasons(expensiveQueryTestConfig()))
}

func (s *QueryCostSuite) TestInstantAndZeroRangeHaveNoTimeRatio() {
	// Given
	timer := s.query.stats.GetTimer(stats.EvalTotalTime)
	timer.Start()
	timer.Stop()

	// When
	cost := newQueryCost(s.query, &Result{})

	// Then
	s.Positive(cost.evalSeconds)
	s.Zero(cost.rangeSeconds)
	s.Zero(cost.evalRangeRatio)
}

func (s *QueryCostSuite) TestPatterns() {
	for _, tc := range []struct {
		query    string
		patterns string
	}{
		{`max by(job) (metric{some=~"match.*"}) > 0`, "aggregation,comparison_filter,regex_matcher"},
		{`metric > bool 0`, ""},
		{`metric{some!~"other.*"}`, "regex_matcher"},
		{`rate(metric[5m])`, "range_selector"},
		{`max_over_time(metric[1h:1m])`, "subquery"},
		{`metric + on(job) group_left() other`, "vector_matching"},
		{`metric or other`, "vector_matching"},
		{`label_replace(metric, "dst", "$1", "src", "(.*)")`, "label_replace"},
	} {
		s.Run(tc.query, func() {
			// Given
			expr, err := parser.ParseExpr(tc.query)
			s.Require().NoError(err)

			// When
			patterns := expensiveQueryPatterns(expr)

			// Then
			s.Equal(tc.patterns, patterns)
		})
	}
}

func (s *QueryCostSuite) TestQueueTimeDoesNotCountAsEvaluation() {
	// Given
	timer := s.query.stats.GetTimer(stats.ExecQueueTime)
	timer.Start()
	timer.Stop()
	s.Require().Positive(timer.Duration())

	// When
	cost := newQueryCost(s.query, &Result{})

	// Then
	s.Zero(cost.evalSeconds)
	s.Empty(cost.reasons(expensiveQueryTestConfig()))
}

func (s *QueryCostSuite) TestRangeRatioUsesRequestedRange() {
	// Given
	s.query.stmt.(*parser.EvalStmt).End = time.Unix(2, 0)
	timer := s.query.stats.GetTimer(stats.EvalTotalTime)
	timer.Start()
	timer.Stop()
	s.Require().Positive(timer.Duration())

	// When
	cost := newQueryCost(s.query, &Result{})

	// Then
	s.Equal(float64(2), cost.rangeSeconds)
	s.Equal(timer.Duration()/2, cost.evalRangeRatio)
}

func (s *QueryCostSuite) TestResultSizes() {
	h := &histogram.FloatHistogram{}
	for _, tc := range []struct {
		name    string
		result  *Result
		series  int
		samples int64
		weight  int64
	}{
		{"empty", &Result{Value: Vector{}}, 0, 0, 0},
		{"scalar", &Result{Value: Scalar{V: 1}}, 0, 1, 1},
		{"string", &Result{Value: String{V: "hello"}}, 0, 0, 0},
		{"vector", &Result{Value: Vector{{F: 1}, {F: 2}}}, 2, 2, 2},
		{"histogram vector", &Result{Value: Vector{{H: h}}}, 1, 1, 1 + int64(h.Size()/16)},
		{"mixed matrix", &Result{Value: Matrix{
			{Floats: []FPoint{{F: 1}, {F: 2}}},
			{Histograms: []HPoint{{H: h}}},
		}}, 2, 3, 2 + int64(HPoint{H: h}.size())},
	} {
		s.Run(tc.name, func() {
			// Given
			s.query.sampleStats.TotalSamples = 100_000

			// When
			cost := newQueryCost(s.query, tc.result)

			// Then
			s.Equal(tc.series, cost.returnedSeries)
			s.Equal(tc.samples, cost.returnedSamples)
			s.Equal(tc.weight, cost.returnedSampleWeight)
			s.Equal(100_000/float64(max(tc.weight, 1)), cost.sampleAmplification)
		})
	}
}

func (s *QueryCostSuite) TestThresholds() {
	for _, tc := range []struct {
		name    string
		cost    queryCost
		reasons []string
	}{
		{name: "cheap"},
		{name: "below sample threshold", cost: queryCost{totalSamples: 9_999_999}},
		{name: "sample threshold", cost: queryCost{totalSamples: 10_000_000}, reasons: []string{"high_samples"}},
		{name: "below peak threshold", cost: queryCost{peakSamples: 999_999}},
		{name: "peak threshold", cost: queryCost{peakSamples: 1_000_000}, reasons: []string{"high_peak_samples"}},
		{name: "small amplified query", cost: queryCost{totalSamples: 99_999, sampleAmplification: 99_999}},
		{name: "below amplification ratio", cost: queryCost{totalSamples: 100_000, sampleAmplification: 999}},
		{
			name: "amplification threshold", cost: queryCost{totalSamples: 100_000, sampleAmplification: 1000},
			reasons: []string{"sample_amplification"},
		},
		{name: "below evaluation threshold", cost: queryCost{evalSeconds: 0.999}},
		{name: "evaluation threshold", cost: queryCost{evalSeconds: 1}, reasons: []string{"slow_evaluation"}},
		{name: "short range evaluation", cost: queryCost{rangeSeconds: 1, evalSeconds: 0.099, evalRangeRatio: 1}},
		{name: "below range ratio", cost: queryCost{rangeSeconds: 1, evalSeconds: 0.1, evalRangeRatio: 0.099}},
		{
			name: "range threshold", cost: queryCost{rangeSeconds: 1, evalSeconds: 0.1, evalRangeRatio: 0.1},
			reasons: []string{"high_range_time_ratio"},
		},
		{name: "all reasons", cost: queryCost{
			totalSamples: 10_000_000, peakSamples: 1_000_000,
			sampleAmplification: 1000, rangeSeconds: 10, evalSeconds: 1, evalRangeRatio: 0.1,
		}, reasons: []string{
			"high_samples", "high_peak_samples", "sample_amplification", "slow_evaluation", "high_range_time_ratio",
		}},
	} {
		s.Run(tc.name, func() {
			// When
			reasons := tc.cost.reasons(expensiveQueryTestConfig())

			// Then
			s.Equal(tc.reasons, reasons)
		})
	}
}

func expensiveQueryTestConfig() *ExpensiveQueryLoggingConfig {
	highSamples := SampleCountThreshold(10_000_000)
	highPeakSamples := SampleCountThreshold(1_000_000)
	slowEvaluation := model.Duration(time.Second)

	return &ExpensiveQueryLoggingConfig{
		HighSamples:     &highSamples,
		HighPeakSamples: &highPeakSamples,
		SampleAmplification: &SampleAmplificationThreshold{
			MinSamples: 100_000, Ratio: 1000,
		},
		SlowEvaluation: &slowEvaluation,
		HighRangeTimeRatio: &RangeTimeRatioThreshold{
			MinEvaluation: model.Duration(100 * time.Millisecond), Ratio: 0.1,
		},
	}
}
