package promql_test

// Engine-level metrics tests for the label_replace cache: hit accounting
// between identical queries, eviction accounting under a small configured
// capacity, and the "cache" label taking both of its values. Query behavior
// is covered in label_replace_cache_query_test.go.

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

// Metric families of the label_replace cache not yet declared in
// label_replace_cache_query_test.go, plus the "cache" label value of the
// compiled-regex cache.
const (
	labelReplaceCacheEvictionsFamily = "prometheus_engine_label_replace_cache_evictions_total"
	labelReplaceCacheEntriesFamily   = "prometheus_engine_label_replace_cache_entries"
	labelReplaceCacheRegexID         = "regex"
)

// TestLabelReplaceCacheMetrics covers the Engine-level label_replace cache
// metrics read from a per-test registry: hit accounting between identical
// queries, eviction accounting under a small configured capacity, and the
// "cache" label taking both of its values.
func TestLabelReplaceCacheMetrics(t *testing.T) {
	// Arrange: an engine with the default cache capacity and two series.
	env := newLabelReplaceTestEnv(t, 0)
	env.appendSamples(t,
		labels.FromStrings(labels.MetricName, "cache_metrics_hit", "instance", "a", "src", "value"),
		labels.FromStrings(labels.MetricName, "cache_metrics_hit", "instance", "b", "src", "value"),
	)
	qs := `label_replace(cache_metrics_hit, "dst", "value-$1", "src", "(.*)")`

	// Act: run the identical query twice.
	env.mustRun(t, qs)
	hitsFirst := env.cacheSum(t, labelReplaceCacheHitsFamily, labelReplaceCacheLabelsID)
	env.mustRun(t, qs)
	hitsSecond := env.cacheSum(t, labelReplaceCacheHitsFamily, labelReplaceCacheLabelsID)

	// Assert: the first run only misses, so the second one increases the hit
	// counter.
	require.Zero(t, hitsFirst, "the first query must not produce labelset cache hits.")
	require.Greater(t, hitsSecond, hitsFirst, "the second identical query must increase the hit counter.")

	// Assert: both "cache" label values are taken — "labels" by the
	// per-series labelset lookups and "regex" by getOrCompileRegex, which runs
	// on every query, so the regex children exist after the two runs above.
	for _, family := range []string{
		labelReplaceCacheHitsFamily,
		labelReplaceCacheMissesFamily,
		labelReplaceCacheEntriesFamily,
	} {
		for _, cacheID := range []string{labelReplaceCacheLabelsID, labelReplaceCacheRegexID} {
			require.Greater(t, env.cacheSum(t, family, cacheID), 0.0,
				"family %s must expose a child with cache=%s.", family, cacheID)
		}
	}

	// Arrange: a small capacity and more unique series than fit into it.
	const cacheCapacity = 4
	small := newLabelReplaceTestEnv(t, cacheCapacity)
	series := make([]labels.Labels, 0, 8)
	for i := range 8 {
		series = append(series, labels.FromStrings(
			labels.MetricName, "cache_metrics_evict",
			"instance", fmt.Sprintf("i%d", i),
			"src", "value",
		))
	}
	small.appendSamples(t, series...)
	evictQS := `label_replace(cache_metrics_evict, "dst", "value-$1", "src", "(.*)")`

	// Act: the first run overflows the labelset cache...
	small.mustRun(t, evictQS)
	evictionsFirst := small.cacheSum(t, labelReplaceCacheEvictionsFamily, labelReplaceCacheLabelsID)
	entriesFirst := small.cacheSum(t, labelReplaceCacheEntriesFamily, labelReplaceCacheLabelsID)

	// ...and the second one keeps adding entries that no longer fit.
	small.mustRun(t, evictQS)
	evictionsSecond := small.cacheSum(t, labelReplaceCacheEvictionsFamily, labelReplaceCacheLabelsID)
	entriesSecond := small.cacheSum(t, labelReplaceCacheEntriesFamily, labelReplaceCacheLabelsID)

	// Assert: overflowing the capacity evicts entries, keeps increasing the
	// eviction counter and holds the entries gauge at the configured capacity.
	require.Greater(t, evictionsFirst, 0.0, "exceeding the configured capacity must increase the eviction counter.")
	require.Greater(t, evictionsSecond, evictionsFirst, "further overflows must keep increasing the eviction counter.")
	require.Equal(t, float64(cacheCapacity), entriesFirst, "the entries gauge must equal Len() capped at the configured capacity.")
	require.Equal(t, float64(cacheCapacity), entriesSecond, "the entries gauge must stay at the configured capacity under continued overflow.")
}
