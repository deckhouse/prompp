package cppbridge

import (
	"runtime"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/suite"
)

type MetricsSuite struct {
	suite.Suite
}

func TestMetricsSuite(t *testing.T) {
	suite.Run(t, new(MetricsSuite))
}

type cppMetric struct {
	labels  Labels
	counter *dto.Counter
}

func newCppMetric(metric *dto.Metric) cppMetric {
	m := cppMetric{labels: make(Labels, 0, cap(metric.Label)), counter: metric.Counter}
	for _, l := range metric.Label {
		m.labels = append(m.labels, Label{Name: *l.Name, Value: *l.Value})
	}

	return m
}

func (s *MetricsSuite) getMetrics() []cppMetric {
	metrics := []cppMetric(nil)
	for metric := range CppMetrics {
		metrics = append(metrics, newCppMetric(metric))
	}

	return metrics
}

func (s *MetricsSuite) TestEnumMetrics() {
	// Arrange
	const counterValue = 123

	page := prometheusMetricsPageForTestCtor(Labels{Label{Name: "metrics_page", Value: "for_test"}}, "counter", counterValue)
	defer func() { prometheusMetricsPageForTestDetach(page) }()

	// Act
	metrics := s.getMetrics()

	// Assert
	s.Require().Len(metrics, 1)
	s.Require().NotNil(metrics[0].counter)
	s.Equal(float64(counterValue), *metrics[0].counter.Value)
	s.Equal(Labels{
		Label{Name: "__name__", Value: "counter"},
		Label{Name: "metrics_page", Value: "for_test"},
	}, metrics[0].labels)

	runtime.KeepAlive(page)
}
