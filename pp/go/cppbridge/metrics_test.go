package cppbridge

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
)

type CppMetricsSuite struct {
	suite.Suite
}

func TestCppMetricsSuite(t *testing.T) {
	suite.Run(t, new(CppMetricsSuite))
}

// jemallocMetricDescPrefix matches the global jemalloc arena pool metrics that
// are registered in init() via prompp_metrics_register. They are unrelated to
// per-test metric pages and are filtered out of the suite's view of CppMetrics.
const jemallocMetricDescPrefix = `Desc{fqName: "prompp_common_jemalloc_`

func (s *CppMetricsSuite) getMetrics() []*CppMetric {
	metrics := []*CppMetric(nil)
	for metric := range CppMetrics {
		if strings.HasPrefix(metric.descriptor.String(), jemallocMetricDescPrefix) {
			continue
		}
		metrics = append(metrics, metric)
	}

	return metrics
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
	s.Require().Len(metrics, 2)
	s.Equal(`Desc{fqName: "counter", help: "", constLabels: {metrics_page="for_test"}, variableLabels: {}}`, metrics[0].descriptor.String())
	s.Equal(metrics[0].metric.Counter.GetValue(), float64(counterValue))

	s.Equal(`Desc{fqName: "counter", help: "", constLabels: {metrics_page="for_test"}, variableLabels: {}}`, metrics[0].descriptor.String())
	s.Equal(metrics[1].metric.Gauge.GetValue(), float64(counterValue))
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
	s.Require().Len(metrics, 4)

	s.Equal(`Desc{fqName: "counter2", help: "", constLabels: {metrics_page2="for_test"}, variableLabels: {}}`, metrics[0].descriptor.String())
	s.Equal(metrics[0].metric.Counter.GetValue(), float64(counterValue2))

	s.Equal(`Desc{fqName: "counter2", help: "", constLabels: {metrics_page2="for_test"}, variableLabels: {}}`, metrics[1].descriptor.String())
	s.Equal(metrics[1].metric.Gauge.GetValue(), float64(counterValue2))

	s.Equal(`Desc{fqName: "counter1", help: "", constLabels: {metrics_page1="for_test"}, variableLabels: {}}`, metrics[2].descriptor.String())
	s.Equal(metrics[2].metric.Counter.GetValue(), float64(counterValue1))

	s.Equal(`Desc{fqName: "counter1", help: "", constLabels: {metrics_page1="for_test"}, variableLabels: {}}`, metrics[3].descriptor.String())
	s.Equal(metrics[3].metric.Gauge.GetValue(), float64(counterValue1))
}
