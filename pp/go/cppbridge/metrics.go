package cppbridge

import (
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// CppMetric mirrors, field for field, the C++ PromPP::Primitives::Go::Metric struct: the pointer returned by
// prometheusMetricsIteratorNext points directly into C++ metrics-page memory, so this struct is a memory view of it,
// not a Go-owned copy. The field order, sizes and offsets MUST stay in lockstep with go_metric.h (descriptor and
// metric pointers followed by the active flag); a static_assert on the C++ side guards the layout.
//
// active is written from Go (via the finalizer below, atomic.Uint32.Store) and read from C++
// (std::atomic<uint32_t>) over the very same word. It is the handshake that keeps a detached page alive until Go
// no longer references any of its metrics.
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
			// The finalizer clears the C++ active flag once Go drops the last reference to this wrapper. This is the
			// whole point of the wrapper cache: the backing C++ page is physically freed by remove_unused_pages()
			// only when it is detached AND !is_active(), and active is cleared *exclusively* here. So the page is
			// guaranteed to still be alive when the finalizer writes to it, and its address cannot be reused until
			// every metric of the page has been finalized (no ABA). The tradeoff is that a detached page lingers
			// until the next GC collects its wrappers, i.e. reclamation latency is now tied to GC scheduling.
			wrappedMetric = &CppMetricWrapper{cppMetric: cppMetric, existsInCppStorageAtGeneration: m.generation}
			runtime.SetFinalizer(wrappedMetric, func(wrappedMetric *CppMetricWrapper) {
				wrappedMetric.cppMetric.active.Store(0)
			})
			m.cache[cppMetric] = wrappedMetric
		} else {
			wrappedMetric.existsInCppStorageAtGeneration = m.generation
		}

		// Once a consumer returns false we stop invoking f, but we deliberately keep iterating every remaining page:
		// the loop below prunes cache entries by generation, so each still-present page must be stamped with the
		// current generation on every call, otherwise a live page's wrapper would be dropped and finalized too early.
		callHandler = callHandler && f(wrappedMetric)
	}

	// Drop wrappers for pages that were not visited this generation (i.e. detached and no longer iterated). Removing
	// the last reference lets the finalizer run, clear active, and thus allow remove_unused_pages() to free the page.
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
