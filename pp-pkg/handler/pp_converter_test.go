package handler_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/suite"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"

	"github.com/prometheus/prometheus/pp-pkg/handler"
	ppmodel "github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/remote/otlptranslator/prometheusremotewrite"
)

type PPConverterSuite struct {
	suite.Suite
}

func TestPPConverterSuite(t *testing.T) {
	suite.Run(t, new(PPConverterSuite))
}

func (s *PPConverterSuite) TestHappyPath() {
	payload := createExportRequest(5, 5, 5, 5, 5)

	converter := prometheusremotewrite.NewPrometheusConverter()
	_, err := converter.FromMetrics(
		context.Background(),
		payload.Metrics(),
		prometheusremotewrite.Settings{AddMetricSuffixes: true},
	)
	s.Require().NoError(err)

	expected := []ppmodel.TimeSeries{}

	b := ppmodel.NewLabelSetSimpleBuilder()
	for _, ts := range converter.TimeSeries() {
		ls := labelProtosToLabels(b, ts.Labels)

		for _, s := range ts.Samples {
			expected = append(expected, ppmodel.TimeSeries{
				LabelSet:  ls,
				Timestamp: uint64(s.Timestamp),
				Value:     s.Value,
			})
		}
	}
	slices.SortFunc(
		expected,
		func(a, b ppmodel.TimeSeries) int {
			return strings.Compare(a.LabelSet.String(), b.LabelSet.String())
		},
	)

	ppconverter := handler.NewPPConverter(log.NewNopLogger(), payload.Metrics().MetricCount())
	s.Require().NoError(ppconverter.FromMetrics(payload.Metrics()))

	actual := ppconverter.TimeSeries().TimeSeries()

	s.Require().Len(actual, len(expected))
	for i := range expected {
		s.Equal(expected[i].LabelSet.String(), actual[i].LabelSet.String())
		s.Equal(expected[i].Timestamp, actual[i].Timestamp)
		s.Equal(expected[i].Value, actual[i].Value)
	}
}

func createExportRequest(
	resourceAttributeCount,
	histogramCount,
	nonHistogramCount,
	labelsPerMetric,
	exemplarsPerSeries int,
) pmetricotlp.ExportRequest {
	request := pmetricotlp.NewExportRequest()

	rm := request.Metrics().ResourceMetrics().AppendEmpty()
	generateAttributes(rm.Resource().Attributes(), "resource", resourceAttributeCount)

	metrics := rm.ScopeMetrics().AppendEmpty().Metrics()
	ts := pcommon.NewTimestampFromTime(time.Now())

	for i := 1; i <= histogramCount; i++ {
		m := metrics.AppendEmpty()
		m.SetEmptyHistogram()
		m.SetName(fmt.Sprintf("histogram-%v", i))
		m.Histogram().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		h := m.Histogram().DataPoints().AppendEmpty()
		h.SetTimestamp(ts)

		// Set 50 samples, 10 each with values 0.5, 1, 2, 4, and 8
		h.SetCount(50)
		h.SetSum(155)
		h.BucketCounts().FromRaw([]uint64{10, 10, 10, 10, 10, 0})
		// Bucket boundaries include the upper limit (ie. each sample is on the upper limit of its bucket)
		h.ExplicitBounds().FromRaw([]float64{.5, 1, 2, 4, 8, 16})

		generateAttributes(h.Attributes(), "series", labelsPerMetric)
		generateExemplars(h.Exemplars(), exemplarsPerSeries, ts)
	}

	for i := 1; i <= nonHistogramCount; i++ {
		m := metrics.AppendEmpty()
		m.SetEmptySum()
		m.SetName(fmt.Sprintf("sum-%v", i))
		m.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
		point := m.Sum().DataPoints().AppendEmpty()
		point.SetTimestamp(ts)
		point.SetDoubleValue(1.23)
		generateAttributes(point.Attributes(), "series", labelsPerMetric)
		generateExemplars(point.Exemplars(), exemplarsPerSeries, ts)
	}

	for i := 1; i <= nonHistogramCount; i++ {
		m := metrics.AppendEmpty()
		m.SetEmptyGauge()
		m.SetName(fmt.Sprintf("gauge-%v", i))
		point := m.Gauge().DataPoints().AppendEmpty()
		point.SetTimestamp(ts)
		point.SetDoubleValue(1.23)
		generateAttributes(point.Attributes(), "series", labelsPerMetric)
		generateExemplars(point.Exemplars(), exemplarsPerSeries, ts)
	}

	return request
}

func generateAttributes(m pcommon.Map, prefix string, count int) {
	for i := 1; i <= count; i++ {
		m.PutStr(fmt.Sprintf("%v-name-%v", prefix, i), fmt.Sprintf("value-%v", i))
	}
}

func generateExemplars(exemplars pmetric.ExemplarSlice, count int, ts pcommon.Timestamp) {
	for i := 1; i <= count; i++ {
		e := exemplars.AppendEmpty()
		e.SetTimestamp(ts)
		e.SetDoubleValue(2.22)
		e.SetSpanID(pcommon.SpanID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08})
		e.SetTraceID(pcommon.TraceID{
			0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
		})
	}
}

func labelProtosToLabels(b *ppmodel.LabelSetSimpleBuilder, labelPairs []prompb.Label) ppmodel.LabelSet {
	b.Reset()
	for _, l := range labelPairs {
		b.Add(l.Name, l.Value)
	}

	return b.Build()
}
