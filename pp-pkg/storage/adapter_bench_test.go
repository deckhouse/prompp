package storage

import (
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	pp_model "github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/storage"
)

// BenchmarkHeadQuerierSelectWithDownsampling measures Select() performance
// when WouldDownsample() == true with a rate() function.
// This represents the cost of upsampler wrapping active head queries.
func BenchmarkHeadQuerierSelectWithDownsampling(b *testing.B) {
	const (
		retentionMS    = int64(60 * 1000)  // 60s retention
		downsamplingMS = int64(30 * 1000)  // 30s downsampling
		queryRangeMS   = int64(120 * 1000) // 120s query range > retention → WouldDownsample=true
	)

	// Setup: create adapter with downsampling, ingest data
	suite := &BatchStorageSuite{}
	suite.SetT(&testing.T{})
	suite.SetupTest()
	defer suite.adapter.Close()

	adapterOpts := &AdapterOptions{
		RetentionMS:    retentionMS,
		DownsamplingMS: downsamplingMS,
	}
	benchAdapter := NewAdapter(
		suite.clock,
		suite.manager.Proxy(),
		suite.manager.Builder(),
		adapterOpts,
		suite.manager.MergeOutOfOrderChunks,
		nil,
	)
	defer benchAdapter.Close()

	ctx := b.Context()

	// Ingest time series data: 100 series × 50 samples each
	const (
		numSeries  = 100
		numSamples = 50
		stepMs     = 10_000 // 10s intervals
	)
	for seriesIdx := range numSeries {
		batch := &testTimeSeriesBatch{
			timeSeries: make([]pp_model.TimeSeries, numSamples),
		}
		for sampleIdx := range numSamples {
			tMs := int64(sampleIdx) * stepMs
			b := pp_model.NewLabelSetBuilder()
			b.Set("__name__", "http_requests_total")
			b.Set("instance", "server-"+string(rune(seriesIdx)))
			b.Set("job", "api")

			batch.timeSeries[sampleIdx] = pp_model.TimeSeries{
				LabelSet:  b.Build(),
				Timestamp: uint64(tMs),
				Value:     float64(seriesIdx*numSamples + sampleIdx),
			}
		}
		_, err := benchAdapter.AppendTimeSeries(ctx, batch, suite.state, false)
		if err != nil {
			b.Fatalf("failed to ingest data: %v", err)
		}
	}

	matcher := labels.MustNewMatcher(labels.MatchEqual, "__name__", "bench")

	// Query parameters: wide range triggering WouldDownsample()
	endMs := int64(numSamples-1) * stepMs
	startMs := endMs - queryRangeMS

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; b.Loop(); i++ {
		q, err := benchAdapter.Querier(startMs, endMs)
		if err != nil {
			b.Fatalf("failed to create querier: %v", err)
		}

		// Execute Select with rate() function (allow-list for upsampling)
		hints := &storage.SelectHints{
			Func:  "rate",
			Range: 60_000,
			Start: startMs,
			End:   endMs,
		}
		ss := q.Select(ctx, false, hints, matcher)

		// Consume the series set to measure full cost
		count := 0
		for ss.Next() {
			series := ss.At()
			_ = series
			count++
		}
		if err := ss.Err(); err != nil {
			b.Fatalf("select failed: %v", err)
		}
		_ = q.Close()
	}
}

// BenchmarkHeadQuerierSelectWithoutDownsampling measures Select() performance
// when WouldDownsample() == false (baseline: no upsampler overhead).
// Query range < retention → no downsampling applied, no wrapper created.
func BenchmarkHeadQuerierSelectWithoutDownsampling(b *testing.B) {
	const (
		longRetentionMS = int64(24 * 60 * 60 * 1000) // 24h retention
		downsamplingMS  = int64(30 * 1000)           // 30s downsampling (not applied)
		queryRangeMS    = int64(5 * 60 * 1000)       // 5m query range < retention → WouldDownsample=false
	)

	// Setup: create adapter with long retention, ingest same data
	suite := &BatchStorageSuite{}
	suite.SetT(&testing.T{})
	suite.SetupTest()
	defer suite.adapter.Close()

	adapterOpts := &AdapterOptions{
		RetentionMS:    longRetentionMS,
		DownsamplingMS: downsamplingMS,
	}
	benchAdapter := NewAdapter(
		suite.clock,
		suite.manager.Proxy(),
		suite.manager.Builder(),
		adapterOpts,
		suite.manager.MergeOutOfOrderChunks,
		nil,
	)
	defer benchAdapter.Close()

	ctx := b.Context()

	// Ingest same data as downsampling benchmark for fair comparison
	const (
		numSeries  = 100
		numSamples = 50
		stepMs     = 10_000
	)
	for seriesIdx := range numSeries {
		batch := &testTimeSeriesBatch{
			timeSeries: make([]pp_model.TimeSeries, numSamples),
		}
		for sampleIdx := range numSamples {
			tMs := int64(sampleIdx) * stepMs
			b := pp_model.NewLabelSetBuilder()
			b.Set("__name__", "http_requests_total")
			b.Set("instance", "server-"+string(rune(seriesIdx)))
			b.Set("job", "api")

			batch.timeSeries[sampleIdx] = pp_model.TimeSeries{
				LabelSet:  b.Build(),
				Timestamp: uint64(tMs),
				Value:     float64(seriesIdx*numSamples + sampleIdx),
			}
		}
		_, err := benchAdapter.AppendTimeSeries(ctx, batch, suite.state, false)
		if err != nil {
			b.Fatalf("failed to ingest data: %v", err)
		}
	}

	matcher := labels.MustNewMatcher(labels.MatchEqual, "__name__", "bench")

	// Query parameters: narrow range NOT triggering WouldDownsample()
	endMs := int64(numSamples-1) * stepMs
	startMs := int64(0) // Short range, well within retention

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; b.Loop(); i++ {
		q, err := benchAdapter.Querier(startMs, endMs)
		if err != nil {
			b.Fatalf("failed to create querier: %v", err)
		}

		// Same Select with rate() but WouldDownsample() == false
		// No upsampler wrapper is created, pure baseline
		hints := &storage.SelectHints{
			Func:  "rate",
			Range: 60_000,
			Start: startMs,
			End:   endMs,
		}
		ss := q.Select(ctx, false, hints, matcher)

		// Consume the series set
		count := 0
		for ss.Next() {
			series := ss.At()
			_ = series
			count++
		}
		if err := ss.Err(); err != nil {
			b.Fatalf("select failed: %v", err)
		}
		_ = q.Close()
	}
}
