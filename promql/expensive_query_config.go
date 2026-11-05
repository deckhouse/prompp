package promql

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v2"
)

//
// ExpensiveQueryLoggingConfig
//

// ExpensiveQueryLoggingConfig selects cost diagnostics with inclusive, positive thresholds.
// Nil fields disable their reasons. The configuration must not be modified after passing it to [NewEngine].
type ExpensiveQueryLoggingConfig struct {
	// HighSamples is the minimum total sample weight processed across all evaluation steps.
	// Processing the same sample at multiple steps counts it multiple times.
	HighSamples *SampleCountThreshold `yaml:"high_samples,omitempty"`
	// HighPeakSamples is the minimum peak sample weight held during evaluation, not a byte count.
	HighPeakSamples *SampleCountThreshold `yaml:"high_peak_samples,omitempty"`
	// SampleAmplification sets the minimum work-to-result ratio and processed sample weight for successful queries.
	SampleAmplification *SampleAmplificationThreshold `yaml:"sample_amplification,omitempty"`
	// SlowEvaluation is the minimum evaluation duration, including storage preparation but excluding queue time.
	// YAML values use duration strings such as "1s" or "100ms".
	SlowEvaluation *model.Duration `yaml:"slow_evaluation,omitempty"`
	// HighRangeTimeRatio compares evaluation time to the requested end-start range after a minimum evaluation duration.
	// Instant queries and ranges with equal start and end times do not trigger this reason.
	HighRangeTimeRatio *RangeTimeRatioThreshold `yaml:"high_range_time_ratio,omitempty"`
}

// LoadExpensiveQueryLoggingConfig reads the startup YAML configuration at filename.
// A missing file or a configuration without any reasons returns nil, disabling diagnostics.
func LoadExpensiveQueryLoggingConfig(filename string) (*ExpensiveQueryLoggingConfig, error) {
	data, err := os.ReadFile(filepath.Clean(filename))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("read expensive query configuration %q: %w", filename, err)
	}

	var cfg ExpensiveQueryLoggingConfig
	if err := yaml.UnmarshalStrict(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse expensive query configuration %q: %w", filename, err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid expensive query configuration %q: %w", filename, err)
	}

	if !cfg.enabled() {
		return nil, nil
	}

	return &cfg, nil
}

// enabled reports whether at least one diagnostic reason is configured; a nil receiver disables diagnostics.
func (c *ExpensiveQueryLoggingConfig) enabled() bool {
	return c != nil && (c.HighSamples != nil || c.HighPeakSamples != nil || c.SampleAmplification != nil ||
		c.SlowEvaluation != nil || c.HighRangeTimeRatio != nil)
}

// validate checks that configured thresholds are positive, ratios are finite, and compound reasons have both fields.
func (c *ExpensiveQueryLoggingConfig) validate() error {
	if c.HighSamples != nil && *c.HighSamples <= 0 {
		return errors.New("high_samples must be positive")
	}

	if c.HighPeakSamples != nil && *c.HighPeakSamples <= 0 {
		return errors.New("high_peak_samples must be positive")
	}

	if t := c.SampleAmplification; t != nil {
		if t.MinSamples <= 0 || !positiveFiniteRatio(t.Ratio) {
			return errors.New("sample_amplification requires positive min_samples and a finite positive ratio")
		}
	}

	if c.SlowEvaluation != nil && *c.SlowEvaluation <= 0 {
		return errors.New("slow_evaluation must be positive")
	}

	if t := c.HighRangeTimeRatio; t != nil {
		if t.MinEvaluation <= 0 || !positiveFiniteRatio(t.Ratio) {
			return errors.New("high_range_time_ratio requires positive min_evaluation and a finite positive ratio")
		}
	}

	return nil
}

// positiveFiniteRatio reports whether ratio is greater than zero and is neither infinite nor NaN.
func positiveFiniteRatio(ratio float64) bool {
	return ratio > 0 && !math.IsInf(ratio, 0) && !math.IsNaN(ratio)
}

//
// SampleAmplificationThreshold
//

// SampleAmplificationThreshold requires both a minimum sample count and work-to-result ratio.
type SampleAmplificationThreshold struct {
	// MinSamples is the minimum total processed sample weight required to check Ratio.
	MinSamples SampleCountThreshold `yaml:"min_samples"`
	// Ratio is the minimum processed sample weight divided by max(returned sample weight, 1).
	Ratio float64 `yaml:"ratio"`
}

//
// RangeTimeRatioThreshold
//

// RangeTimeRatioThreshold requires both a minimum evaluation duration and evaluation-to-range ratio.
type RangeTimeRatioThreshold struct {
	// MinEvaluation is the minimum evaluation duration required to check Ratio, expressed as e.g. "100ms" in YAML.
	MinEvaluation model.Duration `yaml:"min_evaluation"`
	// Ratio is the minimum evaluation duration divided by the requested end-start range; 0.1 means 10 percent.
	Ratio float64 `yaml:"ratio"`
}

//
// SampleCountThreshold
//

// SampleCountThreshold is an integer engine sample weight, not a physical storage read count.
type SampleCountThreshold int64

// UnmarshalYAML rejects fractional or overflowing sample counts instead of truncating them.
func (t *SampleCountThreshold) UnmarshalYAML(unmarshal func(any) error) error {
	var value string
	if err := unmarshal(&value); err != nil {
		return err
	}

	count, err := strconv.ParseInt(value, 0, 64)
	if err != nil {
		return fmt.Errorf("sample count must be an integer: %w", err)
	}

	*t = SampleCountThreshold(count)

	return nil
}
