package cppbridge

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/suite"
)

type CppMetricsSuite struct {
	suite.Suite
}

func TestCppMetricsSuite(t *testing.T) {
	suite.Run(t, new(CppMetricsSuite))
}

func (s *CppMetricsSuite) getMetrics() []*CppMetric {
	metrics := []*CppMetric(nil)
	for metric := range CppMetrics {
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
	s.Require().Len(metrics, 1)
	s.Equal("counter", reflect.ValueOf(metrics[0].descriptor).Elem().FieldByName("fqName").String())
	s.Equal(metrics[0].metric.Counter.GetValue(), float64(counterValue))
	s.Equal(Labels{Label{Name: "metrics_page", Value: "for_test"}}, metrics[0].Labels())
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

	s.Equal("counter2", reflect.ValueOf(metrics[0].descriptor).Elem().FieldByName("fqName").String())
	s.Equal(metrics[0].metric.Counter.GetValue(), float64(counterValue2))
	s.Equal(Labels{Label{Name: "metrics_page2", Value: "for_test"}}, metrics[0].Labels())

	s.Equal("counter1", reflect.ValueOf(metrics[1].descriptor).Elem().FieldByName("fqName").String())
	s.Equal(metrics[1].metric.Counter.GetValue(), float64(counterValue1))
	s.Equal(Labels{Label{Name: "metrics_page1", Value: "for_test"}}, metrics[1].Labels())
}
