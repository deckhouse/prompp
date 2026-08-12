package upsampler_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

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
			hints:        &storage.SelectHints{Func: "irate", Range: 120_000},
			expectWrap:   false,
		},
		{
			name:         "resolution <= range: no wrap (optimization)",
			resolutionMS: 120_000,
			hints:        &storage.SelectHints{Func: "rate", Range: 120_000},
			expectWrap:   false,
		},
		{
			name:         "resolution > range: wrap",
			resolutionMS: 300_000,
			hints:        &storage.SelectHints{Func: "rate", Range: 120_000},
			expectWrap:   true,
		},
		{
			name:         "resolutionMS = 0 (head querier): always wrap if func matches",
			resolutionMS: 0,
			hints:        &storage.SelectHints{Func: "rate", Range: 120_000},
			expectWrap:   true,
		},
		{
			name:         "resolutionMS = 0 with non-matching func: no wrap",
			resolutionMS: 0,
			hints:        &storage.SelectHints{Func: "irate", Range: 120_000},
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
