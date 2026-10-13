package promql

import (
	"hash/maphash"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/grafana/regexp"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/hashicorp/golang-lru/v2/simplelru"
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

	// labelReplaceCacheSweepIntervalMs is the minimum interval between query-driven sweeps in milliseconds.
	labelReplaceCacheSweepIntervalMs int64 = 60_000

	// labelReplaceCacheStaleTicks is the number of sweep intervals without a
	// hit after which an entry counts as stale and is swept.
	labelReplaceCacheStaleTicks = 2

	labelReplaceCacheStaleAfterMs = labelReplaceCacheStaleTicks * labelReplaceCacheSweepIntervalMs

	// labelReplaceCacheSweepBatchSize bounds removals while labelsMu is held.
	labelReplaceCacheSweepBatchSize = 128

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

// labelReplaceCacheGrowSteps maps a current capacity to its growth
// multiplier, checked from the largest threshold down: small capacities
// jump aggressively since unused capacity costs nothing (the LRU never
// preallocates), which shortens the ramp to a large working set.
var labelReplaceCacheGrowSteps = [...][2]int{
	{2048, 2},
	{512, 4},
	{0, 8},
}

type labelReplaceOperationKey struct {
	dst, repl, src, regexStr string
}

// labelReplaceOperation keeps immutable call parameters. Bucket links and
// references to active evaluations and cached keys are guarded by labelsMu.
type labelReplaceOperation struct {
	key        labelReplaceOperationKey
	hash       uint64
	next       *labelReplaceOperation
	references int
}

// labelReplaceCacheKey includes operation identity and the whole source labelset
// hash because the result depends on every source label, not just srcVal.
type labelReplaceCacheKey struct {
	operation *labelReplaceOperation
	srcVal    string
	baseHash  uint64
}

// labelReplaceCacheEntry is the value stored in the labels cache: the
// memoized labelset plus the idle timestamp. idleTs counts monotonic
// milliseconds from wrapper creation and is refreshed on every hit, so it
// measures idleness rather than age. labelsMu guards the entry; lookups return
// a labelset snapshot so an evicted entry can be reused safely.
type labelReplaceCacheEntry struct {
	result labels.Labels
	idleTs int64
}

type labelReplaceCacheMetrics struct {
	hits      prometheus.Counter
	misses    prometheus.Counter
	evictions prometheus.Counter
	entries   prometheus.Gauge
	capacity  prometheus.Gauge
}

// labelReplaceCache memoizes label_replace results between queries. It owns
// two LRUs: one for resulting labelsets and one for compiled regexes. A nil
// *labelReplaceCache means "disabled" and every method is nil-safe. Either
// LRU may independently be nil when its mode is disabled. labelsMu serializes
// label lookups, timestamp updates and sweeps; sweepMu prevents overlapping sweeps.
type labelReplaceCache struct {
	labelsCache   *simplelru.LRU[labelReplaceCacheKey, *labelReplaceCacheEntry]
	regexCache    *lru.Cache[string, *regexp.Regexp]
	operations    map[uint64]*labelReplaceOperation
	operationSeed maphash.Seed

	// spareEntry holds one cleared entry for reuse on the next insertion.
	spareEntry *labelReplaceCacheEntry

	labelsMu sync.Mutex
	sweepMu  sync.Mutex

	// sweepClosed is guarded by sweepMu and prevents scheduling after close.
	sweepClosed bool

	labelsMetrics labelReplaceCacheMetrics
	regexMetrics  labelReplaceCacheMetrics
	growResizes   prometheus.Counter
	shrinkResizes prometheus.Counter

	// labelsMax is the configured maximum and labelsCap the current capacity
	// of the labels cache. labelsMax is immutable; labelsMu guards labelsCap.
	labelsMax int
	labelsCap int

	// nowMs is the cached monotonic clock (milliseconds since wrapper
	// creation) refreshed by query-driven sweeps and read by the hit path, so
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
// construction errors fail open (nil) instead of panicking. Metrics are
// registered during construction when reg is non-nil.
func newLabelReplaceCache(labelSize, regexSize int, reg prometheus.Registerer) *labelReplaceCache {
	if labelSize < 0 && regexSize < 0 {
		return nil
	}

	hits := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "label_replace_cache_hits_total",
		Help:      "Total number of label_replace cache hits.",
	}, []string{"cache"})
	misses := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "label_replace_cache_misses_total",
		Help:      "Total number of label_replace cache misses.",
	}, []string{"cache"})
	evictions := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "label_replace_cache_evictions_total",
		Help:      "Total number of label_replace cache evictions.",
	}, []string{"cache"})
	resizes := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "label_replace_cache_resizes_total",
		Help:      "Total number of label_replace cache capacity resizes.",
	}, []string{"direction"})
	entries := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "label_replace_cache_entries",
		Help:      "Current number of entries in the label_replace cache.",
	}, []string{"cache"})
	capacity := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "label_replace_cache_capacity",
		Help:      "Configured capacity of the label_replace cache.",
	}, []string{"cache"})

	metricsFor := func(cacheID string) labelReplaceCacheMetrics {
		return labelReplaceCacheMetrics{
			hits:      hits.WithLabelValues(cacheID),
			misses:    misses.WithLabelValues(cacheID),
			evictions: evictions.WithLabelValues(cacheID),
			entries:   entries.WithLabelValues(cacheID),
			capacity:  capacity.WithLabelValues(cacheID),
		}
	}

	c := &labelReplaceCache{startedAt: time.Now()}

	// Labels eviction callbacks run under labelsMu; regex callbacks run outside
	// the library lock. Each callback can safely read its cache length.
	if labelSize >= 0 {
		if labelSize == 0 {
			labelSize = DefaultLabelReplaceCacheSize
		}
		c.labelsMetrics = metricsFor(labelReplaceCacheLabelsID)
		c.growResizes = resizes.WithLabelValues(labelReplaceCacheGrowDirection)
		c.shrinkResizes = resizes.WithLabelValues(labelReplaceCacheShrinkDirection)
		c.labelsMax = labelSize
		c.operations = make(map[uint64]*labelReplaceOperation)
		c.operationSeed = maphash.MakeSeed()
		// Start at the floor: below it the cache cannot shrink anyway, so a
		// higher start would only hold garbage longer. A tiny maximum still
		// starts at itself.
		c.labelsCap = min(labelReplaceCacheShrinkFloor(labelSize), labelSize)

		labelsCache, err := simplelru.NewLRU(c.labelsCap, c.onLabelsEvict)
		if err != nil {
			return nil
		}

		c.labelsCache = labelsCache
		c.labelsMetrics.entries.Set(0)
		c.labelsMetrics.capacity.Set(float64(c.labelsCap))
	}

	if regexSize >= 0 {
		if regexSize == 0 {
			regexSize = DefaultLabelReplaceRegexCacheSize
		}

		c.regexMetrics = metricsFor(labelReplaceCacheRegexID)
		regexCache, err := lru.NewWithEvict(regexSize, func(string, *regexp.Regexp) {
			c.regexMetrics.evictions.Inc()
			c.regexMetrics.entries.Set(float64(c.regexCache.Len()))
		})
		if err != nil {
			return nil
		}

		c.regexCache = regexCache
		c.regexMetrics.entries.Set(0)
		c.regexMetrics.capacity.Set(float64(regexSize))
	}

	if reg != nil {
		reg.MustRegister(hits, misses, evictions, resizes, entries, capacity)
	}

	return c
}

// acquireOperation holds one evaluation reference until releaseOperation.
func (c *labelReplaceCache) acquireOperation(key labelReplaceOperationKey) *labelReplaceOperation {
	if c == nil || c.labelsCache == nil {
		return nil
	}

	return c.acquireOperationWithHash(key, maphash.Comparable(c.operationSeed, key))
}

// acquireOperationWithHash verifies the full parameters so hash collisions never
// alias operations. All bucket links and reference counts are guarded by labelsMu.
func (c *labelReplaceCache) acquireOperationWithHash(key labelReplaceOperationKey, hash uint64) *labelReplaceOperation {
	c.labelsMu.Lock()
	for operation := c.operations[hash]; operation != nil; operation = operation.next {
		if operation.key == key {
			operation.references++
			c.labelsMu.Unlock()

			return operation
		}
	}

	operation := &labelReplaceOperation{key: key, hash: hash, next: c.operations[hash], references: 1}
	c.operations[hash] = operation
	c.labelsMu.Unlock()

	return operation
}

// addLabels stores a resulting labelset stamped with the cached timestamp
// and refreshes the entries gauge; it is a noop when the labels cache is
// disabled but the wrapper is alive.
func (c *labelReplaceCache) addLabels(key labelReplaceCacheKey, val labels.Labels) {
	if c == nil || c.labelsCache == nil {
		return
	}

	c.labelsMu.Lock()

	entry := c.spareEntry
	if entry == nil {
		entry = &labelReplaceCacheEntry{}
	}

	c.spareEntry = nil
	entry.result = val
	entry.idleTs = c.nowMs.Load()
	if !c.labelsCache.Contains(key) {
		key.operation.references++
	}

	c.labelsCache.Add(key, entry)
	c.labelsMetrics.entries.Set(float64(c.labelsCache.Len()))

	c.labelsMu.Unlock()
}

// close waits for an admitted sweep and prevents future background sweeps.
func (c *labelReplaceCache) close() {
	if c == nil {
		return
	}

	c.sweepMu.Lock()
	c.sweepClosed = true
	c.sweepMu.Unlock()
}

// getLabels looks up a memoized labelset, accounting a hit or a miss. Entry
// age is irrelevant here — a memoized result stays valid forever, so even a
// stale entry is served; age matters only to the sweep. A hit refreshes the
// entry idle timestamp. When the labels cache is disabled but the wrapper
// is alive, every lookup misses without accounting, mirroring the disabled
// regex cache.
func (c *labelReplaceCache) getLabels(key labelReplaceCacheKey) (labels.Labels, bool) {
	if c == nil || c.labelsCache == nil {
		return labels.EmptyLabels(), false
	}

	c.labelsMu.Lock()

	val, ok := c.labelsCache.Get(key)
	if !ok {
		c.labelsMu.Unlock()
		c.labelsMetrics.misses.Inc()

		return labels.EmptyLabels(), false
	}

	val.idleTs = c.nowMs.Load()
	result := val.result
	c.labelsMu.Unlock()

	c.labelsMetrics.hits.Inc()

	return result, true
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
		c.regexMetrics.hits.Inc()
		return re, nil
	}

	c.regexMetrics.misses.Inc()
	re, err := regexp.Compile("^(?:" + regexStr + ")$")
	if err != nil {
		return nil, err
	}

	c.regexCache.Add(regexStr, re)
	c.regexMetrics.entries.Set(float64(c.regexCache.Len()))

	return re, nil
}

// len returns the current number of labelset entries; it reports 0 when the
// labels cache is disabled and is nil-safe.
func (c *labelReplaceCache) len() int {
	if c == nil || c.labelsCache == nil {
		return 0
	}

	c.labelsMu.Lock()
	defer c.labelsMu.Unlock()

	return c.labelsCache.Len()
}

// maybeSweep schedules a background sweep when a sweep interval elapsed since
// the previous one. An in-progress sweep causes concurrent callers to return
// without waiting. Nil-safe: a nil or disabled cache never sweeps.
func (c *labelReplaceCache) maybeSweep() {
	if c == nil || c.labelsCache == nil {
		return
	}

	c.maybeSweepAt(time.Since(c.startedAt).Milliseconds())
}

// maybeSweepAt is maybeSweep with an injected monotonic clock, so tests stay
// deterministic without sleeping.
func (c *labelReplaceCache) maybeSweepAt(wallMs int64) {
	if c == nil || c.labelsCache == nil {
		return
	}

	current := c.nowMs.Load()
	if wallMs-current < labelReplaceCacheSweepIntervalMs {
		return
	}

	if !c.sweepMu.TryLock() {
		return
	}

	current = c.nowMs.Load()
	if c.sweepClosed || wallMs-current < labelReplaceCacheSweepIntervalMs {
		c.sweepMu.Unlock()
		return
	}

	c.nowMs.Store(wallMs)
	go func() {
		c.sweepOnce(wallMs)
		c.sweepMu.Unlock()
	}()
}

// onLabelsEvict clears the entry and counts an eviction under labelsMu.
// The caller updates the entries gauge after the complete cache mutation.
func (c *labelReplaceCache) onLabelsEvict(key labelReplaceCacheKey, entry *labelReplaceCacheEntry) {
	entry.result = labels.EmptyLabels()
	c.spareEntry = entry
	c.releaseOperationLocked(key.operation)
	c.labelsMetrics.evictions.Inc()
}

func (c *labelReplaceCache) releaseOperation(operation *labelReplaceOperation) {
	if c == nil || operation == nil {
		return
	}

	c.labelsMu.Lock()
	c.releaseOperationLocked(operation)
	c.labelsMu.Unlock()
}

// releaseOperationLocked drops a reference while labelsMu is held. An operation
// leaves the canonical table only when neither evaluations nor cache keys use it.
func (c *labelReplaceCache) releaseOperationLocked(operation *labelReplaceOperation) {
	operation.references--
	if operation.references != 0 {
		return
	}

	head := c.operations[operation.hash]
	if head == operation {
		if operation.next == nil {
			delete(c.operations, operation.hash)
		} else {
			c.operations[operation.hash] = operation.next
		}
	} else {
		for previous := head; previous != nil; previous = previous.next {
			if previous.next == operation {
				previous.next = operation.next

				break
			}
		}
	}

	operation.next = nil
}

// resize applies a labels-cache capacity change and updates the capacity
// gauge and the direction counter. The caller must hold labelsMu.
func (c *labelReplaceCache) resize(target int, resizes prometheus.Counter) {
	c.labelsCap = target
	c.labelsCache.Resize(target)
	c.labelsMetrics.capacity.Set(float64(target))
	resizes.Inc()
}

// sweepBatchLocked removes at most limit stale entries while labelsMu is held.
// The caller rechecks the oldest entry after every unlocked batch boundary.
func (c *labelReplaceCache) sweepBatchLocked(nowMs int64, limit int) int {
	freed := 0

	for range limit {
		_, val, ok := c.labelsCache.GetOldest()
		if !ok || nowMs-val.idleTs <= labelReplaceCacheStaleAfterMs {
			break
		}

		c.labelsCache.RemoveOldest()
		freed++
	}

	if freed > 0 {
		c.labelsMetrics.entries.Set(float64(c.labelsCache.Len()))
	}

	return freed
}

// sweepOnce removes stale entries in batches, yielding between batches so
// waiting lookups can refresh entries before the next oldest-entry check.
// Its removal budget is the initial length; resize uses the final live length.
// maybeSweepAt serializes complete passes with sweepMu.
func (c *labelReplaceCache) sweepOnce(nowMs int64) {
	if c == nil || c.labelsCache == nil {
		return
	}

	freed := 0

	c.labelsMu.Lock()
	remaining := c.labelsCache.Len()

	for remaining > 0 {
		limit := min(remaining, labelReplaceCacheSweepBatchSize)
		removed := c.sweepBatchLocked(nowMs, limit)
		freed += removed
		remaining -= removed

		if removed < limit || remaining == 0 {
			break
		}

		c.labelsMu.Unlock()
		runtime.Gosched()
		c.labelsMu.Lock()
	}

	length := c.labelsCache.Len()
	switch {
	// Still full after the sweep: the live set does not fit, grow along the
	// progressive schedule toward the configured maximum.
	case length == c.labelsCap && c.labelsCap < c.labelsMax:
		c.resize(min(labelReplaceCacheGrowTarget(c.labelsCap), c.labelsMax), c.growResizes)

	// Freed a quarter or more, or drifted below two thirds with progress:
	// shrink toward the survivors plus headroom, never below the floor.
	case freed >= c.labelsCap/labelReplaceCacheShrinkFreedDivisor ||
		(freed > 0 && length*3 < c.labelsCap*2):
		floor := max(labelReplaceCacheShrinkFloorMin, c.labelsMax/labelReplaceCacheShrinkFloorDivisor)
		if target := max(length+length/labelReplaceCacheShrinkTargetDivisor, floor); target < c.labelsCap {
			c.resize(target, c.shrinkResizes)
		}
	}

	c.labelsMu.Unlock()
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

// labelReplaceCacheShrinkFloor returns the capacity floor: an absolute 64
// entries or a hundredth of the configured maximum, whichever is larger.
func labelReplaceCacheShrinkFloor(maximum int) int {
	return max(labelReplaceCacheShrinkFloorMin, maximum/labelReplaceCacheShrinkFloorDivisor)
}
