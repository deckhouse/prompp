package upsampler_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pp/go/storage/upsampler"
	"github.com/prometheus/prometheus/storage"
)

func TestNeedsUpsampling(t *testing.T) {
	testCases := []struct {
		name  string
		hints *storage.SelectHints
		want  bool
	}{
		{
			name:  "nil hints",
			hints: nil,
			want:  false,
		},
		{
			name:  "zero range",
			hints: &storage.SelectHints{Func: "rate", Range: 0},
			want:  false,
		},
		{
			name:  "negative range",
			hints: &storage.SelectHints{Func: "rate", Range: -1},
			want:  false,
		},
		{
			name:  "empty func",
			hints: &storage.SelectHints{Func: "", Range: 120_000},
			want:  false,
		},
		{
			name:  "changes not in allow-list",
			hints: &storage.SelectHints{Func: "changes", Range: 120_000},
			want:  false,
		},
		{
			name:  "resets not in allow-list",
			hints: &storage.SelectHints{Func: "resets", Range: 120_000},
			want:  false,
		},
		{
			name:  "predict_linear not in allow-list",
			hints: &storage.SelectHints{Func: "predict_linear", Range: 120_000},
			want:  false,
		},
		{
			name:  "last_over_step not in allow-list",
			hints: &storage.SelectHints{Func: "last_over_step", Range: 120_000},
			want:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, upsampler.NeedsUpsampling(tc.hints))
		})
	}
}

// TestNeedsUpsamplingAllowedFuncs pins the full allow-list: rate-like functions and the
// _over_time family, all of which read a whole range window and so tolerate synthetic points.
func TestNeedsUpsamplingAllowedFuncs(t *testing.T) {
	allowedFuncs := []string{
		"rate",
		"increase",
		"delta",
		"deriv",
		"irate",
		"idelta",
		"avg_over_time",
		"min_over_time",
		"max_over_time",
		"sum_over_time",
		"count_over_time",
		"quantile_over_time",
		"stddev_over_time",
		"stdvar_over_time",
		"mad_over_time",
		"last_over_time",
		"present_over_time",
	}

	for _, fn := range allowedFuncs {
		t.Run(fn, func(t *testing.T) {
			require.True(t, upsampler.NeedsUpsampling(&storage.SelectHints{Func: fn, Range: 120_000}))
		})
	}
}

func TestIsCounterFunc(t *testing.T) {
	testCases := []struct {
		name  string
		hints *storage.SelectHints
		want  bool
	}{
		{
			name:  "nil hints",
			hints: nil,
			want:  false,
		},
		{
			name:  "empty func",
			hints: &storage.SelectHints{Func: "", Range: 120_000},
			want:  false,
		},
		{
			name:  "rate is a counter func",
			hints: &storage.SelectHints{Func: "rate", Range: 120_000},
			want:  true,
		},
		{
			name:  "increase is a counter func",
			hints: &storage.SelectHints{Func: "increase", Range: 120_000},
			want:  true,
		},
		{
			name:  "irate is a counter func",
			hints: &storage.SelectHints{Func: "irate", Range: 120_000},
			want:  true,
		},
		{
			name:  "delta is a gauge func",
			hints: &storage.SelectHints{Func: "delta", Range: 120_000},
			want:  false,
		},
		{
			name:  "deriv is a gauge func",
			hints: &storage.SelectHints{Func: "deriv", Range: 120_000},
			want:  false,
		},
		{
			name:  "idelta is a gauge func",
			hints: &storage.SelectHints{Func: "idelta", Range: 120_000},
			want:  false,
		},
		{
			name:  "min_over_time is a gauge func",
			hints: &storage.SelectHints{Func: "min_over_time", Range: 120_000},
			want:  false,
		},
		{
			name:  "sum_over_time is a gauge func",
			hints: &storage.SelectHints{Func: "sum_over_time", Range: 120_000},
			want:  false,
		},
		{
			name:  "func outside the allow-list",
			hints: &storage.SelectHints{Func: "changes", Range: 120_000},
			want:  false,
		},
		{
			name:  "range is not consulted",
			hints: &storage.SelectHints{Func: "rate", Range: 0},
			want:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, upsampler.IsCounterFunc(tc.hints))
		})
	}
}

func TestIsOverTimeFunc(t *testing.T) {
	testCases := []struct {
		name  string
		hints *storage.SelectHints
		want  bool
	}{
		{
			name:  "nil hints",
			hints: nil,
			want:  false,
		},
		{
			name:  "empty func",
			hints: &storage.SelectHints{Func: "", Range: 120_000},
			want:  false,
		},
		{
			name:  "avg_over_time holds the last value",
			hints: &storage.SelectHints{Func: "avg_over_time", Range: 120_000},
			want:  true,
		},
		{
			name:  "count_over_time holds the last value",
			hints: &storage.SelectHints{Func: "count_over_time", Range: 120_000},
			want:  true,
		},
		{
			name:  "present_over_time holds the last value",
			hints: &storage.SelectHints{Func: "present_over_time", Range: 120_000},
			want:  true,
		},
		{
			name:  "rate is interpolated",
			hints: &storage.SelectHints{Func: "rate", Range: 120_000},
			want:  false,
		},
		{
			name:  "delta is interpolated",
			hints: &storage.SelectHints{Func: "delta", Range: 120_000},
			want:  false,
		},
		{
			name:  "last_over_step is not an _over_time func of the allow-list",
			hints: &storage.SelectHints{Func: "last_over_step", Range: 120_000},
			want:  false,
		},
		{
			name:  "range is not consulted",
			hints: &storage.SelectHints{Func: "sum_over_time", Range: 0},
			want:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, upsampler.IsOverTimeFunc(tc.hints))
		})
	}
}
