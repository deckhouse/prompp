package upsampler_test

import (
	"context"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp-pkg/blocks/upsampler"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

//
// Iterator benchmarks
//

// BenchmarkIteratorSeekLinear measures the cost of frequent Seek operations on the linear path.
// This tests the worst-case scenario where Seek() rescans from the start each time.
func BenchmarkIteratorSeekLinear(b *testing.B) {
	// Create a large iterator with many samples
	const numSamples = 10000
	samples := make([]struct {
		t int64
		v float64
	}, numSamples)
	for i := 0; i < numSamples; i++ {
		samples[i].t = int64(i * 1000) // Every 1 second
		samples[i].v = float64(i)
	}

	baseIterator := newMockIterator(samples)
	baseSeries := &mockSeries{
		labels: labels.FromStrings("__name__", "bench_metric"),
		iteratorFunc: func(chunkenc.Iterator) chunkenc.Iterator {
			return baseIterator
		},
	}

	baseSeriesSet := &mockSeriesSet{
		atFunc: func() storage.Series {
			return baseSeries
		},
	}

	ss := upsampler.NewSeriesSet(baseSeriesSet, 60000)
	series := ss.At()

	// Create upsampler iterator
	it := series.Iterator(nil)

	for i := 0; b.Loop(); i++ {
		// Seek to different positions throughout the sample range
		// This tests the cost of linear rescan in mockIterator.Seek()
		target := int64((i % (numSamples - 1)) * 1000)
		it.Seek(target)
		// Force evaluation by reading a sample
		it.At()
	}
}

// BenchmarkIteratorSeekNoWrap measures Seek performance when wrapping is disabled.
// This provides a baseline for comparison.
func BenchmarkIteratorSeekNoWrap(b *testing.B) {
	const numSamples = 10000
	samples := make([]struct {
		t int64
		v float64
	}, numSamples)
	for i := range numSamples {
		samples[i].t = int64(i * 1000)
		samples[i].v = float64(i)
	}

	baseIterator := newMockIterator(samples)

	for i := 0; b.Loop(); i++ {
		// Direct seek on base iterator (no wrapping)
		target := int64((i % (numSamples - 1)) * 1000)
		baseIterator.Seek(target)
		baseIterator.At()
	}
}

//
// SeriesSet benchmarks
//

// BenchmarkSeriesSetIterationWrapped measures iteration when upsampling is active.
// This tests the wrapped path where SeriesSet wraps each series.
func BenchmarkSeriesSetIterationWrapped(b *testing.B) {
	const numSeries = 1000

	// Pre-allocate series list
	seriesList := make([]*mockSeries, numSeries)
	for i := 0; i < numSeries; i++ {
		idx := i
		seriesList[i] = &mockSeries{
			labels: labels.FromStrings("__name__", "metric", "index", string(rune(idx))),
			iteratorFunc: func(chunkenc.Iterator) chunkenc.Iterator {
				return newMockIterator([]struct {
					t int64
					v float64
				}{
					{100, 1.0},
					{200, 2.0},
				})
			},
		}
	}

	for b.Loop() {
		// Create base series set that iterates through all series
		seriesIndex := 0
		baseSS := &mockSeriesSet{
			nextFunc: func() bool {
				defer func() { seriesIndex++ }()
				return seriesIndex < numSeries
			},
			atFunc: func() storage.Series {
				if seriesIndex < numSeries {
					return seriesList[seriesIndex]
				}
				return nil
			},
		}

		ss := upsampler.NewSeriesSet(baseSS, 60000)

		// Iterate through all series
		count := 0
		for ss.Next() {
			_ = ss.At()
			count++
		}
	}
}

// BenchmarkSeriesSetIterationNoWrap measures iteration when wrapping is not triggered.
// This is the baseline: no upsampling overhead.
func BenchmarkSeriesSetIterationNoWrap(b *testing.B) {
	const numSeries = 1000

	seriesList := make([]*mockSeries, numSeries)
	for i := 0; i < numSeries; i++ {
		seriesList[i] = &mockSeries{
			labels: labels.FromStrings("__name__", "metric", "index", string(rune(i))),
		}
	}

	for b.Loop() {
		seriesIndex := 0
		baseSS := &mockSeriesSet{
			nextFunc: func() bool {
				defer func() { seriesIndex++ }()
				return seriesIndex < numSeries
			},
			atFunc: func() storage.Series {
				if seriesIndex < numSeries {
					return seriesList[seriesIndex]
				}
				return nil
			},
		}

		// Iterate without wrapping (baseline)
		count := 0
		for baseSS.Next() {
			_ = baseSS.At()
			count++
		}
	}
}

//
// Querier benchmarks
//

// BenchmarkQuerierSelectWrapped measures Select performance when upsampling is triggered.
// This tests the wrapped path where Querier wraps the SeriesSet.
func BenchmarkQuerierSelectWrapped(b *testing.B) {
	baseQuerier := &mockQuerier{
		selectFunc: func(context.Context, bool, *storage.SelectHints, ...*labels.Matcher) storage.SeriesSet {
			// Return a base series set with some series
			seriesIndex := 0
			return &mockSeriesSet{
				nextFunc: func() bool {
					seriesIndex++
					return seriesIndex <= 100
				},
				atFunc: func() storage.Series {
					return &mockSeries{
						labels: labels.FromStrings("__name__", "metric"),
					}
				},
			}
		},
	}

	ctx := b.Context()
	hints := &storage.SelectHints{Func: "rate", Range: 120000}

	for b.Loop() {
		q := upsampler.NewQuerier(baseQuerier, 300000)
		ss := q.Select(ctx, false, hints)

		// Consume the series set
		count := 0
		for ss.Next() {
			_ = ss.At()
			count++
		}
		_ = q.Close()
	}
}

// BenchmarkQuerierSelectNoWrap measures Select performance when wrapping is not triggered.
// This is the baseline: no upsampling overhead.
func BenchmarkQuerierSelectNoWrap(b *testing.B) {
	baseQuerier := &mockQuerier{
		selectFunc: func(context.Context, bool, *storage.SelectHints, ...*labels.Matcher) storage.SeriesSet {
			seriesIndex := 0
			return &mockSeriesSet{
				nextFunc: func() bool {
					seriesIndex++
					return seriesIndex <= 100
				},
				atFunc: func() storage.Series {
					return &mockSeries{
						labels: labels.FromStrings("__name__", "metric"),
					}
				},
			}
		},
	}

	ctx := b.Context()
	// Use a hint that won't trigger wrapping (irate is not in allow-list)
	hints := &storage.SelectHints{Func: "irate", Range: 120000}

	for b.Loop() {
		q := upsampler.NewQuerier(baseQuerier, 300000)
		ss := q.Select(ctx, false, hints)

		// Consume the series set (should not be wrapped)
		count := 0
		for ss.Next() {
			_ = ss.At()
			count++
		}
		_ = q.Close()
	}
}
