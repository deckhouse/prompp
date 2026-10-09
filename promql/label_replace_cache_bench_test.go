package promql_test

// Benchmarks for the label_replace cache: cold (empty cache) versus warm
// (prefilled cache) over 2000 series, reporting ns/op and allocs/op.

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/util/teststorage"
)

// labelReplaceCacheConfig is one labels/regex cache size pair under test.
type labelReplaceCacheConfig struct {
	name  string
	size  int
	regex int
}

// BenchmarkLabelReplace measures label_replace over the generated target_info
// series with a cold (empty) and a warm (prefilled) labelset cache.
func BenchmarkLabelReplace(b *testing.B) {
	// Initialize test storage and generate test series data.
	testStorage := teststorage.New(b)
	defer testStorage.Close()

	// Generate 2000 target_info series whose instance label serves as the label_replace source.
	generateInfoFunctionTestSeries(b, testStorage, 2000, 2000, 3600)

	// Instance values are unique per series, so every series forms its own cache key.
	const queryExpr = `label_replace(target_info, 'dst', '$1', 'instance', '(.*)')`

	// Every generated series has a sample at this timestamp.
	ts := time.Unix(3600, 0)
	ctx := context.Background()

	baseOpts := promql.EngineOpts{
		Logger:               nil,
		Reg:                  nil,
		MaxSamples:           50000000,
		Timeout:              100 * time.Second,
		EnableAtModifier:     true,
		EnableNegativeOffset: true,
	}

	// Both caches sized to hold the whole query working set so that the warm
	// variant hits on every series; -1 disables a cache entirely.
	configs := []labelReplaceCacheConfig{
		{name: "both_on", size: 65536, regex: 128},
		{name: "both_off", size: -1, regex: -1},
		{name: "labels_only", size: 65536, regex: -1},
		{name: "regex_only", size: -1, regex: 128},
	}

	for _, cfg := range configs {
		opts := baseOpts
		opts.LabelReplaceCacheSize = cfg.size
		opts.LabelReplaceRegexCacheSize = cfg.regex

		// Cold: every iteration runs on a fresh engine with an empty cache.
		b.Run(cfg.name+"/cold", func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer() // Stop the timer to exclude engine and query setup.
				engine := promql.NewEngine(opts)
				qry, err := engine.NewInstantQuery(ctx, testStorage, nil, queryExpr, ts)
				require.NoError(b, err)

				b.StartTimer()
				result := qry.Exec(ctx)
				require.NoError(b, result.Err)
				qry.Close()
			}
		})

		// Warm: one full pre-run populates the cache before timing starts.
		// For both_off and regex_only this prefill is a no-op by construction:
		// nothing is stored or only the compiled regex is cached.
		b.Run(cfg.name+"/warm", func(b *testing.B) {
			engine := promql.NewEngine(opts)
			prefill, err := engine.NewInstantQuery(ctx, testStorage, nil, queryExpr, ts)
			require.NoError(b, err)
			require.NoError(b, prefill.Exec(ctx).Err)
			prefill.Close()

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer() // Stop the timer to exclude query setup.
				qry, err := engine.NewInstantQuery(ctx, testStorage, nil, queryExpr, ts)
				require.NoError(b, err)

				b.StartTimer()
				result := qry.Exec(ctx)
				require.NoError(b, result.Err)
				qry.Close()
			}
		})
	}
}
