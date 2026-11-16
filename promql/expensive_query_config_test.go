package promql

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
)

// ExpensiveQueryConfigSuite checks startup configuration and independently selected cost reasons.
type ExpensiveQueryConfigSuite struct {
	suite.Suite
	filename string
}

func TestExpensiveQueryConfigSuite(t *testing.T) {
	suite.Run(t, new(ExpensiveQueryConfigSuite))
}

func (s *ExpensiveQueryConfigSuite) SetupTest() {
	s.filename = filepath.Join(s.T().TempDir(), "expensive_queries.yaml")
}

func (s *ExpensiveQueryConfigSuite) TestConfiguredReasons() {
	for _, tc := range []struct {
		name    string
		yaml    string
		reasons []string
	}{
		{"samples only", "high_samples: 500000", []string{"high_samples"}},
		{"samples raised", "high_samples: 1000001", nil},
		{"peak only", "high_peak_samples: 500000", []string{"high_peak_samples"}},
		{"peak raised", "high_peak_samples: 1000001", nil},
		{
			"amplification only", "sample_amplification: {min_samples: 500000, ratio: 2000}",
			[]string{"sample_amplification"},
		},
		{"amplification ratio raised", "sample_amplification: {min_samples: 500000, ratio: 5000}", nil},
		{"amplification minimum raised", "sample_amplification: {min_samples: 1000001, ratio: 2000}", nil},
		{"evaluation only", "slow_evaluation: 500ms", []string{"slow_evaluation"}},
		{"evaluation raised", "slow_evaluation: 2s", nil},
		{
			"range ratio only", "high_range_time_ratio: {min_evaluation: 500ms, ratio: 0.05}",
			[]string{"high_range_time_ratio"},
		},
		{"range ratio raised", "high_range_time_ratio: {min_evaluation: 500ms, ratio: 0.2}", nil},
		{"range minimum raised", "high_range_time_ratio: {min_evaluation: 2s, ratio: 0.05}", nil},
	} {
		s.Run(tc.name, func() {
			// Given
			cfg := s.load(tc.yaml)
			cost := queryCost{
				totalSamples: 1_000_000, peakSamples: 1_000_000, sampleAmplification: 4000,
				evalSeconds: 1, rangeSeconds: 10, evalRangeRatio: 0.1,
			}

			// When
			reasons := cost.reasons(cfg)

			// Then
			s.Equal(tc.reasons, reasons)
		})
	}
}

func (s *ExpensiveQueryConfigSuite) TestEmptyConfigurationDisablesLogging() {
	for _, contents := range []string{"", "{}", "# No diagnostics configured\n", "high_samples: null"} {
		s.Run(contents, func() {
			// When
			cfg := s.load(contents)

			// Then
			s.Nil(cfg)
		})
	}
}

func (s *ExpensiveQueryConfigSuite) TestExampleConfiguration() {
	// Given
	cfg := s.load(`high_samples: 10000000
high_peak_samples: 1000000
sample_amplification:
  min_samples: 100000
  ratio: 1000
slow_evaluation: 1s
high_range_time_ratio:
  min_evaluation: 100ms
  ratio: 0.1
`)
	cost := queryCost{
		totalSamples: 10_000_000, peakSamples: 1_000_000, sampleAmplification: 1000,
		evalSeconds: 1, rangeSeconds: 10, evalRangeRatio: 0.1,
	}

	// When
	reasons := cost.reasons(cfg)

	// Then
	s.Equal([]string{
		"high_samples", "high_peak_samples", "sample_amplification", "slow_evaluation", "high_range_time_ratio",
	}, reasons)
}

func (s *ExpensiveQueryConfigSuite) TestInvalidConfiguration() {
	for _, contents := range []string{
		"unknown_reason: 1", "high_samples: 0", "high_samples: -1", "high_samples: 1.5",
		"high_peak_samples: -1", "high_peak_samples: 0", "high_peak_samples: 1.5",
		"high_samples: 9223372036854775808", "slow_evaluation: 0s", "slow_evaluation: invalid",
		"high_samples: [", "high_samples: 1\nhigh_samples: 2", "high_samples: many",
		"sample_amplification: {}", "sample_amplification: {min_samples: 1}",
		"sample_amplification: {ratio: 1}", "sample_amplification: {min_samples: -1, ratio: 1}",
		"sample_amplification: {min_samples: 1.5, ratio: 1}",
		"sample_amplification: {min_samples: 1, ratio: 0}",
		"sample_amplification: {min_samples: 1, ratio: .nan}",
		"sample_amplification: {min_samples: 1, ratio: .inf}",
		"sample_amplification: {min_samples: 1, ratio: 1, unknown: 1}",
		"high_range_time_ratio: {}", "high_range_time_ratio: {min_evaluation: 1s}",
		"high_range_time_ratio: {ratio: 1}", "high_range_time_ratio: {min_evaluation: 1s, ratio: -1}",
		"high_range_time_ratio: {min_evaluation: 0s, ratio: 1}",
		"high_range_time_ratio: {min_evaluation: 1s, ratio: .nan}",
		"high_range_time_ratio: {min_evaluation: 1s, ratio: .inf}",
	} {
		s.Run(contents, func() {
			// Given
			s.Require().NoError(os.WriteFile(s.filename, []byte(contents), 0o600))

			// When
			cfg, err := LoadExpensiveQueryLoggingConfig(s.filename)

			// Then
			s.Require().ErrorContains(err, s.filename)
			s.Nil(cfg)
		})
	}
}

func (s *ExpensiveQueryConfigSuite) TestMissingFileDisablesLogging() {
	// When
	cfg, err := LoadExpensiveQueryLoggingConfig(s.filename)

	// Then
	s.Require().NoError(err)
	s.Nil(cfg)
}

func (s *ExpensiveQueryConfigSuite) TestReadError() {
	// Given
	s.Require().NoError(os.Mkdir(s.filename, 0o700))

	// When
	cfg, err := LoadExpensiveQueryLoggingConfig(s.filename)

	// Then
	s.Require().ErrorContains(err, s.filename)
	s.Nil(cfg)
}

func (s *ExpensiveQueryConfigSuite) load(contents string) *ExpensiveQueryLoggingConfig {
	s.T().Helper()
	s.Require().NoError(os.WriteFile(s.filename, []byte(contents), 0o600))
	cfg, err := LoadExpensiveQueryLoggingConfig(s.filename)
	s.Require().NoError(err)

	return cfg
}
