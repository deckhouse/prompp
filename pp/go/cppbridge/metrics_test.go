package cppbridge

import (
	"strings"
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

// jemallocMetricDescPrefix matches the global jemalloc arena pool metrics that
// are registered in init() via prompp_metrics_register. They are unrelated to
// per-test metric pages and are filtered out of the suite's view of CppMetrics.
const jemallocMetricDescPrefix = `Desc{fqName: "prompp_common_jemalloc_`

func (s *CppMetricsSuite) getMetrics() []*CppMetricCopy {
	metrics := []*CppMetricCopy(nil)
	for metric := range CppMetrics {
		if strings.HasPrefix(metric.meta.desc.String(), jemallocMetricDescPrefix) {
			continue
		}
		metrics = append(metrics, metric)
	}

	return metrics
}

func cacheContainsMeta(meta *CppMetricMeta) bool {
	cppMetricsMu.Lock()
	defer cppMetricsMu.Unlock()

	for _, m := range metaCache {
		if m == meta {
			return true
		}
	}

	return false
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
	s.Equal(`Desc{fqName: "counter", help: "", constLabels: {metrics_page="for_test"}, variableLabels: {}}`, metrics[0].meta.desc.String())
	s.Equal(cppMetricCounter, metrics[0].meta.kind)
	s.Equal(float64(counterValue), metrics[0].value)

	s.Equal(`Desc{fqName: "counter", help: "", constLabels: {metrics_page="for_test"}, variableLabels: {}}`, metrics[1].meta.desc.String())
	s.Equal(cppMetricGauge, metrics[1].meta.kind)
	s.Equal(float64(counterValue), metrics[1].value)
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

	s.Equal(`Desc{fqName: "counter2", help: "", constLabels: {metrics_page2="for_test"}, variableLabels: {}}`, metrics[0].meta.desc.String())
	s.Equal(float64(counterValue2), metrics[0].value)

	s.Equal(`Desc{fqName: "counter2", help: "", constLabels: {metrics_page2="for_test"}, variableLabels: {}}`, metrics[1].meta.desc.String())
	s.Equal(float64(counterValue2), metrics[1].value)

	s.Equal(`Desc{fqName: "counter1", help: "", constLabels: {metrics_page1="for_test"}, variableLabels: {}}`, metrics[2].meta.desc.String())
	s.Equal(float64(counterValue1), metrics[2].value)

	s.Equal(`Desc{fqName: "counter1", help: "", constLabels: {metrics_page1="for_test"}, variableLabels: {}}`, metrics[3].meta.desc.String())
	s.Equal(float64(counterValue1), metrics[3].value)
}

// TestCopyOutlivesFreedPage is the use-after-free regression: copies returned by CppMetrics must remain fully readable
// even after the underlying C++ metrics page has been detached and physically freed by remove_unused_pages().
func (s *CppMetricsSuite) TestCopyOutlivesFreedPage() {
	// Arrange
	const counterValue = 777

	page := prometheusMetricsPageForTestCtor(Labels{Label{Name: "metrics_page", Value: "for_test"}}, "counter", counterValue)

	metrics := s.getMetrics()
	s.Require().Len(metrics, 2)

	// Act: detach the page and trigger the next iterator ctor, which runs remove_unused_pages() -> delete page.
	prometheusMetricsPageForTestDetach(page)
	s.getMetrics()

	// Assert: the earlier copies are Go-owned and still valid.
	s.Equal(`Desc{fqName: "counter", help: "", constLabels: {metrics_page="for_test"}, variableLabels: {}}`, metrics[0].meta.desc.String())
	s.Equal(float64(counterValue), metrics[0].value)
	s.Equal(float64(counterValue), metrics[1].value)

	out := &dto.Metric{}
	s.Require().NoError(metrics[0].Write(out))
	s.Require().Len(out.Label, 1)
	s.Equal("metrics_page", out.Label[0].GetName())
	s.Equal("for_test", out.Label[0].GetValue())
	s.Require().NotNil(out.Counter)
	s.Equal(float64(counterValue), out.Counter.GetValue())
}

// TestCacheReusesMetaAcrossScrapes verifies that the immutable meta (descriptor + labels) is cached and reused across
// scrapes for the same descriptor identity, while the value is copied fresh each time.
func (s *CppMetricsSuite) TestCacheReusesMetaAcrossScrapes() {
	// Arrange
	page := prometheusMetricsPageForTestCtor(Labels{Label{Name: "metrics_page", Value: "reuse"}}, "counter_reuse", 1)
	defer func() { prometheusMetricsPageForTestDetach(page) }()

	// Act
	first := s.getMetrics()
	second := s.getMetrics()

	// Assert
	s.Require().Len(first, 2)
	s.Require().Len(second, 2)
	s.Same(first[0].meta, second[0].meta)
	s.Same(first[1].meta, second[1].meta)
}

// TestCacheEvictsDisappearedMetric verifies that cached meta for a metric whose page was freed is swept out.
func (s *CppMetricsSuite) TestCacheEvictsDisappearedMetric() {
	// Arrange
	page := prometheusMetricsPageForTestCtor(Labels{Label{Name: "metrics_page", Value: "evict"}}, "counter_evict", 1)

	first := s.getMetrics()
	s.Require().Len(first, 2)
	s.True(cacheContainsMeta(first[0].meta))

	// Act
	prometheusMetricsPageForTestDetach(page)
	s.getMetrics()

	// Assert
	s.False(cacheContainsMeta(first[0].meta))
}
