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
			name:  "func not in allow-list",
			hints: &storage.SelectHints{Func: "min_over_time", Range: 120_000},
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
			name:  "irate with positive range",
			hints: &storage.SelectHints{Func: "irate", Range: 120_000},
			want:  true,
		},
		{
			name:  "idelta with positive range",
			hints: &storage.SelectHints{Func: "idelta", Range: 120_000},
			want:  true,
		},
		{
			name:  "rate with positive range",
			hints: &storage.SelectHints{Func: "rate", Range: 120_000},
			want:  true,
		},
		{
			name:  "increase with positive range",
			hints: &storage.SelectHints{Func: "increase", Range: 120_000},
			want:  true,
		},
		{
			name:  "delta with positive range",
			hints: &storage.SelectHints{Func: "delta", Range: 120_000},
			want:  true,
		},
		{
			name:  "deriv with positive range",
			hints: &storage.SelectHints{Func: "deriv", Range: 120_000},
			want:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, upsampler.NeedsUpsampling(tc.hints))
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
			name:  "func outside the allow-list",
			hints: &storage.SelectHints{Func: "min_over_time", Range: 120_000},
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
