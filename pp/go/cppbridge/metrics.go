package cppbridge

import (
	"strings"
	"sync"
	"unsafe"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// CppMetric is the raw view returned by the C++ metrics iterator. Its descriptor/metric pointers and every string and
// value they reference live inside a C++ metrics page that may be physically freed after the scrape (see
// prompp_metrics_remove_unused_pages, called at the end of CppMetrics). It must never be handed to Go consumers
// directly; it is only used, under cppMetricsMu, to build a fully Go-owned CppMetricCopy.
type CppMetric struct {
	descriptor *prometheus.Desc
	metric     *dto.Metric
}

// cppMetricDescriptor mirrors the leading fields of PromPP::Primitives::Go::dto::MetricDescriptor
// (pp/primitives/go_metric.h). It is used as a read-only view over the C++ descriptor to cheaply read fqName/help,
// which prometheus.Desc keeps unexported, when (re)building a Desc. Only the leading fields are mirrored; the trailing
// fields are never read here. Keep the field order and sizes of the mirrored prefix in sync with the C++ struct.
type cppMetricDescriptor struct {
	fqName string // C++ String{data, len}
	help   string // C++ String{data, len}
}

type cppMetricKind uint8

const (
	cppMetricCounter cppMetricKind = iota
	cppMetricGauge
)

// kindAndValue reads the metric kind and its current numeric value from the C++-owned dto.Metric. The value pointer
// dereferenced here points into the C++ page, so this must be called while cppMetricsMu is held; the returned float64
// is a plain Go copy.
func (m *CppMetric) kindAndValue() (cppMetricKind, float64) {
	if c := m.metric.Counter; c != nil {
		return cppMetricCounter, c.GetValue()
	}
	return cppMetricGauge, m.metric.Gauge.GetValue()
}

// CppMetricMeta is the immutable, fully Go-owned part of a metric: the descriptor, the emitted label pairs and the
// metric kind. It is cached across scrapes keyed by the C++ descriptor address because only the numeric value changes
// between scrapes, so the (relatively expensive) string copies are reused.
type CppMetricMeta struct {
	desc        *prometheus.Desc
	labelPairs  []*dto.LabelPair
	kind        cppMetricKind
	lastSeenGen uint64
}

// CppMetricCopy is a per-scrape, fully Go-owned snapshot of a single metric. It implements prometheus.Metric and is
// therefore safe to buffer in the collect channel and read after cppMetricsMu has been released.
type CppMetricCopy struct {
	meta  *CppMetricMeta
	value float64
}

func (m CppMetricCopy) Desc() *prometheus.Desc {
	return m.meta.desc
}

func (m CppMetricCopy) Write(out *dto.Metric) error {
	out.Label = m.meta.labelPairs
	switch m.meta.kind {
	case cppMetricCounter:
		out.Counter = &dto.Counter{Value: &m.value}
	case cppMetricGauge:
		out.Gauge = &dto.Gauge{Value: &m.value}
	}
	return nil
}

// cppMetricsMu serializes the whole metrics-page iteration and guards metaCache. The underlying C++ storage is not safe
// for concurrent readers: prompp_metrics_remove_unused_pages() (called at the end of the scrape) physically deletes
// pages detached from the list. If two scrapes ran concurrently (client_golang does not serialize Gather/Collect), one
// scrape could delete a page that another is still iterating, causing a use-after-free. Holding this mutex for the whole
// build phase guarantees a single reader at a time, which is the invariant the detach/remove_unused_pages design relies
// on. Because CppMetrics returns Go-owned copies, consumers may safely read them after the lock is released.
//
// The cache is keyed by the C++ descriptor address. This is safe against address reuse (ABA) because page deletion is
// deferred by one generation on the C++ side: a page detached during a scrape is only freed by the next scrape, which
// no longer observes it, so the mark-and-sweep below evicts its cache entry in the very scrape that frees its address.
var (
	cppMetricsMu sync.Mutex
	metaCache    = make(map[unsafe.Pointer]*CppMetricMeta)
	metaCacheGen uint64
)

// CppMetrics iterates the C++ metric pages under cppMetricsMu, builds a fully Go-owned CppMetricCopy for every metric
// and passes it by value to f. Iteration stops early if f returns false.
//
// The lock is held for the entire call, f included: the C++ storage is not safe for concurrent readers (see
// cppMetricsMu), so the whole iteration must run as a single reader. Each CppMetricCopy is fully Go-owned, so f may
// retain it and read it after CppMetrics returns (e.g. buffer it in prometheus' collect channel and drain it later).
// Passing the copy by value lets callers forward it straight to a consumer without allocating a per-scrape slice.
func CppMetrics(f func(metric CppMetricCopy) bool) {
	cppMetricsMu.Lock()
	defer cppMetricsMu.Unlock()

	metaCacheGen++
	gen := metaCacheGen

	iterator := prometheusMetricsIteratorCtor()

	for {
		metric := prometheusMetricsIteratorNext(&iterator)
		if metric == nil {
			break
		}

		kind, value := metric.kindAndValue()

		key := unsafe.Pointer(metric.descriptor)
		meta := metaCache[key]
		if meta == nil {
			desc := (*cppMetricDescriptor)(unsafe.Pointer(metric.descriptor))
			meta = buildMeta(metric, desc, kind)
			metaCache[key] = meta
		}
		meta.lastSeenGen = gen

		if !f(CppMetricCopy{meta: meta, value: value}) {
			// Early stop: not every page was visited, so skip both the page reclamation and the mark-and-sweep below.
			// Nothing is freed and nothing is evicted, so no cache entry can dangle; the next full scrape cleans up.
			return
		}
	}

	// Reclaim pages detached before this scrape (their deletion was deferred one generation on the C++ side). Freeing an
	// address here and evicting its stale cache entry in the sweep below happen in the same scrape, so no key can dangle.
	prometheusMetricsRemoveUnusedPages(&iterator)

	// Mark-and-sweep: drop metadata for descriptors that disappeared (their page was freed above). Entries seen in this
	// iteration carry the current generation; everything else is stale.
	for key, meta := range metaCache {
		if meta.lastSeenGen != gen {
			delete(metaCache, key)
		}
	}
}

// buildMeta deep-copies the descriptor and label pairs from the C++ page into Go-owned memory. Every string is cloned
// so the resulting CppMetricMeta holds no pointers into C++ memory.
func buildMeta(metric *CppMetric, desc *cppMetricDescriptor, kind cppMetricKind) *CppMetricMeta {
	constLabels := make(prometheus.Labels, len(metric.metric.Label))
	labelPairs := make([]*dto.LabelPair, 0, len(metric.metric.Label))
	for _, l := range metric.metric.Label {
		name := strings.Clone(l.GetName())
		value := strings.Clone(l.GetValue())
		constLabels[name] = value
		labelPairs = append(labelPairs, &dto.LabelPair{Name: &name, Value: &value})
	}

	return &CppMetricMeta{
		desc:       prometheus.NewDesc(strings.Clone(desc.fqName), strings.Clone(desc.help), nil, constLabels),
		labelPairs: labelPairs,
		kind:       kind,
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
