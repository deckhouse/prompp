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
			hints: &storage.SelectHints{Func: "irate", Range: 120_000},
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
			name:  "idelta not in allow-list",
			hints: &storage.SelectHints{Func: "idelta", Range: 120_000},
			want:  false,
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
