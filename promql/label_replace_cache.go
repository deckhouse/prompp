package promql

import (
	"sync/atomic"
	"time"

	"github.com/grafana/regexp"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/model/labels"
)

const (
	// DefaultLabelReplaceCacheSize is the default maximum capacity of the
	// labelset cache used to memoize label_replace results.
	DefaultLabelReplaceCacheSize = 65536

	// DefaultLabelReplaceRegexCacheSize is the default capacity of the
	// compiled-regex cache: regex texts in queries are few and static, so the
	// default is small.
	DefaultLabelReplaceRegexCacheSize = 128

	// Values of the "cache" metric label distinguishing the two caches.
	labelReplaceCacheLabelsID = "labels"
	labelReplaceCacheRegexID  = "regex"

	// Values of the "direction" metric label of the capacity-resize counter.
	labelReplaceCacheGrowDirection   = "grow"
	labelReplaceCacheShrinkDirection = "shrink"

	// labelReplaceCacheSweepInterval is the period of the labels-cache sweep
	// ticker run by the engine.
	labelReplaceCacheSweepInterval = 60 * time.Second

	// labelReplaceCacheStaleTicks is the number of sweep intervals without a
	// hit after which an entry counts as stale and is swept.
	labelReplaceCacheStaleTicks = 2

	// labelReplaceCacheShrinkFreedDivisor sets the shrink trigger to a freed
	// quarter of the current capacity.
	labelReplaceCacheShrinkFreedDivisor = 4

	// labelReplaceCacheShrinkTargetDivisor leaves a tenth of headroom above
	// the surviving entry count on shrink.
	labelReplaceCacheShrinkTargetDivisor = 10

	// labelReplaceCacheShrinkFloorMin is the absolute floor of the labels
	// cache capacity.
	labelReplaceCacheShrinkFloorMin = 64

	// labelReplaceCacheShrinkFloorDivisor makes the relative floor of the
	// labels cache capacity a hundredth of the configured maximum.
	labelReplaceCacheShrinkFloorDivisor = 100
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

// labelReplaceCacheEntry is the value stored in the labels cache: the
// memoized labelset plus the idle timestamp. idleTs counts monotonic
// milliseconds from wrapper creation and is refreshed on every hit, so it
// measures idleness rather than age. It is atomic because concurrent query
// hits refresh it without holding any lock; the entry itself is shared by
// pointer so every holder observes the refresh.
type labelReplaceCacheEntry struct {
	result labels.Labels
	idleTs atomic.Int64
}

// labelReplaceCache memoizes label_replace results between queries. It owns
// two LRUs: one for resulting labelsets and one for compiled regexes. A nil
// *labelReplaceCache means "disabled" and every method is nil-safe. Either
// LRU may independently be nil when its mode is disabled. The sweep
// controller runs in a single engine goroutine; concurrent query callbacks
// only touch lock-free or library-guarded state.
type labelReplaceCache struct {
	labelsCache *lru.Cache[labelReplaceCacheKey, *labelReplaceCacheEntry]
	regexCache  *lru.Cache[string, *regexp.Regexp]

	hits      *prometheus.CounterVec
	misses    *prometheus.CounterVec
	evictions *prometheus.CounterVec
	resizes   *prometheus.CounterVec
	entries   *prometheus.GaugeVec
	capacity  *prometheus.GaugeVec

	// labelsMax is the configured maximum and labelsCap the current capacity
	// of the labels cache. Both are written only by construction and by the
	// sweep goroutine, and read by it as well as by tests.
	labelsMax int
	labelsCap int

	// nowMs is the cached monotonic clock (milliseconds since wrapper
	// creation) refreshed by the sweep ticker and read by the hit path, so
	// hits never call the wall clock. Tests inject it directly.
	nowMs atomic.Int64

	// startedAt anchors the monotonic clock of nowMs.
	startedAt time.Time
}

// newLabelReplaceCache builds the wrapper for the two caches. The sizes
// combine independently: labelSize 0 selects DefaultLabelReplaceCacheSize
// and a negative labelSize disables only the labels cache; regexSize 0
// selects DefaultLabelReplaceRegexCacheSize and a negative regexSize
// disables only the regex cache (compile on every call without storing).
// Both sizes negative returns a nil wrapper (everything off); a negative
// labelSize with a non-negative regexSize still yields a live wrapper so a
// regex-only configuration stays representable. The labels LRU starts at
// the shrink floor and the sweep grows or shrinks it from there. LRU
// construction errors fail open (nil) instead of panicking.
func newLabelReplaceCache(labelSize, regexSize int) *labelReplaceCache {
	if labelSize < 0 && regexSize < 0 {
		return nil
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
		resizes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "label_replace_cache_resizes_total",
			Help:      "Total number of label_replace cache capacity resizes.",
		}, []string{"direction"}),
		entries: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "label_replace_cache_entries",
			Help:      "Current number of entries in the label_replace cache.",
		}, []string{"cache"}),
		capacity: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "label_replace_cache_capacity",
			Help:      "Configured capacity of the label_replace cache.",
		}, []string{"cache"}),
		startedAt: time.Now(),
	}

	// The eviction callbacks run outside the LRU lock, so synchronously
	// touching metrics and reading Len() is safe. Each cache field is read
	// at callback time, after its constructor has returned.
	if labelSize >= 0 {
		if labelSize == 0 {
			labelSize = DefaultLabelReplaceCacheSize
		}
		c.labelsMax = labelSize
		// Start at the floor: below it the cache cannot shrink anyway, so a
		// higher start would only hold garbage longer. A tiny maximum still
		// starts at itself.
		c.labelsCap = min(labelReplaceCacheShrinkFloor(labelSize), labelSize)

		labelsCache, err := lru.NewWithEvict(c.labelsCap, c.onLabelsEvict)
		if err != nil {
			return nil
		}

		c.labelsCache = labelsCache
		c.entries.WithLabelValues(labelReplaceCacheLabelsID).Set(0)
		c.capacity.WithLabelValues(labelReplaceCacheLabelsID).Set(float64(c.labelsCap))
	}

	if regexSize >= 0 {
		if regexSize == 0 {
			regexSize = DefaultLabelReplaceRegexCacheSize
		}

		regexCache, err := lru.NewWithEvict(regexSize, func(string, *regexp.Regexp) {
			c.evictions.WithLabelValues(labelReplaceCacheRegexID).Inc()
			c.entries.WithLabelValues(labelReplaceCacheRegexID).Set(float64(c.regexCache.Len()))
		})
		if err != nil {
			return nil
		}

		c.regexCache = regexCache
		c.entries.WithLabelValues(labelReplaceCacheRegexID).Set(0)
		c.capacity.WithLabelValues(labelReplaceCacheRegexID).Set(float64(regexSize))
	}

	return c
}

// addLabels stores a resulting labelset stamped with the cached timestamp
// and refreshes the entries gauge; it is a noop when the labels cache is
// disabled but the wrapper is alive.
func (c *labelReplaceCache) addLabels(key labelReplaceCacheKey, val labels.Labels) {
	if c == nil || c.labelsCache == nil {
		return
	}

	entry := &labelReplaceCacheEntry{result: val}
	entry.idleTs.Store(c.nowMs.Load())
	c.labelsCache.Add(key, entry)
	c.entries.WithLabelValues(labelReplaceCacheLabelsID).Set(float64(c.labelsCache.Len()))
}

// getLabels looks up a memoized labelset, accounting a hit or a miss. Entry
// age is irrelevant here — a memoized result stays valid forever, so even a
// stale entry is served; age matters only to the sweep. A hit refreshes the
// entry idle timestamp. When the labels cache is disabled but the wrapper
// is alive, every lookup misses without accounting, mirroring the disabled
// regex cache.
func (c *labelReplaceCache) getLabels(key labelReplaceCacheKey) (*labelReplaceCacheEntry, bool) {
	if c == nil || c.labelsCache == nil {
		return nil, false
	}

	val, ok := c.labelsCache.Get(key)
	if !ok {
		c.misses.WithLabelValues(labelReplaceCacheLabelsID).Inc()

		return nil, false
	}

	c.hits.WithLabelValues(labelReplaceCacheLabelsID).Inc()
	val.idleTs.Store(c.nowMs.Load())

	return val, true
}

// getOrCompileRegex returns the compiled and fully anchored regex for
// regexStr. Compilation errors are returned without caching, so an invalid
// regex never poisons the cache and always fails the same way. When the regex
// cache is disabled but the wrapper is alive, every call compiles without
// storing or accounting, preserving eager validation.
func (c *labelReplaceCache) getOrCompileRegex(regexStr string) (*regexp.Regexp, error) {
	if c == nil || c.regexCache == nil {
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

// len returns the current number of labelset entries; it reports 0 when the
// labels cache is disabled and is nil-safe.
func (c *labelReplaceCache) len() int {
	if c == nil || c.labelsCache == nil {
		return 0
	}

	return c.labelsCache.Len()
}

// onLabelsEvict is the labels LRU eviction callback: it accounts the
// eviction and refreshes the entries gauge. Capacity decisions belong to the
// sweep, which runs in a single goroutine.
func (c *labelReplaceCache) onLabelsEvict(_ labelReplaceCacheKey, _ *labelReplaceCacheEntry) {
	c.evictions.WithLabelValues(labelReplaceCacheLabelsID).Inc()
	c.entries.WithLabelValues(labelReplaceCacheLabelsID).Set(float64(c.labelsCache.Len()))
}

// maybeSweep runs a labels-cache sweep when a sweep interval elapsed since
// the previous one. The winner is elected by a compare-and-swap on the
// cached clock, so concurrent queries never sweep twice; losers return
// without waiting. Nil-safe: a nil or disabled cache never sweeps.
func (c *labelReplaceCache) maybeSweep() {
	if c == nil || c.labelsCache == nil {
		return
	}

	c.maybeSweepAt(time.Since(c.startedAt).Milliseconds())
}

// maybeSweepAt is maybeSweep with an injected wall clock, so tests stay
// deterministic without sleeping.
func (c *labelReplaceCache) maybeSweepAt(wallMs int64) {
	if c == nil || c.labelsCache == nil {
		return
	}

	current := c.nowMs.Load()
	if wallMs-current < int64(labelReplaceCacheSweepInterval.Milliseconds()) {
		return
	}
	if !c.nowMs.CompareAndSwap(current, wallMs) {
		return
	}

	c.sweepOnce(wallMs)
}

// register registers the cache metrics; call it only with a non-nil
// Registerer (mirrors the opts.Reg != nil pattern in NewEngine).
func (c *labelReplaceCache) register(reg prometheus.Registerer) {
	if c == nil || reg == nil {
		return
	}

	reg.MustRegister(c.hits, c.misses, c.evictions, c.resizes, c.entries, c.capacity)
}

// resize applies a labels-cache capacity change and updates the capacity
// gauge and the direction counter. Only the sweep calls it.
func (c *labelReplaceCache) resize(target int, direction string) {
	c.labelsCap = target
	c.labelsCache.Resize(target)
	c.capacity.WithLabelValues(labelReplaceCacheLabelsID).Set(float64(target))
	c.resizes.WithLabelValues(direction).Inc()
}

// sweepOnce walks the labels cache from the oldest entry, removing entries
// idle for longer than two sweep intervals, and stops at the first fresh
// entry. Afterwards a still-full cache grows doubled toward its maximum and
// a sweep that freed a quarter or more shrinks toward the surviving count
// plus headroom. No-op decisions change nothing.
func (c *labelReplaceCache) sweepOnce(nowMs int64) {
	if c == nil || c.labelsCache == nil {
		return
	}

	staleAfterMs := int64(labelReplaceCacheStaleTicks) * labelReplaceCacheSweepInterval.Milliseconds()
	freed := 0
	for {
		_, val, ok := c.labelsCache.GetOldest()
		if !ok || nowMs-val.idleTs.Load() <= staleAfterMs {
			break
		}

		c.labelsCache.RemoveOldest()
		freed++
	}

	length, capacity := c.labelsCache.Len(), c.labelsCap
	switch {
	// Still full after the sweep: the live set does not fit, grow along the
	// progressive schedule toward the configured maximum.
	case length == capacity && capacity < c.labelsMax:
		c.resize(min(labelReplaceCacheGrowTarget(capacity), c.labelsMax), labelReplaceCacheGrowDirection)
	// Freed a quarter or more, or drifted below two thirds with progress:
	// shrink toward the survivors plus headroom, never below the floor.
	case freed >= capacity/labelReplaceCacheShrinkFreedDivisor ||
		(freed > 0 && length*3 < capacity*2):
		floor := max(labelReplaceCacheShrinkFloorMin, c.labelsMax/labelReplaceCacheShrinkFloorDivisor)
		if target := max(length+length/labelReplaceCacheShrinkTargetDivisor, floor); target < capacity {
			c.resize(target, labelReplaceCacheShrinkDirection)
		}
	}
}

// labelReplaceCacheShrinkFloor returns the capacity floor: an absolute 64
// entries or a twentieth of the configured maximum, whichever is larger.
func labelReplaceCacheShrinkFloor(maximum int) int {
	return max(labelReplaceCacheShrinkFloorMin, maximum/labelReplaceCacheShrinkFloorDivisor)
}

// labelReplaceCacheGrowSteps maps a current capacity to its growth
// multiplier, checked from the largest threshold down: small capacities
// jump aggressively since unused capacity costs nothing (the LRU never
// preallocates), which shortens the ramp to a large working set.
var labelReplaceCacheGrowSteps = [...][2]int{
	{2048, 2},
	{512, 4},
	{0, 8},
}

// labelReplaceCacheGrowTarget returns the next capacity after the current
// one: the current capacity times the first multiplier whose threshold it
// exceeds.
func labelReplaceCacheGrowTarget(capacity int) int {
	for _, step := range labelReplaceCacheGrowSteps {
		if capacity > step[0] {
			return capacity * step[1]
		}
	}

	return capacity * 2
}
