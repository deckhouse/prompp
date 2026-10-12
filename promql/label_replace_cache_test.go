package promql

// Unit tests for the label_replace cache wrapper itself: hit/miss
// accounting, eviction, gauges, regex compilation and nil-safety. No Engine
// or query evaluation is involved; Engine wiring lives in
// label_replace_cache_engine_test.go, query behavior in
// label_replace_cache_query_test.go.

import (
	"strconv"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
)

type LabelReplaceCacheSuite struct {
	suite.Suite

	cache *labelReplaceCache
}

func TestLabelReplaceCache(t *testing.T) {
	suite.Run(t, new(LabelReplaceCacheSuite))
}

func (s *LabelReplaceCacheSuite) SetupTest() {
	s.cache = newLabelReplaceCache(10, 0)
}

func (s *LabelReplaceCacheSuite) TestCapacityGauge() {
	// Arrange: explicit labels size, default regex size.
	c := newLabelReplaceCache(10, 0)

	// Assert: the gauge exposes the configured capacity of each live cache.
	s.Equal(10.0, testutil.ToFloat64(c.capacity.WithLabelValues(labelReplaceCacheLabelsID)))
	s.Equal(
		float64(DefaultLabelReplaceRegexCacheSize),
		testutil.ToFloat64(c.capacity.WithLabelValues(labelReplaceCacheRegexID)),
	)
}

func (s *LabelReplaceCacheSuite) TestEntriesGauge() {
	// Arrange.
	entries := s.cache.entries.WithLabelValues(labelReplaceCacheLabelsID)
	s.Equal(0.0, testutil.ToFloat64(entries))

	// Act.
	for i := range 3 {
		s.cache.addLabels(labelReplaceTestKey(uint64(i)), labels.FromStrings("k", strconv.Itoa(i)))
	}

	// Assert.
	s.Equal(3, s.cache.len())
	s.Equal(3.0, testutil.ToFloat64(entries))
}

func (s *LabelReplaceCacheSuite) TestEviction() {
	// Arrange: capacity 1 forces the oldest entry out.
	s.cache = newLabelReplaceCache(1, 0)
	keyOldest := labelReplaceTestKey(1)
	keyNewest := labelReplaceTestKey(2)
	oldest := labels.FromStrings("k", "1")
	newest := labels.FromStrings("k", "2")

	// Act: adding the second key evicts the first one.
	s.cache.addLabels(keyOldest, oldest)
	s.cache.addLabels(keyNewest, newest)

	// Assert.
	s.Equal(1, s.cache.len())
	_, ok := s.cache.getLabels(keyOldest)
	s.False(ok, "oldest entry must be evicted")
	s.Equal(1.0, testutil.ToFloat64(s.cache.evictions.WithLabelValues(labelReplaceCacheLabelsID)))
	s.LessOrEqual(testutil.ToFloat64(s.cache.entries.WithLabelValues(labelReplaceCacheLabelsID)), 1.0)

	got, ok := s.cache.getLabels(keyNewest)
	s.True(ok, "newest entry must survive")
	s.True(labels.Equal(got.result, newest))
}

func (s *LabelReplaceCacheSuite) TestHitMiss() {
	// Arrange.
	key := labelReplaceTestKey(1)
	want := labels.FromStrings("__name__", "metric", "job", "api")

	// Act: first lookup misses on the empty cache, then the entry is stored.
	_, ok := s.cache.getLabels(key)
	s.cache.addLabels(key, want)
	got, hit := s.cache.getLabels(key)

	// Assert.
	s.False(ok, "empty cache must miss")
	s.True(hit, "stored key must hit")
	s.True(labels.Equal(got.result, want))
	s.Equal(want.Hash(), got.result.Hash())
	s.Equal(1, s.cache.len())
	s.Equal(1.0, testutil.ToFloat64(s.cache.hits.WithLabelValues(labelReplaceCacheLabelsID)))
	s.Equal(1.0, testutil.ToFloat64(s.cache.misses.WithLabelValues(labelReplaceCacheLabelsID)))
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepAtMostOncePerInterval() {
	// Arrange: a full cache of fresh entries below its maximum.
	c := newLabelReplaceCache(100, 0)
	s.Require().NotNil(c)
	for i := range 64 {
		c.addLabels(labelReplaceTestKey(uint64(i)), labels.FromStrings("k", strconv.Itoa(i)))
	}

	// Act: two sweep attempts within one interval.
	c.maybeSweepAt(70_000)
	c.maybeSweepAt(70_000)

	// Assert: exactly one sweep ran and grew the capacity.
	s.Equal(100, c.labelsCap)
	s.Equal(1.0, testutil.ToFloat64(c.resizes.WithLabelValues(labelReplaceCacheGrowDirection)))

	// Act: a throttled attempt changes nothing.
	c.maybeSweepAt(70_001)

	// Assert.
	s.Equal(100, c.labelsCap)
	s.Equal(1.0, testutil.ToFloat64(c.resizes.WithLabelValues(labelReplaceCacheGrowDirection)))

	// Act: a nil wrapper never sweeps.
	var nilCache *labelReplaceCache
	nilCache.maybeSweepAt(70_000)

	// Assert: no panic, nothing to assert beyond survival.
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepGrowsWhenFull() {
	// Arrange: a full cache of fresh entries below its maximum.
	c := newLabelReplaceCache(100, 0)
	s.Require().NotNil(c)
	c.nowMs.Store(0)
	for i := range 64 {
		c.addLabels(labelReplaceTestKey(uint64(i)), labels.FromStrings("k", strconv.Itoa(i)))
	}

	// Act: a sweep over an all-fresh full cache.
	c.sweepOnce(0)

	// Assert: the capacity doubles toward the maximum.
	s.Equal(100, c.labelsCap)
	s.Equal(100.0, testutil.ToFloat64(c.capacity.WithLabelValues(labelReplaceCacheLabelsID)))
	s.Equal(1.0, testutil.ToFloat64(c.resizes.WithLabelValues(labelReplaceCacheGrowDirection)))
	s.Equal(0.0, testutil.ToFloat64(c.resizes.WithLabelValues(labelReplaceCacheShrinkDirection)))
	s.Equal(64, c.len())
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepInitialCapacity() {
	// Arrange: a configured maximum starts at the shrink floor.
	c := newLabelReplaceCache(1000, 0)
	s.Require().NotNil(c)

	// Assert: the wrapper records the floor while keeping the maximum.
	s.Equal(1000, c.labelsMax)
	s.Equal(64, c.labelsCap)
	s.Equal(64.0, testutil.ToFloat64(c.capacity.WithLabelValues(labelReplaceCacheLabelsID)))

	// Act: the LRU is built at the floor capacity — the 65th entry evicts.
	for i := range 65 {
		c.addLabels(labelReplaceTestKey(uint64(i)), labels.FromStrings("k", strconv.Itoa(i)))
	}

	// Assert.
	s.Equal(64, c.len())
	s.Equal(1.0, testutil.ToFloat64(c.evictions.WithLabelValues(labelReplaceCacheLabelsID)))
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepRefreshesIdleTsOnHit() {
	// Arrange: a wrapper whose clock sits at 0 with one stored entry.
	c := newLabelReplaceCache(1000, 0)
	s.Require().NotNil(c)
	key := labelReplaceTestKey(1)
	c.addLabels(key, labels.FromStrings("k", "v"))

	// Act: a hit at a later cached timestamp refreshes the idle time.
	c.nowMs.Store(100_000)
	got, hit := c.getLabels(key)

	// Assert.
	s.Require().True(hit)
	s.Equal(int64(100_000), got.idleTs.Load())
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepRemovesStaleFromOldest() {
	// Arrange: a full cache whose oldest quarter went stale; the rest was re-hit.
	c := newLabelReplaceCache(100, 0)
	s.Require().NotNil(c)
	for i := range 64 {
		c.addLabels(labelReplaceTestKey(uint64(i)), labels.FromStrings("k", strconv.Itoa(i)))
	}
	c.nowMs.Store(200_000)
	for i := 16; i < 64; i++ {
		_, ok := c.getLabels(labelReplaceTestKey(uint64(i)))
		s.Require().True(ok)
	}

	// Act: a sweep removes the stale prefix and stops at the first fresh entry.
	c.sweepOnce(200_000)

	// Assert: only the stale prefix is gone; the freed quarter is not enough
	// to shrink below the floor, so the capacity is untouched.
	s.Equal(48, c.len())
	s.Equal(16.0, testutil.ToFloat64(c.evictions.WithLabelValues(labelReplaceCacheLabelsID)))
	for i := 16; i < 64; i++ {
		_, ok := c.getLabels(labelReplaceTestKey(uint64(i)))
		s.True(ok, "fresh entries must survive the sweep")
	}
	s.Equal(64, c.labelsCap)
	s.Equal(0.0, testutil.ToFloat64(c.resizes.WithLabelValues(labelReplaceCacheGrowDirection)))
	s.Equal(0.0, testutil.ToFloat64(c.resizes.WithLabelValues(labelReplaceCacheShrinkDirection)))
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepShrinksOnDrift() {
	// Arrange: capacity arranged at 200, draining ten entries per sweep.
	c := newLabelReplaceCache(1000, 0)
	s.Require().NotNil(c)
	c.labelsCap = 200
	c.labelsCache.Resize(200)
	c.capacity.WithLabelValues(labelReplaceCacheLabelsID).Set(200)
	for i := range 200 {
		c.addLabels(labelReplaceTestKey(uint64(i)), labels.FromStrings("k", strconv.Itoa(i)))
	}

	// Act: seven sweeps two ticks apart, survivors re-hit each time so no
	// single sweep frees a quarter.
	for round := range 7 {
		c.nowMs.Store(int64(200_000 + round*200_000))
		for i := 10 * (round + 1); i < 200; i++ {
			_, ok := c.getLabels(labelReplaceTestKey(uint64(i)))
			s.Require().True(ok)
		}
		c.sweepOnce(c.nowMs.Load())
	}

	// Assert: the slow drain shrinks once the survivors drop below two
	// thirds of the capacity.
	s.Equal(130, c.len())
	s.Equal(143, c.labelsCap)
	s.Equal(143.0, testutil.ToFloat64(c.capacity.WithLabelValues(labelReplaceCacheLabelsID)))
	s.Equal(1.0, testutil.ToFloat64(c.resizes.WithLabelValues(labelReplaceCacheShrinkDirection)))
	s.Equal(0.0, testutil.ToFloat64(c.resizes.WithLabelValues(labelReplaceCacheGrowDirection)))
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepShrinksWhenQuarterFreed() {
	// Arrange: capacity arranged at 200, its oldest half went stale.
	c := newLabelReplaceCache(1000, 0)
	s.Require().NotNil(c)
	c.labelsCap = 200
	c.labelsCache.Resize(200)
	c.capacity.WithLabelValues(labelReplaceCacheLabelsID).Set(200)
	for i := range 200 {
		c.addLabels(labelReplaceTestKey(uint64(i)), labels.FromStrings("k", strconv.Itoa(i)))
	}
	c.nowMs.Store(200_000)
	for i := 100; i < 200; i++ {
		_, ok := c.getLabels(labelReplaceTestKey(uint64(i)))
		s.Require().True(ok)
	}

	// Act: the sweep frees 100 entries, a quarter of the capacity or more.
	c.sweepOnce(200_000)

	// Assert: the capacity shrinks toward the surviving count plus headroom.
	s.Equal(100, c.len())
	s.Equal(110, c.labelsCap)
	s.Equal(110.0, testutil.ToFloat64(c.capacity.WithLabelValues(labelReplaceCacheLabelsID)))
	s.Equal(1.0, testutil.ToFloat64(c.resizes.WithLabelValues(labelReplaceCacheShrinkDirection)))
	s.Equal(0.0, testutil.ToFloat64(c.resizes.WithLabelValues(labelReplaceCacheGrowDirection)))
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepStaleEntryStillServed() {
	// Arrange: one stored entry, the clock far past the stale threshold.
	c := newLabelReplaceCache(10, 0)
	s.Require().NotNil(c)
	key := labelReplaceTestKey(1)
	want := labels.FromStrings("k", "v")
	c.addLabels(key, want)
	c.nowMs.Store(200_000)

	// Act: a lookup of the stale entry.
	got, hit := c.getLabels(key)

	// Assert: a stale entry is still a valid hit — age matters only to the
	// sweep, never to serving.
	s.True(hit, "a stale entry must still be served")
	s.True(labels.Equal(want, got.result))
	s.Equal(int64(200_000), got.idleTs.Load())
	s.Equal(1, c.len())
	s.Equal(1.0, testutil.ToFloat64(c.hits.WithLabelValues(labelReplaceCacheLabelsID)))
	s.Equal(0.0, testutil.ToFloat64(c.misses.WithLabelValues(labelReplaceCacheLabelsID)))
	s.Equal(0.0, testutil.ToFloat64(c.evictions.WithLabelValues(labelReplaceCacheLabelsID)))
}

func (s *LabelReplaceCacheSuite) TestLabelsCacheDisabledRegexEnabled() {
	// Arrange: the labels cache is off, only the regex cache is on.
	c := newLabelReplaceCache(-1, 10)
	s.Require().NotNil(c)
	s.Require().Nil(c.labelsCache)
	key := labelReplaceTestKey(1)

	// Act: a lookup always misses and storing is a noop.
	_, hitFirst := c.getLabels(key)
	c.addLabels(key, labels.FromStrings("k", "v"))
	_, hitAfterAdd := c.getLabels(key)

	// Assert: nothing is stored and no labels metric is ever accounted.
	s.False(hitFirst, "disabled labels cache must always miss")
	s.False(hitAfterAdd, "addLabels must be a noop when the labels cache is off")
	s.Equal(0, c.len())
	s.Equal(0.0, testutil.ToFloat64(c.hits.WithLabelValues(labelReplaceCacheLabelsID)))
	s.Equal(0.0, testutil.ToFloat64(c.misses.WithLabelValues(labelReplaceCacheLabelsID)))
	s.Equal(0.0, testutil.ToFloat64(c.evictions.WithLabelValues(labelReplaceCacheLabelsID)))
	s.Equal(0.0, testutil.ToFloat64(c.entries.WithLabelValues(labelReplaceCacheLabelsID)))

	// Act: the regex cache stays live, the second identical call hits.
	re1, err := c.getOrCompileRegex("(.+)")
	s.Require().NoError(err)
	re2, err := c.getOrCompileRegex("(.+)")

	// Assert: same text yields the identical compiled regex.
	s.Require().NoError(err)
	s.Same(re1, re2)
	s.Equal(1.0, testutil.ToFloat64(c.hits.WithLabelValues(labelReplaceCacheRegexID)))
	s.Equal(1.0, testutil.ToFloat64(c.entries.WithLabelValues(labelReplaceCacheRegexID)))
}

func (s *LabelReplaceCacheSuite) TestNil() {
	// Arrange: a nil wrapper means the cache is disabled.
	var c *labelReplaceCache
	key := labelReplaceTestKey(1)

	// Act and assert: every method is nil-safe.
	_, ok := c.getLabels(key)
	s.False(ok, "nil cache must always miss")

	c.addLabels(key, labels.FromStrings("k", "v"))
	s.Equal(0, c.len())

	re, err := c.getOrCompileRegex("a+")
	s.Require().NoError(err)
	s.True(re.MatchString("aaa"))
	s.False(re.MatchString("b"))

	c.register(nil)
	reg := prometheus.NewRegistry()
	c.register(reg)
	mfs, err := reg.Gather()
	s.Require().NoError(err)
	s.Empty(mfs, "nil cache must not register any metrics")
}

func (s *LabelReplaceCacheSuite) TestRegex() {
	// Arrange.
	hits := s.cache.hits.WithLabelValues(labelReplaceCacheRegexID)
	misses := s.cache.misses.WithLabelValues(labelReplaceCacheRegexID)
	entries := s.cache.entries.WithLabelValues(labelReplaceCacheRegexID)

	// Act: first call compiles, the second one is served from the cache.
	re1, err := s.cache.getOrCompileRegex("(.+)")
	s.Require().NoError(err)
	re2, err := s.cache.getOrCompileRegex("(.+)")

	// Assert: same text yields the identical compiled regex.
	s.Require().NoError(err)
	s.Same(re1, re2)
	s.Equal(1.0, testutil.ToFloat64(misses))
	s.Equal(1.0, testutil.ToFloat64(hits))
	s.Equal(1.0, testutil.ToFloat64(entries))

	// Act: an invalid regex fails twice and never grows the cache.
	_, err = s.cache.getOrCompileRegex("a(")
	s.Require().Error(err)
	_, err = s.cache.getOrCompileRegex("a(")
	s.Require().Error(err)

	// Assert.
	s.Equal(3.0, testutil.ToFloat64(misses))
	s.Equal(1.0, testutil.ToFloat64(hits))
	s.Equal(1.0, testutil.ToFloat64(entries))
	s.Equal(0.0, testutil.ToFloat64(s.cache.evictions.WithLabelValues(labelReplaceCacheRegexID)))
}

func (s *LabelReplaceCacheSuite) TestRegexCacheDisabled() {
	// Arrange: the labels cache is on, only the regex cache is off.
	c := newLabelReplaceCache(10, -1)
	s.Require().NotNil(c)
	s.Require().Nil(c.regexCache)

	// Act: a valid regex still compiles and matches, an invalid one still errors.
	re, err := c.getOrCompileRegex("a+")
	s.Require().NoError(err)
	s.True(re.MatchString("aaa"))
	s.False(re.MatchString("b"))
	_, err = c.getOrCompileRegex("a(")
	s.Require().Error(err)

	// Assert: nothing is stored and no regex metric is ever accounted.
	s.Equal(0.0, testutil.ToFloat64(c.hits.WithLabelValues(labelReplaceCacheRegexID)))
	s.Equal(0.0, testutil.ToFloat64(c.misses.WithLabelValues(labelReplaceCacheRegexID)))
	s.Equal(0.0, testutil.ToFloat64(c.evictions.WithLabelValues(labelReplaceCacheRegexID)))
	s.Equal(0.0, testutil.ToFloat64(c.entries.WithLabelValues(labelReplaceCacheRegexID)))
}

func (s *LabelReplaceCacheSuite) TestRegexCacheEviction() {
	// Arrange: regex capacity 1 forces the oldest compiled regex out.
	c := newLabelReplaceCache(10, 1)
	s.Require().NotNil(c)

	// Act: two distinct regexes exceed the configured capacity.
	_, err := c.getOrCompileRegex("a+")
	s.Require().NoError(err)
	_, err = c.getOrCompileRegex("b+")
	s.Require().NoError(err)

	// Assert.
	s.Equal(1.0, testutil.ToFloat64(c.evictions.WithLabelValues(labelReplaceCacheRegexID)))
	s.Equal(1.0, testutil.ToFloat64(c.entries.WithLabelValues(labelReplaceCacheRegexID)))
}

// labelReplaceTestKey builds a distinct cache key per baseHash so tests never
// put labels.Labels into a map key.
func labelReplaceTestKey(baseHash uint64) labelReplaceCacheKey {
	return labelReplaceCacheKey{
		dst:      "exported_job",
		repl:     "$1",
		src:      "job",
		regexStr: "(.*)",
		srcVal:   "job",
		baseHash: baseHash,
	}
}
