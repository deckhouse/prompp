package upsampler_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp-pkg/blocks/upsampler"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

//
// QuerierSuite
//

type QuerierSuite struct {
	suite.Suite
}

func TestQuerierSuite(t *testing.T) {
	suite.Run(t, new(QuerierSuite))
}

// TestQuerierShouldWrap tests the shouldWrap decision logic.
func (s *QuerierSuite) TestQuerierShouldWrap() {
	testCases := []struct {
		name         string
		resolutionMS int64
		hints        *storage.SelectHints
		expectWrap   bool
	}{
		{
			name:         "nil hints",
			resolutionMS: 300_000,
			hints:        nil,
			expectWrap:   false,
		},
		{
			name:         "zero range",
			resolutionMS: 300_000,
			hints:        &storage.SelectHints{Func: "rate", Range: 0},
			expectWrap:   false,
		},
		{
			name:         "func not in allow-list",
			resolutionMS: 300_000,
			hints:        &storage.SelectHints{Func: "changes", Range: 120_000},
			expectWrap:   false,
		},
		{
			name:         "resolution*2 < range: no wrap (optimization)",
			resolutionMS: 50_000,
			hints:        &storage.SelectHints{Func: "rate", Range: 120_000},
			expectWrap:   false,
		},
		{
			name:         "resolution*2 == range: wrap",
			resolutionMS: 60_000,
			hints:        &storage.SelectHints{Func: "rate", Range: 120_000},
			expectWrap:   true,
		},
		{
			name:         "resolution > range: wrap",
			resolutionMS: 300_000,
			hints:        &storage.SelectHints{Func: "irate", Range: 120_000},
			expectWrap:   true,
		},
		{
			name:         "resolutionMS = 0: no wrap",
			resolutionMS: 0,
			hints:        &storage.SelectHints{Func: "rate", Range: 120_000},
			expectWrap:   false,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			q := upsampler.NewQuerier(&mockQuerier{}, tc.resolutionMS)
			ss := q.Select(s.T().Context(), false, tc.hints)

			// Check if the result is wrapped by type.
			_, isWrapped := ss.(*upsampler.SeriesSet)
			s.Require().Equal(tc.expectWrap, isWrapped, "expected wrap=%v, got type=%T", tc.expectWrap, ss)
		})
	}
}

// TestQuerierSelectExtendsHintsStart tests that the wrapped Select shifts the left
// border back by one resolution without mutating the caller's hints.
func (s *QuerierSuite) TestQuerierSelectExtendsHintsStart() {
	const resolutionMS = int64(300_000)

	hints := &storage.SelectHints{Func: "rate", Range: 120_000, Start: 1_000_000, End: 2_000_000}

	var passed *storage.SelectHints
	baseQuerier := &mockQuerier{
		selectFunc: func(
			_ context.Context,
			_ bool,
			h *storage.SelectHints,
			_ ...*labels.Matcher,
		) storage.SeriesSet {
			passed = h
			return &mockSeriesSet{}
		},
	}

	q := upsampler.NewQuerier(baseQuerier, resolutionMS)
	q.Select(s.T().Context(), false, hints)

	s.Require().NotNil(passed)
	s.Equal(hints.Start-resolutionMS, passed.Start)
	s.Equal(hints.End, passed.End)
	s.Equal(hints.Range, passed.Range)
	s.Equal(int64(1_000_000), hints.Start, "caller hints must not be mutated")
}

// TestQuerierSelectTakesFuncKindFromHints tests that the shape of a synthetic sample is
// chosen by hints.Func: a counter function holds a drop but interpolates a rise, a gauge
// function interpolates both, and an _over_time function holds the last known value either way.
func (s *QuerierSuite) TestQuerierSelectTakesFuncKindFromHints() {
	const resolutionMS = int64(120_000)

	testCases := []struct {
		name           string
		function       string
		secondSample   float64
		expectedSecond float64
	}{
		{name: "counter function holds a drop", function: "rate", secondSample: 20.0, expectedSecond: 100.0},
		{name: "counter function interpolates a rise", function: "rate", secondSample: 300.0, expectedSecond: 150.0},
		{name: "gauge function interpolates a drop", function: "delta", secondSample: 20.0, expectedSecond: 80.0},
		{name: "over_time function holds a drop", function: "min_over_time", secondSample: 20.0, expectedSecond: 100.0},
		{name: "over_time function holds a rise", function: "max_over_time", secondSample: 300.0, expectedSecond: 100.0},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			base := newMockIterator([]struct {
				t int64
				v float64
			}{
				{t: 60_000, v: 100.0},
				{t: 300_000, v: tc.secondSample},
			})
			baseQuerier := &mockQuerier{
				selectFunc: func(
					_ context.Context,
					_ bool,
					_ *storage.SelectHints,
					_ ...*labels.Matcher,
				) storage.SeriesSet {
					return newSingleSeriesSet(base)
				},
			}

			q := upsampler.NewQuerier(baseQuerier, resolutionMS)
			hints := &storage.SelectHints{Func: tc.function, Range: 120_000, Start: 0, End: 300_000}
			ss := q.Select(s.T().Context(), false, hints)

			s.Require().True(ss.Next())
			it := ss.At().Iterator(nil)
			s.Require().Equal(chunkenc.ValFloat, it.Next())
			s.Require().Equal(chunkenc.ValFloat, it.Next())

			ts, v := it.At()
			s.Require().Equal(int64(120_000), ts)
			s.Require().InDelta(tc.expectedSecond, v, 0.001)
		})
	}
}

// TestQuerierLabelValues tests delegation to base querier.
func (s *QuerierSuite) TestQuerierLabelValues() {
	expectedValues := []string{"value1", "value2"}

	baseQuerier := &mockLabelValuesQuerier{
		values: expectedValues,
	}

	q := upsampler.NewQuerier(baseQuerier, 5000)

	values, _, err := q.LabelValues(s.T().Context(), "metric_name", nil)

	s.Require().NoError(err)
	s.Equal(expectedValues, values)
}

// TestQuerierLabelNames tests delegation to base querier.
func (s *QuerierSuite) TestQuerierLabelNames() {
	expectedNames := []string{"__name__", "job"}

	baseQuerier := &mockLabelNamesQuerier{
		names: expectedNames,
	}

	q := upsampler.NewQuerier(baseQuerier, 5000)

	names, _, err := q.LabelNames(s.T().Context(), nil)

	s.Require().NoError(err)
	s.Equal(expectedNames, names)
}

// TestQuerierClose tests delegation to base querier.
func (s *QuerierSuite) TestQuerierClose() {
	baseQuerier := &closeableQuerier{}

	q := upsampler.NewQuerier(baseQuerier, 5000)

	err := q.Close()

	s.Require().NoError(err)
	s.True(baseQuerier.closed, "base querier Close() was not called")
}
