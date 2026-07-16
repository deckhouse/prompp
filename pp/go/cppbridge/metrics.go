package cppbridge

import (
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

type CppMetric struct {
	descriptor *prometheus.Desc
	metric     *dto.Metric
	active     atomic.Uint32
}

type CppMetricWrapper struct {
	cppMetric                      *CppMetric
	existsInCppStorageAtGeneration uint64
}

func (m *CppMetricWrapper) Desc() *prometheus.Desc {
	return m.cppMetric.descriptor
}

func (m *CppMetricWrapper) Write(out *dto.Metric) error {
	out.Label = m.cppMetric.metric.Label
	out.Counter = m.cppMetric.metric.Counter
	out.Gauge = m.cppMetric.metric.Gauge
	out.Untyped = m.cppMetric.metric.Untyped
	return nil
}

func (m *CppMetricWrapper) Labels() Labels {
	labels := make(Labels, 0, len(m.cppMetric.metric.Label))
	for _, l := range m.cppMetric.metric.Label {
		labels = append(labels, Label{Name: *l.Name, Value: *l.Value})
	}
	return labels
}

type cppMetrics struct {
	generation uint64
	cache      map[*CppMetric]*CppMetricWrapper

	// mutex serializes the whole metrics-page iteration. The underlying C++ storage is not safe for concurrent
	// readers: prompp_metrics_iterator_ctor first calls remove_unused_pages(), which physically deletes pages detached from
	// the list. If two scrapes run concurrently (client_golang does not serialize Gather/Collect), one scrape could delete a
	// page that another scrape is still iterating, causing a use-after-free. Holding this mutex for the full iteration
	// guarantees a single reader at a time, which is the invariant the detach/remove_unused_pages design relies on.
	mutex sync.Mutex
}

func (m *cppMetrics) Range(f func(metric *CppMetricWrapper) bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	iterator := prometheusMetricsIteratorCtor()
	m.generation++

	callHandler := true
	for {
		cppMetric := prometheusMetricsIteratorNext(&iterator)
		if cppMetric == nil {
			break
		}

		wrappedMetric, ok := m.cache[cppMetric]
		if !ok {
			wrappedMetric = &CppMetricWrapper{cppMetric: cppMetric, existsInCppStorageAtGeneration: m.generation}
			runtime.SetFinalizer(wrappedMetric, func(wrappedMetric *CppMetricWrapper) {
				wrappedMetric.cppMetric.active.Store(0)
			})
			m.cache[cppMetric] = wrappedMetric
		} else {
			wrappedMetric.existsInCppStorageAtGeneration = m.generation
		}

		callHandler = callHandler && f(wrappedMetric)
	}

	for key, wrappedMetric := range m.cache {
		if wrappedMetric.existsInCppStorageAtGeneration != m.generation {
			delete(m.cache, key)
		}
	}
}

var CppMetrics = &cppMetrics{
	cache: make(map[*CppMetric]*CppMetricWrapper),
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
	for metric := range CppMetrics.Range {
		ch <- metric
	}
}

func init() {
	prometheusMetricsRegister()
}
