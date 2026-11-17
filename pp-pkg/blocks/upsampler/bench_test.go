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

// BenchmarkIteratorSeekLinear measures full forward passes through a wrapped XOR chunk.
func BenchmarkIteratorSeekLinear(b *testing.B) {
	benchmarkIteratorSeek(b, true)
}

// BenchmarkIteratorSeekNoWrap measures the same forward passes without interpolation.
func BenchmarkIteratorSeekNoWrap(b *testing.B) {
	benchmarkIteratorSeek(b, false)
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
	for i := range numSeries {
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

		ss := upsampler.NewSeriesSet(baseSS, 60000, 60000, upsampler.CounterFunc)

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
	for i := range numSeries {
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
	ls := labels.FromStrings("__name__", "metric")
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
						labels: ls,
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
	ls := labels.FromStrings("__name__", "metric")
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
						labels: ls,
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

func benchmarkIteratorSeek(b *testing.B, wrapped bool) {
	b.Helper()

	const (
		// numSamples is the number of real samples traversed by each benchmark operation.
		numSamples = 10000
		// sampleIntervalMS is the spacing between real samples.
		sampleIntervalMS = 1000
		// samplesPerSeek is the forward distance between consecutive seek targets.
		samplesPerSeek = 10
	)

	c := chunkenc.NewXORChunk()
	appender, err := c.Appender()
	if err != nil {
		b.Fatal(err)
	}

	for i := range numSamples {
		appender.Append(int64(i*sampleIntervalMS), float64(i))
	}

	targets := make([]int64, 0, numSamples/samplesPerSeek)
	for i := samplesPerSeek; i < numSamples; i += samplesPerSeek {
		targets = append(targets, int64(i*sampleIntervalMS))
	}
	targets = append(targets, int64((numSamples-1)*sampleIntervalMS))

	base := c.Iterator(nil)
	var interpolator upsampler.Iterator
	b.ReportAllocs()

	for b.Loop() {
		b.StopTimer()
		base = c.Iterator(base)
		it := base
		if wrapped {
			interpolator.Reset(base, 30000, 120000, upsampler.CounterFunc)
			it = &interpolator
		}
		b.StartTimer()

		for _, target := range targets {
			if it.Seek(target) != chunkenc.ValFloat {
				b.Fatalf("Seek(%d) returned no float sample", target)
			}
			if timestamp, value := it.At(); timestamp != target || value != float64(target)/sampleIntervalMS {
				b.Fatalf("Seek(%d) returned (%d, %g)", target, timestamp, value)
			}
		}
	}

	b.ReportMetric(float64(len(targets)), "seeks/op")
}
