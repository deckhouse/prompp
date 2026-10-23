package querier_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/storage"
)

// benchmarkOptimizationGate keeps dispatch indirect, as it is through [querier.Querier.IsPossibleToOptimize].
var benchmarkOptimizationGate = querier.DefaultIsPossibleToOptimize

// benchmarkOptimizationGateResult retains the predicate result during allocation measurements.
var benchmarkOptimizationGateResult bool

// DefaultOptimizationGateSuite checks scrape interval validation and the history, range, and step boundaries.
type DefaultOptimizationGateSuite struct {
	suite.Suite

	hints            *storage.SelectHints
	scrapeIntervalMS int64
	headMinTSMS      int64
}

func TestDefaultOptimizationGateSuite(t *testing.T) {
	suite.Run(t, new(DefaultOptimizationGateSuite))
}

func (s *DefaultOptimizationGateSuite) SetupTest() {
	s.scrapeIntervalMS = 1_000
	s.headMinTSMS = 10_000
	s.hints = &storage.SelectHints{
		Start: 15_000,
		End:   20_000,
		Step:  0,
		Range: 5_000,
		Func:  "max_over_time",
	}
}

func (s *DefaultOptimizationGateSuite) TestCrossSeriesWithNonPositiveScrapeInterval() {
	lss := cppbridge.NewQueryableLssStorage()
	for i := range querier.DefaultCountOfSeriesToOptimize + 3 {
		lss.FindOrEmplace(model.NewLabelSetBuilder().Set("job", "test").Set("instance", strconv.Itoa(i)).Build())
	}

	selector, status := lss.QuerySelector([]model.LabelMatcher{{
		Name: "job", Value: "test", MatcherType: model.MatcherTypeExactMatch,
	}})
	s.Require().Equal(cppbridge.LSSQueryStatusMatch, status)
	result := lss.CreateLabelSetSnapshot().Query(selector)
	s.T().Cleanup(result.Close)
	s.Require().Equal(querier.DefaultCountOfSeriesToOptimize+3, result.Len())
	results := []*cppbridge.LSSQueryResult{result}
	s.hints.Func = "sum"
	s.hints.Step = s.scrapeIntervalMS * 2
	s.hints.Range = 0
	s.hints.By = true

	for _, grouping := range [][]string{nil, {"job"}} {
		s.hints.Grouping = grouping
		s.True(querier.DefaultIsPossibleToOptimize(results, s.hints, s.scrapeIntervalMS, s.headMinTSMS))
		s.False(querier.DefaultIsPossibleToOptimize(results, s.hints, 0, s.headMinTSMS))
		s.False(querier.DefaultIsPossibleToOptimize(results, s.hints, -s.scrapeIntervalMS, s.headMinTSMS))
	}
}

func (s *DefaultOptimizationGateSuite) TestInstantAggregationAtHistoryAndRangeThresholds() {
	possible := querier.DefaultIsPossibleToOptimize(nil, s.hints, s.scrapeIntervalMS, s.headMinTSMS)

	s.True(possible)
}

func (s *DefaultOptimizationGateSuite) TestInstantAggregationWithInsufficientHistory() {
	s.hints.Start--
	possible := querier.DefaultIsPossibleToOptimize(nil, s.hints, s.scrapeIntervalMS, s.headMinTSMS)

	s.False(possible)
}

func (s *DefaultOptimizationGateSuite) TestInstantAggregationWithInsufficientRange() {
	s.hints.Range--
	possible := querier.DefaultIsPossibleToOptimize(nil, s.hints, s.scrapeIntervalMS, s.headMinTSMS)

	s.False(possible)
}

func (s *DefaultOptimizationGateSuite) TestInstantAggregationWithNonPositiveScrapeInterval() {
	s.False(querier.DefaultIsPossibleToOptimize(nil, s.hints, 0, s.headMinTSMS))
	s.False(querier.DefaultIsPossibleToOptimize(nil, s.hints, -s.scrapeIntervalMS, s.headMinTSMS))
}

func (s *DefaultOptimizationGateSuite) TestRangeAggregationAboveScrapeInterval() {
	s.hints.Step = s.scrapeIntervalMS * 4
	possible := querier.DefaultIsPossibleToOptimize(nil, s.hints, s.scrapeIntervalMS, s.headMinTSMS)

	s.True(possible)
}

func (s *DefaultOptimizationGateSuite) TestRangeAggregationAtScrapeInterval() {
	s.hints.Step = s.scrapeIntervalMS
	possible := querier.DefaultIsPossibleToOptimize(nil, s.hints, s.scrapeIntervalMS, s.headMinTSMS)

	s.False(possible)
}

func (s *DefaultOptimizationGateSuite) TestRangeAggregationBelowScrapeInterval() {
	s.hints.Step = s.scrapeIntervalMS - 1
	possible := querier.DefaultIsPossibleToOptimize(nil, s.hints, s.scrapeIntervalMS, s.headMinTSMS)

	s.False(possible)
}

func (s *DefaultOptimizationGateSuite) TestRangeAggregationWithNonPositiveScrapeInterval() {
	s.hints.Range = s.scrapeIntervalMS

	for _, step := range []int64{1, s.scrapeIntervalMS * 4} {
		s.hints.Step = step
		s.False(querier.DefaultIsPossibleToOptimize(nil, s.hints, 0, s.headMinTSMS))
		s.False(querier.DefaultIsPossibleToOptimize(nil, s.hints, -s.scrapeIntervalMS, s.headMinTSMS))
	}
}

func BenchmarkDefaultOptimizationGate(b *testing.B) {
	hints := &storage.SelectHints{Start: 15_000, End: 20_000, Range: 5_000, Func: "max_over_time"}
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		benchmarkOptimizationGateResult = benchmarkOptimizationGate(nil, hints, 1_000, 10_000)
	}
}
