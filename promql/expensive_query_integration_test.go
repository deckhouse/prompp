package promql_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/suite"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/util/teststorage"
)

// ExpensiveQueryLoggingSuite exercises diagnostics against a real TSDB and completed engine timers.
type ExpensiveQueryLoggingSuite struct {
	suite.Suite
	storage *teststorage.TestStorage
	logs    bytes.Buffer
}

func TestExpensiveQueryLoggingSuite(t *testing.T) {
	suite.Run(t, new(ExpensiveQueryLoggingSuite))
}

func (s *ExpensiveQueryLoggingSuite) SetupSuite() {
	s.storage = teststorage.New(s.T())
	appender := s.storage.Appender(context.Background())
	metric := labels.FromStrings("__name__", "metric", "job", "test", "some", "matcher")

	for i := int64(1); i <= 100_000; i++ {
		_, err := appender.Append(0, metric, i*1000, 1)
		s.Require().NoError(err)
	}

	s.Require().NoError(appender.Commit())
}

func (s *ExpensiveQueryLoggingSuite) SetupTest() {
	s.logs.Reset()
}

func (s *ExpensiveQueryLoggingSuite) TearDownSuite() {
	s.Require().NoError(s.storage.Close())
}

func (s *ExpensiveQueryLoggingSuite) TestCheapQueryIsNotLogged() {
	// Given
	engine := s.newEngine(true)
	defer engine.Close()
	query, err := engine.NewInstantQuery(context.Background(), s.storage, nil, "metric", time.Unix(100_000, 0))
	s.Require().NoError(err)
	defer query.Close()

	// When
	result := query.Exec(context.Background())

	// Then
	s.Require().NoError(result.Err)
	s.Empty(s.logs.String())
}

func (s *ExpensiveQueryLoggingSuite) TestConcurrentQueriesHaveIndependentStatistics() {
	// Given
	engine := s.newEngine(true)
	defer engine.Close()
	queries := make([]promql.Query, 4)

	for i := range queries {
		query, err := engine.NewInstantQuery(context.Background(), s.storage, nil,
			`max_over_time(metric[100001s])`, time.Unix(100_000, 0))
		s.Require().NoError(err)
		queries[i] = query
		defer query.Close()
	}

	// When
	var group errgroup.Group

	for _, query := range queries {
		group.Go(func() error { return query.Exec(context.Background()).Err })
	}

	// Then
	s.Require().NoError(group.Wait())
	decoder := json.NewDecoder(&s.logs)

	for range queries {
		var record map[string]interface{}
		s.Require().NoError(decoder.Decode(&record))
		s.Equal(float64(100_000), record["total_samples"])
		s.Equal(float64(1), record["returned_samples"])
	}

	var extra map[string]interface{}
	s.Require().ErrorIs(decoder.Decode(&extra), io.EOF)
}

func (s *ExpensiveQueryLoggingSuite) TestDisabledByDefault() {
	// Given
	engine := promql.NewEngine(promql.EngineOpts{
		Logger: log.NewJSONLogger(&s.logs), MaxSamples: 2_000_000, Timeout: time.Minute,
		LookbackDelta: 5 * time.Minute,
	})
	defer engine.Close()
	query, err := engine.NewInstantQuery(context.Background(), s.storage, nil,
		`max_over_time(metric[100001s])`, time.Unix(100_000, 0))
	s.Require().NoError(err)
	defer query.Close()

	// When
	result := query.Exec(context.Background())

	// Then
	s.Require().NoError(result.Err)
	s.Equal(int64(100_000), query.Stats().Samples.TotalSamples)
	s.Empty(s.logs.String())
}

func (s *ExpensiveQueryLoggingSuite) TestEmptyResultIsLogged() {
	// Given
	engine := s.newEngine(true)
	defer engine.Close()
	query, err := engine.NewInstantQuery(context.Background(), s.storage, nil,
		`max_over_time(metric[100001s]) > 1`, time.Unix(100_000, 0))
	s.Require().NoError(err)
	defer query.Close()

	// When
	result := query.Exec(context.Background())

	// Then
	s.Require().NoError(result.Err)
	s.Empty(result.Value)
	record := s.record()
	s.Equal(float64(0), record["returned_samples"])
	s.Equal(float64(100_000), record["sample_amplification"])
	s.Contains(record["reasons"], "sample_amplification")
}

func (s *ExpensiveQueryLoggingSuite) TestInstantQueryWithOriginAndQueryLogger() {
	// Given
	engine := s.newEngine(true)
	defer engine.Close()
	queryLogger := NewFakeQueryLogger()
	engine.SetQueryLogger(queryLogger)
	expression := `max by(job) (max_over_time(metric{some=~"match.*"}[100001s])) > 0`
	ctx := context.WithValue(context.Background(), promql.QueryOrigin{}, map[string]interface{}{"ruleGroup": "test"})
	span := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2},
	})
	ctx = trace.ContextWithSpanContext(ctx, span)
	query, err := engine.NewInstantQuery(ctx, s.storage, nil, expression, time.Unix(100_000, 0))
	s.Require().NoError(err)
	defer query.Close()

	// When
	result := query.Exec(ctx)

	// Then
	s.Require().NoError(result.Err)
	vector, err := result.Vector()
	s.Require().NoError(err)
	s.Require().Len(vector, 1)
	s.Equal(float64(1), vector[0].F)
	s.NotEmpty(queryLogger.logs)
	record := s.record()
	s.Equal("info", record["level"])
	s.Equal(expression, record["query"])
	s.Equal("instant", record["query_type"])
	s.Equal("test", record["ruleGroup"])
	s.Equal(span.TraceID().String(), record["traceID"])
	s.Equal(span.SpanID().String(), record["spanID"])
	s.Equal(float64(100_000), record["total_samples"])
	s.Equal(float64(1), record["returned_samples"])
	s.Contains(record["patterns"], "aggregation,comparison_filter,regex_matcher,range_selector")
	s.Greater(record["eval_seconds"], float64(0))
	s.GreaterOrEqual(record["exec_seconds"], record["eval_seconds"])
	s.Equal(float64(0), record["eval_range_ratio"])
	s.Nil(query.Stats().Samples.TotalSamplesPerStep)
}

func (s *ExpensiveQueryLoggingSuite) TestRangeQuery() {
	// Given
	engine := s.newEngine(true)
	defer engine.Close()
	query, err := engine.NewRangeQuery(context.Background(), s.storage, nil,
		`max_over_time(metric[100001s])`, time.Unix(99_999, 0), time.Unix(100_000, 0), time.Second)
	s.Require().NoError(err)
	defer query.Close()

	// When
	result := query.Exec(context.Background())

	// Then
	s.Require().NoError(result.Err)
	matrix, err := result.Matrix()
	s.Require().NoError(err)
	s.Require().Len(matrix, 1)
	s.Len(matrix[0].Floats, 2)
	record := s.record()
	s.Equal("range", record["query_type"])
	s.Equal(float64(1), record["range_seconds"])
	s.Equal(float64(1), record["step_seconds"])
	s.Equal(float64(2), record["returned_samples"])
	s.Equal(record["eval_seconds"], record["eval_range_ratio"])
	s.Contains(record["reasons"], "sample_amplification")
}

func (s *ExpensiveQueryLoggingSuite) TestSubqueryKeepsOriginalPatterns() {
	// Given
	engine := s.newEngine(true)
	defer engine.Close()
	query, err := engine.NewInstantQuery(context.Background(), s.storage, nil,
		`max_over_time(metric{some=~"match.*"}[100000s:1s])`, time.Unix(100_000, 0))
	s.Require().NoError(err)
	defer query.Close()

	// When
	result := query.Exec(context.Background())

	// Then
	s.Require().NoError(result.Err)
	s.Equal("regex_matcher,subquery", s.record()["patterns"])
}

func (s *ExpensiveQueryLoggingSuite) newEngine(enabled bool) *promql.Engine {
	return promql.NewEngine(promql.EngineOpts{
		Logger: log.NewJSONLogger(log.NewSyncWriter(&s.logs)), MaxSamples: 2_000_000, Timeout: time.Minute,
		EnableExpensiveQueryLogging: enabled,
		LookbackDelta:               5 * time.Minute,
	})
}

func (s *ExpensiveQueryLoggingSuite) record() map[string]interface{} {
	decoder := json.NewDecoder(&s.logs)
	var record map[string]interface{}
	s.Require().NoError(decoder.Decode(&record))
	var extra map[string]interface{}
	s.Require().ErrorIs(decoder.Decode(&extra), io.EOF)

	return record
}
