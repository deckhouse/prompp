package cppbridge

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

type CppMetric struct {
	descriptor *prometheus.Desc
	metric     *dto.Metric
}

func (m *CppMetric) Desc() *prometheus.Desc {
	return m.descriptor
}

func (m *CppMetric) Write(out *dto.Metric) error {
	out.Label = m.metric.Label
	out.Counter = m.metric.Counter
	out.Gauge = m.metric.Gauge
	out.Untyped = m.metric.Untyped
	return nil
}

func (m *CppMetric) Labels() Labels {
	labels := make(Labels, 0, len(m.metric.Label))
	for _, l := range m.metric.Label {
		labels = append(labels, Label{Name: *l.Name, Value: *l.Value})
	}
	return labels
}

// cppMetricsMu serializes the whole metrics-page iteration. The underlying C++ storage is not safe for concurrent
// readers: prompp_metrics_iterator_ctor first calls remove_unused_pages(), which physically deletes pages detached from
// the list. If two scrapes run concurrently (client_golang does not serialize Gather/Collect), one scrape could delete a
// page that another scrape is still iterating, causing a use-after-free. Holding this mutex for the full iteration
// guarantees a single reader at a time, which is the invariant the detach/remove_unused_pages design relies on.
var cppMetricsMu sync.Mutex

func CppMetrics(f func(metric *CppMetric) bool) {
	cppMetricsMu.Lock()
	defer cppMetricsMu.Unlock()

	iterator := prometheusMetricsIteratorCtor()

	for {
		metric := prometheusMetricsIteratorNext(&iterator)
		if metric == nil || !f(metric) {
			break
		}
	}
}

type CppMetricsCollector struct{}

func NewCppMetricsCollector(reg prometheus.Registerer) {
	if reg == nil {
		return
	}

	_ = reg.Register(&CppMetricsCollector{})
}

func (c *CppMetricsCollector) Describe(chan<- *prometheus.Desc) {
}

func (c *CppMetricsCollector) Collect(ch chan<- prometheus.Metric) {
	for metric := range CppMetrics {
		ch <- metric
	}
}

func init() {
	prometheusMetricsRegister()
}
