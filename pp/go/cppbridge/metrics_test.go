package cppbridge

import (
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/suite"
)

type CppMetricsSuite struct {
	suite.Suite
}

func TestCppMetricsSuite(t *testing.T) {
	suite.Run(t, new(CppMetricsSuite))
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

func (s *CppMetricsSuite) getMetrics() []cppMetric {
	metrics := []cppMetric(nil)
	for metric := range CppMetrics {
		metrics = append(metrics, newCppMetric(metric))
	}

	return metrics
}

func (s *CppMetricsSuite) assertCounter(metric cppMetric, expectedLabels Labels, expectedValue float64) {
	s.Require().NotNil(metric.counter)
	s.Equal(expectedValue, *metric.counter.Value)
	s.Equal(expectedLabels, metric.labels)
}

func (s *CppMetricsSuite) TestNoMetricPages() {
	// Arrange

	// Act
	metrics := s.getMetrics()

	// Assert
	s.Len(metrics, 0)
}

func (s *CppMetricsSuite) TestOneMetricsPage() {
	// Arrange
	const counterValue = 123

	page := prometheusMetricsPageForTestCtor(Labels{Label{Name: "metrics_page", Value: "for_test"}}, "counter", counterValue)
	defer func() { prometheusMetricsPageForTestDetach(page) }()

	// Act
	metrics := s.getMetrics()

	// Assert
	s.Require().Len(metrics, 1)
	s.assertCounter(metrics[0], Labels{
		Label{Name: "__name__", Value: "counter"},
		Label{Name: "metrics_page", Value: "for_test"},
	}, float64(counterValue))
}

func (s *CppMetricsSuite) TestTwoMetricPages() {
	// Arrange
	const counterValue1 = 123
	const counterValue2 = 321

	page1 := prometheusMetricsPageForTestCtor(Labels{Label{Name: "metrics_page1", Value: "for_test"}}, "counter1", counterValue1)
	page2 := prometheusMetricsPageForTestCtor(Labels{Label{Name: "metrics_page2", Value: "for_test"}}, "counter2", counterValue2)
	defer func() {
		prometheusMetricsPageForTestDetach(page1)
		prometheusMetricsPageForTestDetach(page2)
	}()

	// Act
	metrics := s.getMetrics()

	// Assert
	s.Require().Len(metrics, 2)
	s.assertCounter(metrics[0], Labels{
		Label{Name: "__name__", Value: "counter2"},
		Label{Name: "metrics_page2", Value: "for_test"},
	}, float64(counterValue2))
	s.assertCounter(metrics[1], Labels{
		Label{Name: "__name__", Value: "counter1"},
		Label{Name: "metrics_page1", Value: "for_test"},
	}, float64(counterValue1))
}
