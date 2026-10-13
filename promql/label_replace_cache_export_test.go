package promql

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

// PrepareLabelReplaceCacheBenchmark grows the labels cache to the working set,
// fills it and verifies that another query is served entirely from the cache.
func PrepareLabelReplaceCacheBenchmark(b *testing.B, engine *Engine, seriesCount int, run func()) {
	b.Helper()

	c := engine.labelReplaceCache
	if c == nil || c.labelsCache == nil {
		run()

		return
	}

	require.LessOrEqual(b, seriesCount, c.labelsMax)
	c.labelsMu.Lock()
	c.resize(seriesCount, c.growResizes)
	c.labelsMu.Unlock()
	run()

	hits := c.labelsMetrics.hits
	misses := c.labelsMetrics.misses
	hitsBefore := testutil.ToFloat64(hits)
	missesBefore := testutil.ToFloat64(misses)
	run()

	require.Equal(b, float64(seriesCount), testutil.ToFloat64(hits)-hitsBefore)
	require.Equal(b, missesBefore, testutil.ToFloat64(misses))
}
