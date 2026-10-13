package promql

// Unit tests for the label_replace cache wrapper itself: hit/miss
// accounting, eviction, gauges, regex compilation and nil-safety. No Engine
// or query evaluation is involved; Engine wiring lives in
// label_replace_cache_engine_test.go, query behavior in
// label_replace_cache_query_test.go.

import (
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/golang-lru/v2/simplelru"
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
	s.cache = newLabelReplaceCache(10, 0, nil)
}

func (s *LabelReplaceCacheSuite) TestCapacityGauge() {
	// Arrange: explicit labels size, default regex size.
	c := newLabelReplaceCache(10, 0, nil)

	// Assert: the gauge exposes the configured capacity of each live cache.
	s.Equal(10.0, testutil.ToFloat64(c.labelsMetrics.capacity))
	s.Equal(
		float64(DefaultLabelReplaceRegexCacheSize),
		testutil.ToFloat64(c.regexMetrics.capacity),
	)
}

func (s *LabelReplaceCacheSuite) TestConcurrentLabelsRegexAndSweep() {
	// Arrange.
	c := newLabelReplaceCache(1000, 8, nil)
	var clock atomic.Int64
	var workers sync.WaitGroup
	failures := make(chan error, 16)
	start := make(chan struct{})

	// Act.
	for worker := range 16 {
		workers.Go(func() {
			<-start
			for iteration := range 200 {
				id := uint64(worker*16 + iteration%16)
				key := labelReplaceTestKey(id)
				want := labels.FromStrings("k", strconv.FormatUint(id, 10))
				c.addLabels(key, want)
				if got, hit := c.getLabels(key); hit && !labels.Equal(want, got) {
					failures <- fmt.Errorf("wrong labels for key %d", id)

					return
				}

				if _, err := c.getOrCompileRegex("(.*)"); err != nil {
					failures <- err

					return
				}

				c.maybeSweepAt(clock.Add(60_000))
			}
		})
	}

	close(start)
	workers.Wait()
	close(failures)

	// Assert.
	for err := range failures {
		s.NoError(err)
	}

	s.LessOrEqual(c.len(), c.labelsCap)
	s.LessOrEqual(c.labelsCap, c.labelsMax)
	s.Equal(float64(c.len()), testutil.ToFloat64(c.labelsMetrics.entries))
	s.Equal(float64(c.labelsCap), testutil.ToFloat64(c.labelsMetrics.capacity))
}

func (s *LabelReplaceCacheSuite) TestDisabledCacheDoesNotRegisterMetrics() {
	// Arrange.
	reg := prometheus.NewRegistry()

	// Act.
	c := newLabelReplaceCache(-1, -1, reg)
	metrics, err := reg.Gather()

	// Assert.
	s.Nil(c)
	s.Require().NoError(err)
	s.Empty(metrics)
}

func (s *LabelReplaceCacheSuite) TestEntriesGauge() {
	// Arrange.
	entries := s.cache.labelsMetrics.entries
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
	s.cache = newLabelReplaceCache(1, 0, nil)
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
	s.Equal(1.0, testutil.ToFloat64(s.cache.labelsMetrics.evictions))
	s.LessOrEqual(testutil.ToFloat64(s.cache.labelsMetrics.entries), 1.0)

	got, ok := s.cache.getLabels(keyNewest)
	s.True(ok, "newest entry must survive")
	s.True(labels.Equal(got, newest))
}

func (s *LabelReplaceCacheSuite) TestEvictionPreservesReturnedLabels() {
	// Arrange: capacity one forces the first entry to be reused by the third insert.
	c := newLabelReplaceCache(1, 0, nil)
	firstKey := labelReplaceTestKey(1)
	firstLabels := labels.FromStrings("k", "first")
	c.addLabels(firstKey, firstLabels)
	got, hit := c.getLabels(firstKey)
	s.Require().True(hit)

	// Act.
	c.addLabels(labelReplaceTestKey(2), labels.FromStrings("k", "second"))
	c.addLabels(labelReplaceTestKey(3), labels.FromStrings("k", "third"))
	third, thirdHit := c.getLabels(labelReplaceTestKey(3))

	// Assert: a previously returned labelset stays immutable after entry reuse.
	s.True(labels.Equal(firstLabels, got))
	s.Require().True(thirdHit)
	s.Equal("third", third.Get("k"))
	s.Equal(2.0, testutil.ToFloat64(c.labelsMetrics.evictions))
	s.Require().NotNil(c.spareEntry)
	s.True(c.spareEntry.result.IsEmpty(), "the spare entry must not retain an evicted labelset")
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
	s.True(labels.Equal(got, want))
	s.Equal(want.Hash(), got.Hash())
	s.Equal(1, s.cache.len())
	s.Equal(1.0, testutil.ToFloat64(s.cache.labelsMetrics.hits))
	s.Equal(1.0, testutil.ToFloat64(s.cache.labelsMetrics.misses))
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepAtMostOncePerInterval() {
	// Arrange: a full cache of fresh entries below its maximum.
	c := newLabelReplaceCache(100, 0, nil)
	s.Require().NotNil(c)
	for i := range 64 {
		c.addLabels(labelReplaceTestKey(uint64(i)), labels.FromStrings("k", strconv.Itoa(i)))
	}

	// Act: two sweep attempts within one interval.
	c.maybeSweepAt(70_000)
	c.maybeSweepAt(70_000)

	// Assert: exactly one sweep ran and grew the capacity.
	s.Equal(100, c.labelsCap)
	s.Equal(1.0, testutil.ToFloat64(c.growResizes))

	// Act: a throttled attempt changes nothing.
	c.maybeSweepAt(70_001)

	// Assert.
	s.Equal(100, c.labelsCap)
	s.Equal(1.0, testutil.ToFloat64(c.growResizes))

	// Act: a nil wrapper never sweeps.
	var nilCache *labelReplaceCache
	nilCache.maybeSweepAt(70_000)

	// Assert: no panic, nothing to assert beyond survival.
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepDoesNotOverlapIntervals() {
	// Arrange: block the first sweep after it has acquired the sweep guard.
	c := newLabelReplaceCache(100, 0, nil)
	for i := range 64 {
		c.addLabels(labelReplaceTestKey(uint64(i)), labels.FromStrings("k", strconv.Itoa(i)))
	}

	c.labelsMu.Lock()
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.maybeSweepAt(60_000)
	}()

	func() {
		defer c.labelsMu.Unlock()
		s.Require().Eventually(func() bool { return c.nowMs.Load() == 60_000 }, time.Second, time.Millisecond)

		// Act: another interval elapses while the first sweep is still running.
		c.maybeSweepAt(120_000)

		// Assert: the second caller leaves the clock and capacity unchanged.
		s.Equal(int64(60_000), c.nowMs.Load())
		s.Equal(64, c.labelsCap)
	}()
	<-done

	s.Equal(100, c.labelsCap)
	s.Equal(1.0, testutil.ToFloat64(c.growResizes))
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepGrowsProgressively() {
	// Arrange: a full cache at a scheduled step.
	c := newLabelReplaceCache(10000, 0, nil)
	s.Require().NotNil(c)
	c.labelsCap = 512
	c.labelsCache.Resize(512)
	c.labelsMetrics.capacity.Set(512)
	for i := range 512 {
		c.addLabels(labelReplaceTestKey(uint64(i)), labels.FromStrings("k", strconv.Itoa(i)))
	}

	// Act: a sweep over an all-fresh full cache.
	c.sweepOnce(0)

	// Assert: the scheduled multiplier applies instead of doubling.
	s.Equal(4096, c.labelsCap)
	s.Equal(4096.0, testutil.ToFloat64(c.labelsMetrics.capacity))
	s.Equal(1.0, testutil.ToFloat64(c.growResizes))

	// Arrange: an off-schedule capacity takes the catch-all multiplier.
	d := newLabelReplaceCache(10000, 0, nil)
	s.Require().NotNil(d)
	d.labelsCap = 300
	d.labelsCache.Resize(300)
	d.labelsMetrics.capacity.Set(300)
	for i := range 300 {
		d.addLabels(labelReplaceTestKey(uint64(i)), labels.FromStrings("k", strconv.Itoa(i)))
	}

	// Act.
	d.sweepOnce(0)

	// Assert.
	s.Equal(2400, d.labelsCap)
	s.Equal(2400.0, testutil.ToFloat64(d.labelsMetrics.capacity))
	s.Equal(1.0, testutil.ToFloat64(d.growResizes))
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepGrowsWhenFull() {
	// Arrange: a full cache of fresh entries below its maximum.
	c := newLabelReplaceCache(100, 0, nil)
	s.Require().NotNil(c)
	c.nowMs.Store(0)
	for i := range 64 {
		c.addLabels(labelReplaceTestKey(uint64(i)), labels.FromStrings("k", strconv.Itoa(i)))
	}

	// Act: a sweep over an all-fresh full cache.
	c.sweepOnce(0)

	// Assert: the capacity doubles toward the maximum.
	s.Equal(100, c.labelsCap)
	s.Equal(100.0, testutil.ToFloat64(c.labelsMetrics.capacity))
	s.Equal(1.0, testutil.ToFloat64(c.growResizes))
	s.Equal(0.0, testutil.ToFloat64(c.shrinkResizes))
	s.Equal(64, c.len())
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepInitialCapacity() {
	// Arrange: a configured maximum starts at the shrink floor.
	c := newLabelReplaceCache(1000, 0, nil)
	s.Require().NotNil(c)

	// Assert: the wrapper records the floor while keeping the maximum.
	s.Equal(1000, c.labelsMax)
	s.Equal(64, c.labelsCap)
	s.Equal(64.0, testutil.ToFloat64(c.labelsMetrics.capacity))

	// Act: the LRU is built at the floor capacity — the 65th entry evicts.
	for i := range 65 {
		c.addLabels(labelReplaceTestKey(uint64(i)), labels.FromStrings("k", strconv.Itoa(i)))
	}

	// Assert.
	s.Equal(64, c.len())
	s.Equal(1.0, testutil.ToFloat64(c.labelsMetrics.evictions))
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepRefreshesIdleTsOnHit() {
	// Arrange: a wrapper whose clock sits at 0 with one stored entry.
	c := newLabelReplaceCache(1000, 0, nil)
	s.Require().NotNil(c)
	key := labelReplaceTestKey(1)
	c.addLabels(key, labels.FromStrings("k", "v"))

	// Act: a hit at a later cached timestamp refreshes the idle time.
	c.nowMs.Store(100_000)
	_, hit := c.getLabels(key)

	// Assert.
	s.Require().True(hit)
	entry, _ := c.labelsCache.Peek(key)
	s.Equal(int64(100_000), entry.idleTs)
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepRemovesStaleFromOldest() {
	// Arrange: a full cache whose oldest quarter went stale; the rest was re-hit.
	c := newLabelReplaceCache(100, 0, nil)
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
	s.Equal(16.0, testutil.ToFloat64(c.labelsMetrics.evictions))
	for i := 16; i < 64; i++ {
		_, ok := c.getLabels(labelReplaceTestKey(uint64(i)))
		s.True(ok, "fresh entries must survive the sweep")
	}
	s.Equal(64, c.labelsCap)
	s.Equal(0.0, testutil.ToFloat64(c.growResizes))
	s.Equal(0.0, testutil.ToFloat64(c.shrinkResizes))
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepPreservesReturnedLabels() {
	// Arrange.
	c := newLabelReplaceCache(10, 0, nil)
	key := labelReplaceTestKey(1)
	want := labels.FromStrings("k", "original")
	c.addLabels(key, want)
	got, hit := c.getLabels(key)
	s.Require().True(hit)

	// Act: sweep the entry, then reuse it for a different series.
	c.nowMs.Store(200_000)
	c.sweepOnce(200_000)
	c.addLabels(labelReplaceTestKey(2), labels.FromStrings("k", "replacement"))

	// Assert.
	s.True(labels.Equal(want, got))
	_, oldHit := c.getLabels(key)
	s.False(oldHit)
	s.Equal(1, c.len())
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepSerializesRefreshWithRemoval() {
	// Arrange: the first eviction pauses the sweep while another query wants
	// to refresh the next stale entry. The fresh tail must survive both operations.
	c := newLabelReplaceCache(100, 0, nil)
	oldest := labelReplaceTestKey(1)
	next := labelReplaceTestKey(2)
	fresh := labelReplaceTestKey(3)
	c.addLabels(oldest, labels.FromStrings("k", "oldest"))
	c.addLabels(next, labels.FromStrings("k", "next"))
	c.nowMs.Store(200_000)
	c.addLabels(fresh, labels.FromStrings("k", "fresh"))

	// The callback executes under labelsMu and gives a deterministic point
	// inside the sweep at which the competing lookup starts.
	evicted := make(chan struct{})
	resume := make(chan struct{})
	cache, err := simplelru.NewLRU(c.labelsCap, func(key labelReplaceCacheKey, entry *labelReplaceCacheEntry) {
		c.onLabelsEvict(key, entry)
		if key == oldest {
			close(evicted)
			<-resume
		}
	})
	s.Require().NoError(err)
	for _, key := range []labelReplaceCacheKey{oldest, next, fresh} {
		entry, _ := c.labelsCache.Peek(key)
		cache.Add(key, entry)
	}

	c.labelsCache = cache
	sweepDone := make(chan struct{})
	go func() {
		defer close(sweepDone)
		c.sweepOnce(200_000)
	}()
	<-evicted

	// Act.
	lookupDone := make(chan bool, 1)
	lookupStarted := make(chan struct{})
	go func() {
		close(lookupStarted)
		_, hit := c.getLabels(next)
		lookupDone <- hit
	}()
	<-lookupStarted
	s.Never(func() bool { return len(lookupDone) != 0 }, 20*time.Millisecond, time.Millisecond)
	close(resume)
	<-sweepDone
	hit := <-lookupDone

	// Assert: the sweep finishes before the lookup, which misses the removed
	// entry; it never evicts the fresh tail in place of a refreshed stale entry.
	s.False(hit)
	_, freshHit := c.getLabels(fresh)
	s.True(freshHit)
	s.Equal(1, c.len())
	s.Equal(2.0, testutil.ToFloat64(c.labelsMetrics.evictions))
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepShrinksOnDrift() {
	// Arrange: capacity arranged at 200, draining ten entries per sweep.
	c := newLabelReplaceCache(1000, 0, nil)
	s.Require().NotNil(c)
	c.labelsCap = 200
	c.labelsCache.Resize(200)
	c.labelsMetrics.capacity.Set(200)
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
	s.Equal(143.0, testutil.ToFloat64(c.labelsMetrics.capacity))
	s.Equal(1.0, testutil.ToFloat64(c.shrinkResizes))
	s.Equal(0.0, testutil.ToFloat64(c.growResizes))
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepShrinksWhenQuarterFreed() {
	// Arrange: capacity arranged at 200, its oldest half went stale.
	c := newLabelReplaceCache(1000, 0, nil)
	s.Require().NotNil(c)
	c.labelsCap = 200
	c.labelsCache.Resize(200)
	c.labelsMetrics.capacity.Set(200)
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
	s.Equal(110.0, testutil.ToFloat64(c.labelsMetrics.capacity))
	s.Equal(1.0, testutil.ToFloat64(c.shrinkResizes))
	s.Equal(0.0, testutil.ToFloat64(c.growResizes))
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepStaleEntryStillServed() {
	// Arrange: one stored entry, the clock far past the stale threshold.
	c := newLabelReplaceCache(10, 0, nil)
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
	s.True(labels.Equal(want, got))
	entry, _ := c.labelsCache.Peek(key)
	s.Equal(int64(200_000), entry.idleTs)
	s.Equal(1, c.len())
	s.Equal(1.0, testutil.ToFloat64(c.labelsMetrics.hits))
	s.Equal(0.0, testutil.ToFloat64(c.labelsMetrics.misses))
	s.Equal(0.0, testutil.ToFloat64(c.labelsMetrics.evictions))
}

func (s *LabelReplaceCacheSuite) TestLabelsCacheDisabledRegexEnabled() {
	// Arrange: the labels cache is off, only the regex cache is on.
	c := newLabelReplaceCache(-1, 10, nil)
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
	s.Nil(c.labelsMetrics.hits)
	s.Nil(c.labelsMetrics.misses)
	s.Nil(c.labelsMetrics.evictions)
	s.Nil(c.labelsMetrics.entries)

	// Act: the regex cache stays live, the second identical call hits.
	re1, err := c.getOrCompileRegex("(.+)")
	s.Require().NoError(err)
	re2, err := c.getOrCompileRegex("(.+)")

	// Assert: same text yields the identical compiled regex.
	s.Require().NoError(err)
	s.Same(re1, re2)
	s.Equal(1.0, testutil.ToFloat64(c.regexMetrics.hits))
	s.Equal(1.0, testutil.ToFloat64(c.regexMetrics.entries))
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
}

func (s *LabelReplaceCacheSuite) TestRegex() {
	// Arrange.
	hits := s.cache.regexMetrics.hits
	misses := s.cache.regexMetrics.misses
	entries := s.cache.regexMetrics.entries

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
	s.Equal(0.0, testutil.ToFloat64(s.cache.regexMetrics.evictions))
}

func (s *LabelReplaceCacheSuite) TestRegexCacheDisabled() {
	// Arrange: the labels cache is on, only the regex cache is off.
	c := newLabelReplaceCache(10, -1, nil)
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
	s.Nil(c.regexMetrics.hits)
	s.Nil(c.regexMetrics.misses)
	s.Nil(c.regexMetrics.evictions)
	s.Nil(c.regexMetrics.entries)
}

func (s *LabelReplaceCacheSuite) TestRegexCacheEviction() {
	// Arrange: regex capacity 1 forces the oldest compiled regex out.
	c := newLabelReplaceCache(10, 1, nil)
	s.Require().NotNil(c)

	// Act: two distinct regexes exceed the configured capacity.
	_, err := c.getOrCompileRegex("a+")
	s.Require().NoError(err)
	_, err = c.getOrCompileRegex("b+")
	s.Require().NoError(err)

	// Assert.
	s.Equal(1.0, testutil.ToFloat64(c.regexMetrics.evictions))
	s.Equal(1.0, testutil.ToFloat64(c.regexMetrics.entries))
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
