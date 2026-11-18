package querysplit

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/promql/promqltest"
)

// denseBounds returns bounds every 41s in [-1d, 60d). The spacing is not a multiple of the
// test steps, so the split points fall between steps and on steps.
func denseBounds() []int64 {
	const spacing = 41 * time.Second
	bounds := make([]int64, 0, int(61*24*time.Hour/spacing)+1)
	for t := -24 * time.Hour; t < 60*24*time.Hour; t += spacing {
		bounds = append(bounds, t.Milliseconds())
	}
	return bounds
}

func TestBuiltinEquivalence(t *testing.T) {
	// Arrange
	reg := prometheus.NewRegistry()
	inner := promqltest.NewTestEngine(t, false, 0, promqltest.DefaultMaxSamplesPerQuery)
	bounds := denseBounds()
	engine := New(inner, func() []int64 { return bounds }, Options{Registerer: reg})
	// No merge by the look-back window: split as many queries as possible.
	engine.defaults.MergeFactor = 0
	engine.defaults.MinBlocks = 0

	// Act
	promqltest.RunBuiltinTests(t, headerEngine{engine})

	// Assert
	require.Positive(t, testutil.ToFloat64(engine.metrics.splitTotal))
	t.Logf("split: %v, not split: %v",
		testutil.ToFloat64(engine.metrics.splitTotal), testutil.ToFloat64(engine.metrics.notSplitTotal))
}

func TestPrompPPFunctionsEquivalence(t *testing.T) {
	// Arrange
	storage := promqltest.LoadedStorage(t, `
load 1m
	x{i="1"} 0+1x600
	x{i="2"} 0+3x200 _x200 600+2x200
	x{i="3"} 0+2x600
	x{i="4"} 100-1x100 _x500
	x{i="5"} 5x600
	h{i="1"} {{schema:0 sum:1 count:1 buckets:[1]}}+{{schema:0 sum:2 count:2 buckets:[2]}}x600
`)
	t.Cleanup(func() { _ = storage.Close() })
	inner := promqltest.NewTestEngine(t, false, 0, 1_000_000)
	bounds := denseBounds()
	engine := New(inner, func() []int64 { return bounds }, Options{})
	engine.defaults.MergeFactor = 0
	engine.defaults.MinBlocks = 0
	start, end := time.Unix(0, 0), time.Unix(0, 0).Add(10*time.Hour)

	for _, qs := range []string{
		`op_top(2, rate(x[5m]))`,
		`op_top(2, true, rate(x[5m]))`,
		// OPTop builds the labels of ~other from a map without a sort. With __name__ the label
		// order changes between runs also without a split, so the case uses rate().
		`op_top(2, true, max, rate(x[5m]))`,
		`op_smoothie(x[10m])`,
		`op_zero_if_none(x[3m])`,
		`op_replace_nan(x[3m], 17)`,
		`op_defined(x[3m])`,
		`rate(h[10m])`,
		`histogram_quantile(0.9, rate(h[10m]))`,
		`x @ end() - x`,
	} {
		t.Run(qs, func(t *testing.T) {
			expected, err := inner.NewRangeQuery(context.Background(), storage, nil, qs, start, end, 30*time.Second)
			require.NoError(t, err)
			defer expected.Close()
			actual, err := engine.NewRangeQuery(withHeader(context.Background()), storage, nil, qs, start, end, 30*time.Second)
			require.NoError(t, err)
			defer actual.Close()
			_, isSplit := actual.(*splitQuery)

			// Act
			expectedRes := expected.Exec(context.Background())
			actualRes := actual.Exec(context.Background())

			// Assert
			require.True(t, isSplit)
			require.NoError(t, expectedRes.Err)
			require.NoError(t, actualRes.Err)
			require.Equal(t, expectedRes.Value.String(), actualRes.Value.String())
		})
	}
}
