package querier_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/storage"
)

// DefaultOptimizationGateSuite checks the default aggregation gate at its history, range, and step boundaries.
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

func (s *DefaultOptimizationGateSuite) TestInstantAggregationAtHistoryAndRangeThresholds() {
	possible := querier.DefaultIsPossibleToOptimize(nil, s.hints, s.scrapeIntervalMS, s.headMinTSMS)

	s.True(possible())
}

func (s *DefaultOptimizationGateSuite) TestInstantAggregationWithInsufficientHistory() {
	s.hints.Start--
	possible := querier.DefaultIsPossibleToOptimize(nil, s.hints, s.scrapeIntervalMS, s.headMinTSMS)

	s.False(possible())
}

func (s *DefaultOptimizationGateSuite) TestInstantAggregationWithInsufficientRange() {
	s.hints.Range--
	possible := querier.DefaultIsPossibleToOptimize(nil, s.hints, s.scrapeIntervalMS, s.headMinTSMS)

	s.False(possible())
}

func (s *DefaultOptimizationGateSuite) TestRangeAggregationAboveScrapeInterval() {
	s.hints.Step = s.scrapeIntervalMS * 4
	possible := querier.DefaultIsPossibleToOptimize(nil, s.hints, s.scrapeIntervalMS, s.headMinTSMS)

	s.True(possible())
}

func (s *DefaultOptimizationGateSuite) TestRangeAggregationAtScrapeInterval() {
	s.hints.Step = s.scrapeIntervalMS
	possible := querier.DefaultIsPossibleToOptimize(nil, s.hints, s.scrapeIntervalMS, s.headMinTSMS)

	s.False(possible())
}

func (s *DefaultOptimizationGateSuite) TestRangeAggregationBelowScrapeInterval() {
	s.hints.Step = s.scrapeIntervalMS - 1
	possible := querier.DefaultIsPossibleToOptimize(nil, s.hints, s.scrapeIntervalMS, s.headMinTSMS)

	s.False(possible())
}
