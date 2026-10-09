package promql

import (
	"github.com/grafana/regexp"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/model/labels"
)

const (
	// DefaultLabelReplaceCacheSize is the default capacity of the labelset
	// cache used to memoize label_replace results.
	DefaultLabelReplaceCacheSize = 65536

	// labelReplaceRegexCacheSize is the fixed capacity of the compiled-regex
	// cache: regex texts in queries are few and static, so it is not configurable.
	labelReplaceRegexCacheSize = 128

	// Values of the "cache" metric label distinguishing the two caches.
	labelReplaceCacheLabelsID = "labels"
	labelReplaceCacheRegexID  = "regex"
)

// labelReplaceCacheKey fully determines the resulting labelset of a
// label_replace call for one series. baseHash is the hash of the source
// labelset (el.Metric.Hash()) because the result is built from the whole
// source labelset, not only from the string fields. labels.Labels itself
// must not be a key field: it is not comparable under the slicelabels and
// dedupelabels build tags.
type labelReplaceCacheKey struct {
	dst, repl, src, regexStr, srcVal string
	baseHash                         uint64
}

// labelReplaceCache memoizes label_replace results between queries. It owns
// two LRUs: one for resulting labelsets and one for compiled regexes. A nil
// *labelReplaceCache means "disabled" and every method is nil-safe.
type labelReplaceCache struct {
	labelsCache *lru.Cache[labelReplaceCacheKey, labels.Labels]
	regexCache  *lru.Cache[string, *regexp.Regexp]

	hits      *prometheus.CounterVec
	misses    *prometheus.CounterVec
	evictions *prometheus.CounterVec
	entries   *prometheus.GaugeVec
}

// newLabelReplaceCache builds the wrapper: size 0 selects
// DefaultLabelReplaceCacheSize, a negative size disables the cache (nil), and
// any LRU construction error fails open (nil) instead of panicking.
func newLabelReplaceCache(size int) *labelReplaceCache {
	if size < 0 {
		return nil
	}

	if size == 0 {
		size = DefaultLabelReplaceCacheSize
	}

	c := &labelReplaceCache{
		hits: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "label_replace_cache_hits_total",
			Help:      "Total number of label_replace cache hits.",
		}, []string{"cache"}),
		misses: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "label_replace_cache_misses_total",
			Help:      "Total number of label_replace cache misses.",
		}, []string{"cache"}),
		evictions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "label_replace_cache_evictions_total",
			Help:      "Total number of label_replace cache evictions.",
		}, []string{"cache"}),
		entries: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "label_replace_cache_entries",
			Help:      "Current number of entries in the label_replace cache.",
		}, []string{"cache"}),
	}

	// The eviction callback runs outside the LRU lock, so synchronously
	// touching metrics and reading Len() is safe. The cache field is read
	// at callback time, after both constructors below have returned.
	labelsCache, err := lru.NewWithEvict(size, func(labelReplaceCacheKey, labels.Labels) {
		c.evictions.WithLabelValues(labelReplaceCacheLabelsID).Inc()
		c.entries.WithLabelValues(labelReplaceCacheLabelsID).Set(float64(c.labelsCache.Len()))
	})
	if err != nil {
		return nil
	}

	c.labelsCache = labelsCache

	regexCache, err := lru.NewWithEvict(labelReplaceRegexCacheSize, func(string, *regexp.Regexp) {
		c.evictions.WithLabelValues(labelReplaceCacheRegexID).Inc()
		c.entries.WithLabelValues(labelReplaceCacheRegexID).Set(float64(c.regexCache.Len()))
	})
	if err != nil {
		return nil
	}

	c.regexCache = regexCache

	c.entries.WithLabelValues(labelReplaceCacheLabelsID).Set(0)
	c.entries.WithLabelValues(labelReplaceCacheRegexID).Set(0)

	return c
}

// addLabels stores a resulting labelset and refreshes the entries gauge.
func (c *labelReplaceCache) addLabels(key labelReplaceCacheKey, val labels.Labels) {
	if c == nil {
		return
	}

	c.labelsCache.Add(key, val)
	c.entries.WithLabelValues(labelReplaceCacheLabelsID).Set(float64(c.labelsCache.Len()))
}

// getLabels looks up a resulting labelset, accounting a hit or a miss.
func (c *labelReplaceCache) getLabels(key labelReplaceCacheKey) (labels.Labels, bool) {
	if c == nil {
		return labels.EmptyLabels(), false
	}

	if val, ok := c.labelsCache.Get(key); ok {
		c.hits.WithLabelValues(labelReplaceCacheLabelsID).Inc()
		return val, true
	}

	c.misses.WithLabelValues(labelReplaceCacheLabelsID).Inc()

	return labels.EmptyLabels(), false
}

// getOrCompileRegex returns the compiled and fully anchored regex for
// regexStr. Compilation errors are returned without caching, so an invalid
// regex never poisons the cache and always fails the same way.
func (c *labelReplaceCache) getOrCompileRegex(regexStr string) (*regexp.Regexp, error) {
	if c == nil {
		return regexp.Compile("^(?:" + regexStr + ")$")
	}

	if re, ok := c.regexCache.Get(regexStr); ok {
		c.hits.WithLabelValues(labelReplaceCacheRegexID).Inc()
		return re, nil
	}

	c.misses.WithLabelValues(labelReplaceCacheRegexID).Inc()
	re, err := regexp.Compile("^(?:" + regexStr + ")$")
	if err != nil {
		return nil, err
	}

	c.regexCache.Add(regexStr, re)
	c.entries.WithLabelValues(labelReplaceCacheRegexID).Set(float64(c.regexCache.Len()))

	return re, nil
}

// len returns the current number of labelset entries; it is nil-safe.
func (c *labelReplaceCache) len() int {
	if c == nil {
		return 0
	}

	return c.labelsCache.Len()
}

// register registers the cache metrics; call it only with a non-nil
// Registerer (mirrors the opts.Reg != nil pattern in NewEngine).
func (c *labelReplaceCache) register(reg prometheus.Registerer) {
	if c == nil || reg == nil {
		return
	}

	reg.MustRegister(c.hits, c.misses, c.evictions, c.entries)
}
