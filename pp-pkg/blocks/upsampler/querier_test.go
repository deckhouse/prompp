package upsampler_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp-pkg/blocks/upsampler"
	"github.com/prometheus/prometheus/storage"
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
