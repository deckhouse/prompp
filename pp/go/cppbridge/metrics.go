package cppbridge

import (
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

func CppMetrics(f func(metric *CppMetric) bool) {
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
