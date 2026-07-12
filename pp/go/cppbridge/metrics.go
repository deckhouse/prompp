package cppbridge

import (
	"strings"
	"sync"
	"unsafe"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// CppMetric is the raw view returned by the C++ metrics iterator. Its descriptor/metric pointers and every string and
// value they reference live inside a C++ metrics page that may be physically freed on the next scrape (see
// prompp_metrics_iterator_ctor -> remove_unused_pages). It must never be handed to Go consumers directly; it is only
// used, under cppMetricsMu, to build a fully Go-owned CppMetricCopy.
type CppMetric struct {
	descriptor *prometheus.Desc
	metric     *dto.Metric
}

// cppMetricDescriptor mirrors the memory layout of PromPP::Primitives::Go::dto::MetricDescriptor
// (pp/primitives/go_metric.h). It is used as a read-only view over the C++ descriptor to cheaply read fields that
// prometheus.Desc keeps unexported (fqName/help for rebuilding the Desc, id/dimHash for the cache key).
// Keep the field order and sizes in sync with the C++ struct.
type cppMetricDescriptor struct {
	fqName          string           // C++ String{data, len}
	help            string           // C++ String{data, len}
	constLabelPairs []*dto.LabelPair // C++ Slice<const LabelPair*>{data, len, cap}
	variableLabels  unsafe.Pointer   // C++ const CompiledLabels*
	id              uint64
	dimHash         uint64
	// C++ trailing field `Error error` is intentionally omitted: it is never read here.
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
// metric kind. It is cached across scrapes keyed by the descriptor identity ({id, dimHash}) because only the numeric
// value changes between scrapes, so the (relatively expensive) string copies are reused.
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

func (m *CppMetricCopy) Desc() *prometheus.Desc {
	return m.meta.desc
}

func (m *CppMetricCopy) Write(out *dto.Metric) error {
	out.Label = m.meta.labelPairs
	switch m.meta.kind {
	case cppMetricCounter:
		out.Counter = &dto.Counter{Value: &m.value}
	case cppMetricGauge:
		out.Gauge = &dto.Gauge{Value: &m.value}
	}
	return nil
}

// descKey identifies a descriptor for caching. id/dimHash come from the C++ descriptor and together cover
// fqName/help/label names/label values. kind is part of the key because the same descriptor identity can be emitted as
// both a counter and a gauge (e.g. the test page), and their metadata differs.
type descKey struct {
	id      uint64
	dimHash uint64
	kind    cppMetricKind
}

// cppMetricsMu serializes the whole metrics-page iteration and guards metaCache. The underlying C++ storage is not safe
// for concurrent readers: prompp_metrics_iterator_ctor first calls remove_unused_pages(), which physically deletes
// pages detached from the list. If two scrapes ran concurrently (client_golang does not serialize Gather/Collect), one
// scrape could delete a page that another is still iterating, causing a use-after-free. Holding this mutex for the whole
// build phase guarantees a single reader at a time, which is the invariant the detach/remove_unused_pages design relies
// on. Because CppMetrics returns Go-owned copies, consumers may safely read them after the lock is released.
var (
	cppMetricsMu sync.Mutex
	metaCache    = make(map[descKey]*CppMetricMeta)
	metaCacheGen uint64
)

func CppMetrics(f func(metric *CppMetricCopy) bool) {
	for _, metric := range collectCppMetricCopies() {
		if !f(metric) {
			break
		}
	}
}

// collectCppMetricCopies iterates the C++ metric pages under cppMetricsMu, builds fully Go-owned copies, evicts stale
// cache entries and returns the copies. The lock is held only while C++ memory is touched; once this returns every
// referenced byte is Go-owned, so the copies can be read safely after the lock is released (e.g. lazily drained from a
// buffered channel by prometheus.Registry.Gather).
func collectCppMetricCopies() []*CppMetricCopy {
	cppMetricsMu.Lock()
	defer cppMetricsMu.Unlock()

	metaCacheGen++
	gen := metaCacheGen

	iterator := prometheusMetricsIteratorCtor()

	copies := make([]*CppMetricCopy, 0, len(metaCache))
	for {
		metric := prometheusMetricsIteratorNext(&iterator)
		if metric == nil {
			break
		}

		desc := (*cppMetricDescriptor)(unsafe.Pointer(metric.descriptor))
		kind, value := metric.kindAndValue()

		key := descKey{id: desc.id, dimHash: desc.dimHash, kind: kind}
		meta := metaCache[key]
		if meta == nil {
			meta = buildMeta(metric, desc, kind)
			metaCache[key] = meta
		}
		meta.lastSeenGen = gen

		copies = append(copies, &CppMetricCopy{meta: meta, value: value})
	}

	// Mark-and-sweep: drop metadata for descriptors that disappeared (e.g. their page was freed). Entries seen in this
	// iteration carry the current generation; everything else is stale.
	for key, meta := range metaCache {
		if meta.lastSeenGen != gen {
			delete(metaCache, key)
		}
	}

	return copies
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
