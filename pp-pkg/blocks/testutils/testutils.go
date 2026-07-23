package testutils

import (
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

const (
	defaultLabelName  = "labelName"
	defaultLabelValue = "labelValue"
)

//
// Test Helpers
//

// CreateBlock creates a block with given set of series and returns its dir.
func CreateBlock(tb testing.TB, dir string, series []storage.Series) string {
	blockDir, err := tsdb.CreateBlock(series, dir, 0, log.NewNopLogger())
	require.NoError(tb, err)

	return blockDir
}

// GenSeries generates series of float64 samples with a given number of labels and values.
func GenSeries(totalSeries, labelCount int, mint, maxt int64) []storage.Series {
	return genSeriesFromSampleGenerator(totalSeries, labelCount, mint, maxt, 1, func(ts int64) chunks.Sample {
		return SampleTest{TS: ts, V: rand.Float64()} //nolint:gosec // G404: no need for cryptographic strength here
	})
}

// genSeriesFromSampleGenerator generates series of samples with a given number of labels and values.
func genSeriesFromSampleGenerator(
	totalSeries, labelCount int,
	mint, maxt, step int64,
	generator func(ts int64) chunks.Sample,
) []storage.Series {
	if totalSeries == 0 || labelCount == 0 {
		return nil
	}

	series := make([]storage.Series, totalSeries)

	for i := 0; i < totalSeries; i++ {
		lbls := make(map[string]string, labelCount)
		lbls[defaultLabelName] = strconv.Itoa(i)
		for j := 1; len(lbls) < labelCount; j++ {
			lbls[defaultLabelName+strconv.Itoa(j)] = defaultLabelValue + strconv.Itoa(j)
		}
		samples := make([]chunks.Sample, 0, (maxt-mint)/step+1)
		for t := mint; t < maxt; t += step {
			samples = append(samples, generator(t))
		}
		series[i] = storage.NewListSeries(labels.FromMap(lbls), samples)
	}

	return series
}

//
// SampleTest
//

// SampleTest is a mock Sample.
type SampleTest struct {
	TS  int64
	V   float64
	HM  *histogram.Histogram
	FHM *histogram.FloatHistogram
}

// F returns the float value of the sample.
func (s SampleTest) F() float64 { return s.V }

// FH returns the float histogram value of the sample.
func (s SampleTest) FH() *histogram.FloatHistogram { return s.FHM }

// H returns the histogram value of the sample.
func (s SampleTest) H() *histogram.Histogram { return s.HM }

// T returns the time of the sample.
func (s SampleTest) T() int64 { return s.TS }

// Type returns the type of the sample.
func (s SampleTest) Type() chunkenc.ValueType {
	switch {
	case s.HM != nil:
		return chunkenc.ValHistogram
	case s.FHM != nil:
		return chunkenc.ValFloatHistogram
	default:
		return chunkenc.ValFloat
	}
}

//
// SeriesSamplesTest
//

// SeriesSamplesTest is a mock series samples.
type SeriesSamplesTest struct {
	Lset   map[string]string
	Chunks [][]SampleTest
}
