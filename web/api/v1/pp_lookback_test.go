package v1

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/util/teststorage"
)

const (
	// downsamplingLookbackOldSeconds anchors queries into the old data interval.
	downsamplingLookbackOldSeconds = int64(24 * 60 * 60)
	// downsamplingLookbackNowSeconds places fresh queries thirty days after the old interval.
	downsamplingLookbackNowSeconds = downsamplingLookbackOldSeconds + 30*24*60*60
	// downsamplingLookbackMetric has samples eight minutes before each query interval.
	downsamplingLookbackMetric = "lookback_metric"
	// downsamplingLookbackRecentMetric has a sample within the default five-minute lookback.
	downsamplingLookbackRecentMetric = "lookback_recent"
	// downsamplingLookbackLeftMetric has a sample on the expanded lookback's excluded boundary.
	downsamplingLookbackLeftMetric = "lookback_left"
	// downsamplingLookbackStaleMetric contains a stale marker after an otherwise eligible sample.
	downsamplingLookbackStaleMetric = "lookback_stale"
)

// downsamplingLookbackSuite exercises both query endpoints against a real engine and TSDB.
type downsamplingLookbackSuite struct {
	suite.Suite

	api     *API
	storage *teststorage.TestStorage
}

func TestDownsamplingLookbackSuite(t *testing.T) {
	suite.Run(t, new(downsamplingLookbackSuite))
}

func (s *downsamplingLookbackSuite) SetupSuite() {
	s.storage = teststorage.New(s.T())
	s.T().Cleanup(func() { s.Require().NoError(s.storage.Close()) })

	appender := s.storage.Appender(context.Background())
	for _, sample := range []struct {
		metric  string
		seconds int64
		value   float64
	}{
		{downsamplingLookbackLeftMetric, downsamplingLookbackOldSeconds - 20*60, 3},
		{downsamplingLookbackMetric, downsamplingLookbackOldSeconds - 8*60, 7},
		{downsamplingLookbackStaleMetric, downsamplingLookbackOldSeconds - 8*60, 7},
		{downsamplingLookbackRecentMetric, downsamplingLookbackOldSeconds - 4*60, 5},
		{downsamplingLookbackStaleMetric, downsamplingLookbackOldSeconds - 60, math.Float64frombits(value.StaleNaN)},
		{downsamplingLookbackMetric, downsamplingLookbackNowSeconds - 8*60, 11},
	} {
		_, err := appender.Append(
			0, labels.FromStrings(labels.MetricName, sample.metric), sample.seconds*1000, sample.value,
		)
		s.Require().NoError(err)
	}

	s.Require().NoError(appender.Commit())
}

func (s *downsamplingLookbackSuite) SetupTest() {
	s.api = &API{
		Queryable: s.storage,
		now:       func() time.Time { return time.Unix(downsamplingLookbackNowSeconds, 0) },
		minTimeBlocks: func() int64 {
			return (downsamplingLookbackOldSeconds + 15*24*60*60) * 1000
		},
		downsamplingLookbackDelta: 20 * time.Minute,
	}
	s.setEngine(5 * time.Minute)
}

func (s *downsamplingLookbackSuite) TestDefaultLookback() {
	for _, tc := range []struct {
		name      string
		defaultLB time.Duration
		metric    string
		want      float64
	}{
		{"default exceeds extension", 5 * time.Minute, downsamplingLookbackRecentMetric, 5},
		{"custom default", 11 * time.Minute, downsamplingLookbackMetric, 7},
	} {
		s.Run(tc.name, func() {
			// Arrange
			s.setEngine(tc.defaultLB)
			s.api.downsamplingLookbackDelta = 2 * time.Minute

			// Act / Assert
			s.assertQueries(tc.metric, downsamplingLookbackOldSeconds, []float64{tc.want})
		})
	}
}

func (s *downsamplingLookbackSuite) TestModifiers() {
	for _, tc := range []struct {
		name       string
		expression string
	}{
		{"offset", downsamplingLookbackMetric + " offset 30d"},
		{"timestamp", downsamplingLookbackMetric + " @ 86400"},
		{"timestamp and offset", downsamplingLookbackMetric + " @ 2678400 offset 30d"},
		{"start", downsamplingLookbackMetric + " @ start() offset 30d"},
		{"end", downsamplingLookbackMetric + " @ end() offset 30d"},
		{"subquery offset", "last_over_time(" + downsamplingLookbackMetric + "[1m:1m] offset 30d)"},
		{"subquery timestamp", "last_over_time(" + downsamplingLookbackMetric + "[1m:1m] @ 86400)"},
		{"nested subquery", "last_over_time(last_over_time(" + downsamplingLookbackMetric +
			"[1m:1m])[1m:1m] offset 30d)"},
	} {
		s.Run(tc.name, func() {
			// Arrange
			expression := tc.expression

			// Act / Assert
			s.assertQueries(expression, downsamplingLookbackNowSeconds, []float64{7})
		})
	}
}

func (s *downsamplingLookbackSuite) TestPolicyBoundaries() {
	for _, tc := range []struct {
		name     string
		boundary int64
		extra    time.Duration
		want     []float64
	}{
		{"equal boundary", (downsamplingLookbackOldSeconds-5*60)*1000 + 1, 20 * time.Minute, nil},
		{
			"one millisecond past boundary", (downsamplingLookbackOldSeconds-5*60)*1000 + 2,
			20 * time.Minute,
			[]float64{7},
		},
		{"base lookback crosses boundary", (downsamplingLookbackOldSeconds - 60) * 1000, 20 * time.Minute, []float64{7}},
		{"disabled", downsamplingLookbackNowSeconds * 1000, 0, nil},
		{"negative extension", downsamplingLookbackNowSeconds * 1000, -time.Minute, nil},
	} {
		s.Run(tc.name, func() {
			// Arrange
			s.api.minTimeBlocks = func() int64 { return tc.boundary }
			s.api.downsamplingLookbackDelta = tc.extra

			// Act / Assert
			s.assertQueries(downsamplingLookbackMetric, downsamplingLookbackOldSeconds, tc.want)
		})
	}
}

func (s *downsamplingLookbackSuite) TestRangeAndStaleness() {
	for _, tc := range []struct {
		name       string
		expression string
		want       []float64
	}{
		{"stale marker", downsamplingLookbackStaleMetric + " @ 86400", nil},
		{"open left boundary", downsamplingLookbackLeftMetric + " @ 86400", nil},
		{"matrix range unchanged", "last_over_time(" + downsamplingLookbackMetric + "[5m] @ 86400)", nil},
		{"mixed selectors", downsamplingLookbackMetric + " offset 30d + " + downsamplingLookbackMetric, []float64{18}},
	} {
		s.Run(tc.name, func() {
			// Arrange
			expression := tc.expression

			// Act / Assert
			s.assertQueries(expression, downsamplingLookbackNowSeconds, tc.want)
		})
	}
}

func (s *downsamplingLookbackSuite) TestReverseModifiers() {
	for _, expression := range []string{
		downsamplingLookbackMetric + " @ 2678400",
		downsamplingLookbackMetric + " offset -30d",
	} {
		s.Run(expression, func() {
			// Arrange
			evalSeconds := downsamplingLookbackOldSeconds

			// Act / Assert
			s.assertQueries(expression, evalSeconds, nil)
		})
	}
}

func (s *downsamplingLookbackSuite) TestUserLookback() {
	for _, tc := range []struct {
		name     string
		override string
		extra    time.Duration
		metric   string
		want     []float64
	}{
		{"zero uses default", "0", 2 * time.Minute, downsamplingLookbackRecentMetric, []float64{5}},
		{"negative uses default", "-60", 2 * time.Minute, downsamplingLookbackRecentMetric, []float64{5}},
		{"positive replaces default", "1m", 2 * time.Minute, downsamplingLookbackRecentMetric, nil},
		{"extension exceeds override", "2m", 5 * time.Minute, downsamplingLookbackRecentMetric, []float64{5}},
		{"positive exceeds extension", "10m", 2 * time.Minute, downsamplingLookbackMetric, []float64{7}},
		{"durations are not added", "3m", 2 * time.Minute, downsamplingLookbackRecentMetric, nil},
		{"disabled extension preserves override", "1m", 0, downsamplingLookbackRecentMetric, nil},
	} {
		s.Run(tc.name, func() {
			// Arrange
			s.api.downsamplingLookbackDelta = tc.extra

			// Act / Assert
			s.assertQueriesWithOverride(tc.metric, downsamplingLookbackOldSeconds, tc.override, tc.want)
		})
	}
}

func (s *downsamplingLookbackSuite) assertQueries(expression string, evalSeconds int64, want []float64) {
	s.T().Helper()
	s.assertQueriesWithOverride(expression, evalSeconds, "", want)
}

func (s *downsamplingLookbackSuite) assertQueriesWithOverride(
	expression string, evalSeconds int64, override string, want []float64,
) {
	s.T().Helper()

	for _, endpoint := range []struct {
		name  string
		query apiFunc
	}{
		{"instant", s.api.query},
		{"range", s.api.queryRange},
	} {
		s.Run(endpoint.name, func() {
			// Arrange
			params := url.Values{
				"query":          {expression},
				"time":           {strconv.FormatInt(evalSeconds, 10)},
				"start":          {strconv.FormatInt(evalSeconds, 10)},
				"end":            {strconv.FormatInt(evalSeconds+1, 10)},
				"step":           {"1"},
				"lookback_delta": {override},
			}
			req := httptest.NewRequest(http.MethodGet, "/?"+params.Encode(), nil)

			// Act
			result := endpoint.query(req)
			if result.finalizer != nil {
				defer result.finalizer()
			}

			// Assert
			s.Require().Nil(result.err)
			s.Require().IsType(&QueryData{}, result.data)

			switch samples := result.data.(*QueryData).Result.(type) {
			case promql.Vector:
				s.Require().Len(samples, len(want))

				for i, expected := range want {
					s.Equal(evalSeconds*1000, samples[i].T)
					s.Equal(expected, samples[i].F)
				}
			case promql.Matrix:
				s.Require().Len(samples, len(want))

				for i, expected := range want {
					s.Equal([]promql.FPoint{
						{T: evalSeconds * 1000, F: expected},
						{T: (evalSeconds + 1) * 1000, F: expected},
					}, samples[i].Floats)
				}
			default:
				s.FailNow("unexpected query result", "%T", samples)
			}
		})
	}
}

func (s *downsamplingLookbackSuite) setEngine(lookback time.Duration) {
	s.T().Helper()

	engine := promql.NewEngine(promql.EngineOpts{
		LookbackDelta:            lookback,
		MaxSamples:               10_000,
		Timeout:                  10 * time.Second,
		EnableAtModifier:         true,
		EnableNegativeOffset:     true,
		EnablePerStepStats:       true,
		NoStepSubqueryIntervalFn: func(int64) int64 { return time.Minute.Milliseconds() },
	})
	s.T().Cleanup(func() { s.Require().NoError(engine.Close()) })
	s.api.QueryEngine = engine
}
