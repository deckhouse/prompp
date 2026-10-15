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
	"unsafe"

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
	s.T().Cleanup(s.cache.close)
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

func (s *LabelReplaceCacheSuite) TestCloseIsIdempotentAndPreventsSweep() {
	// Arrange.
	c := s.cache
	c.addLabels(labelReplaceTestKey(s.T(), c, 1), labels.FromStrings("k", "value"), true)

	// Act.
	c.close()
	c.close()
	c.maybeSweepAt(200_000)
	var nilCache *labelReplaceCache
	nilCache.close()

	// Assert: no new sweep was admitted and the clock did not advance.
	s.Equal(int64(0), c.nowMs.Load())
	s.Equal(1, c.len())
	s.Equal(0.0, testutil.ToFloat64(c.labelsMetrics.evictions))
}

func (s *LabelReplaceCacheSuite) TestConcurrentCloseAndSweepScheduling() {
	// Arrange.
	c := s.cache
	var workers sync.WaitGroup
	var clock atomic.Int64
	start := make(chan struct{})

	// Act.
	for range 8 {
		workers.Go(func() {
			<-start
			for range 100 {
				c.maybeSweepAt(clock.Add(labelReplaceCacheSweepIntervalMs))
			}
		})
	}

	for range 4 {
		workers.Go(func() {
			<-start
			c.close()
		})
	}

	close(start)
	workers.Wait()
	closedAt := c.nowMs.Load()
	c.maybeSweepAt(closedAt + labelReplaceCacheSweepIntervalMs)

	// Assert: completed close calls prevent any further background work.
	s.Equal(closedAt, c.nowMs.Load())
	s.True(c.sweepClosed)
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
				key := labelReplaceTestKey(s.T(), c, id)
				want := labels.FromStrings("k", strconv.FormatUint(id, 10))
				c.addLabels(key, want, true)
				if got, hit := c.getLabels(key); hit && !labels.Equal(want, got.result) {
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
	c.close()
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
		s.cache.addLabels(labelReplaceTestKey(s.T(), s.cache, uint64(i)), labels.FromStrings("k", strconv.Itoa(i)), true)
	}

	// Assert.
	s.Equal(3, s.cache.len())
	s.Equal(3.0, testutil.ToFloat64(entries))
}

func (s *LabelReplaceCacheSuite) TestEviction() {
	// Arrange: capacity 1 forces the oldest entry out.
	s.cache = newLabelReplaceCache(1, 0, nil)
	keyOldest := labelReplaceTestKey(s.T(), s.cache, 1)
	keyNewest := labelReplaceTestKey(s.T(), s.cache, 2)
	oldest := labels.FromStrings("k", "1")
	newest := labels.FromStrings("k", "2")

	// Act: adding the second key evicts the first one.
	s.cache.addLabels(keyOldest, oldest, true)
	s.cache.addLabels(keyNewest, newest, true)

	// Assert.
	s.Equal(1, s.cache.len())
	_, ok := s.cache.getLabels(keyOldest)
	s.False(ok, "oldest entry must be evicted")
	s.Equal(1.0, testutil.ToFloat64(s.cache.labelsMetrics.evictions))
	s.LessOrEqual(testutil.ToFloat64(s.cache.labelsMetrics.entries), 1.0)

	got, ok := s.cache.getLabels(keyNewest)
	s.True(ok, "newest entry must survive")
	s.True(labels.Equal(got.result, newest))
}

func (s *LabelReplaceCacheSuite) TestEvictionPreservesReturnedLabels() {
	// Arrange: capacity one forces the first entry to be reused by the third insert.
	c := newLabelReplaceCache(1, 0, nil)
	firstKey := labelReplaceTestKey(s.T(), c, 1)
	firstLabels := labels.FromStrings("k", "first")
	c.addLabels(firstKey, firstLabels, true)
	got, hit := c.getLabels(firstKey)
	s.Require().True(hit)

	// Act.
	c.addLabels(labelReplaceTestKey(s.T(), c, 2), labels.FromStrings("k", "second"), true)
	c.addLabels(labelReplaceTestKey(s.T(), c, 3), labels.FromStrings("k", "third"), false)
	third, thirdHit := c.getLabels(labelReplaceTestKey(s.T(), c, 3))

	// Assert: a previously returned labelset stays immutable after entry reuse.
	s.True(labels.Equal(firstLabels, got.result))
	s.True(got.matched)
	s.Require().True(thirdHit)
	s.False(third.matched)
	s.True(third.result.IsEmpty(), "a reused non-match entry must not retain the previous result")
	s.Equal(2.0, testutil.ToFloat64(c.labelsMetrics.evictions))
	s.Require().NotNil(c.spareEntry)
	s.True(c.spareEntry.result.IsEmpty(), "the spare entry must not retain an evicted labelset")
}

func (s *LabelReplaceCacheSuite) TestHitMiss() {
	// Arrange.
	key := labelReplaceTestKey(s.T(), s.cache, 1)
	want := labels.FromStrings("__name__", "metric", "job", "api")

	// Act: first lookup misses on the empty cache, then the entry is stored.
	_, ok := s.cache.getLabels(key)
	s.cache.addLabels(key, want, true)
	got, hit := s.cache.getLabels(key)

	// Assert.
	s.False(ok, "empty cache must miss")
	s.True(hit, "stored key must hit")
	s.True(labels.Equal(got.result, want))
	s.Equal(want.Hash(), got.result.Hash())
	s.Equal(1, s.cache.len())
	s.Equal(1.0, testutil.ToFloat64(s.cache.labelsMetrics.hits))
	s.Equal(1.0, testutil.ToFloat64(s.cache.labelsMetrics.misses))
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepAdmissionDoesNotWaitForLabelsLock() {
	// Arrange: the admitted background sweep cannot acquire labelsMu.
	c := s.cache
	c.labelsMu.Lock()
	admitted := make(chan struct{})

	// Act.
	go func() {
		c.maybeSweepAt(60_000)
		close(admitted)
	}()

	func() {
		defer c.labelsMu.Unlock()
		select {
		case <-admitted:
		case <-time.After(time.Second):
			s.FailNow("sweep admission waited for labelsMu")
		}

		// Assert: scheduling returned while the background pass is blocked.
		s.Equal(int64(60_000), c.nowMs.Load())
		s.Equal(0.0, testutil.ToFloat64(c.growResizes))
	}()
	waitLabelReplaceSweep(c)
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepAtMostOncePerInterval() {
	// Arrange: a full cache of fresh entries below its maximum.
	c := newLabelReplaceCache(100, 0, nil)
	s.T().Cleanup(c.close)
	s.Require().NotNil(c)
	for i := range 64 {
		c.addLabels(labelReplaceTestKey(s.T(), c, uint64(i)), labels.FromStrings("k", strconv.Itoa(i)), true)
	}

	// Act: two sweep attempts within one interval.
	c.maybeSweepAt(70_000)
	c.maybeSweepAt(70_000)
	waitLabelReplaceSweep(c)

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
	s.T().Cleanup(c.close)
	for i := range 64 {
		c.addLabels(labelReplaceTestKey(s.T(), c, uint64(i)), labels.FromStrings("k", strconv.Itoa(i)), true)
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
	waitLabelReplaceSweep(c)

	s.Equal(100, c.labelsCap)
	s.Equal(1.0, testutil.ToFloat64(c.growResizes))
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepBatchPreservesRefreshAndInsertion() {
	// Arrange: one full stale batch followed by two stale entries.
	capacity := labelReplaceCacheSweepBatchSize + 2
	c := newLabelReplaceCache(capacity, -1, nil)
	c.resize(capacity, c.growResizes)
	operation := c.acquireOperation(labelReplaceOperationKey{dst: "dst", repl: "$1", src: "src", regexStr: "(.*)"})
	defer c.releaseOperation(operation)
	result := labels.FromStrings("dst", "value")
	key := labelReplaceCacheKey{operation: operation}
	for i := range capacity {
		key.base = strconv.Itoa(i)
		c.addLabels(key, result, true)
	}

	c.nowMs.Store(200_000)

	// Act: execute one batch, then refresh and insert during the unlocked gap.
	c.labelsMu.Lock()
	removed := c.sweepBatchLocked(200_000, labelReplaceCacheSweepBatchSize)
	c.labelsMu.Unlock()
	key.base = strconv.Itoa(labelReplaceCacheSweepBatchSize)
	_, hit := c.getLabels(key)
	refreshed := key
	key.base = strconv.Itoa(capacity)
	c.addLabels(key, result, true)
	inserted := key
	c.sweepOnce(200_000)

	// Assert: rechecking the LRU removes only the remaining stale entry.
	s.Equal(labelReplaceCacheSweepBatchSize, removed)
	s.True(hit)
	_, refreshedHit := c.getLabels(refreshed)
	s.True(refreshedHit)
	_, insertedHit := c.getLabels(inserted)
	s.True(insertedHit)
	s.Equal(2, c.len())
	s.Equal(2.0, testutil.ToFloat64(c.labelsMetrics.entries))
	s.Equal(float64(capacity-1), testutil.ToFloat64(c.labelsMetrics.evictions))
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepBatchStopsAtFreshEntry() {
	// Arrange: the oldest entry is stale, the next two are fresh.
	c := newLabelReplaceCache(10, -1, nil)
	c.addLabels(labelReplaceTestKey(s.T(), c, 1), labels.FromStrings("k", "old"), true)
	c.nowMs.Store(200_000)
	c.addLabels(labelReplaceTestKey(s.T(), c, 2), labels.FromStrings("k", "fresh"), true)
	c.addLabels(labelReplaceTestKey(s.T(), c, 3), labels.FromStrings("k", "freshest"), true)

	// Act.
	c.labelsMu.Lock()
	removed := c.sweepBatchLocked(200_000, labelReplaceCacheSweepBatchSize)
	c.labelsMu.Unlock()

	// Assert.
	s.Equal(1, removed)
	s.Equal(2, c.len())
	s.Equal(2.0, testutil.ToFloat64(c.labelsMetrics.entries))
	s.Equal(1.0, testutil.ToFloat64(c.labelsMetrics.evictions))
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepGrowsProgressively() {
	// Arrange: a full cache at a scheduled step.
	c := newLabelReplaceCache(10000, 0, nil)
	s.Require().NotNil(c)
	c.labelsCap = 512
	c.labelsCache.Resize(512)
	c.labelsMetrics.capacity.Set(512)
	for i := range 512 {
		c.addLabels(labelReplaceTestKey(s.T(), c, uint64(i)), labels.FromStrings("k", strconv.Itoa(i)), true)
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
		d.addLabels(labelReplaceTestKey(s.T(), d, uint64(i)), labels.FromStrings("k", strconv.Itoa(i)), true)
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
		c.addLabels(labelReplaceTestKey(s.T(), c, uint64(i)), labels.FromStrings("k", strconv.Itoa(i)), true)
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
		c.addLabels(labelReplaceTestKey(s.T(), c, uint64(i)), labels.FromStrings("k", strconv.Itoa(i)), true)
	}

	// Assert.
	s.Equal(64, c.len())
	s.Equal(1.0, testutil.ToFloat64(c.labelsMetrics.evictions))
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepRefreshesIdleTsOnHit() {
	// Arrange: a wrapper whose clock sits at 0 with one stored entry.
	c := newLabelReplaceCache(1000, 0, nil)
	s.Require().NotNil(c)
	key := labelReplaceTestKey(s.T(), c, 1)
	c.addLabels(key, labels.FromStrings("k", "v"), true)

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
		c.addLabels(labelReplaceTestKey(s.T(), c, uint64(i)), labels.FromStrings("k", strconv.Itoa(i)), true)
	}
	c.nowMs.Store(200_000)
	for i := 16; i < 64; i++ {
		_, ok := c.getLabels(labelReplaceTestKey(s.T(), c, uint64(i)))
		s.Require().True(ok)
	}

	// Act: a sweep removes the stale prefix and stops at the first fresh entry.
	c.sweepOnce(200_000)

	// Assert: only the stale prefix is gone; the freed quarter is not enough
	// to shrink below the floor, so the capacity is untouched.
	s.Equal(48, c.len())
	s.Equal(48.0, testutil.ToFloat64(c.labelsMetrics.entries))
	s.Equal(16.0, testutil.ToFloat64(c.labelsMetrics.evictions))
	for i := 16; i < 64; i++ {
		_, ok := c.getLabels(labelReplaceTestKey(s.T(), c, uint64(i)))
		s.True(ok, "fresh entries must survive the sweep")
	}
	s.Equal(64, c.labelsCap)
	s.Equal(0.0, testutil.ToFloat64(c.growResizes))
	s.Equal(0.0, testutil.ToFloat64(c.shrinkResizes))
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepPreservesReturnedLabels() {
	// Arrange.
	c := newLabelReplaceCache(10, 0, nil)
	key := labelReplaceTestKey(s.T(), c, 1)
	want := labels.FromStrings("k", "original")
	c.addLabels(key, want, true)
	got, hit := c.getLabels(key)
	s.Require().True(hit)

	// Act: sweep the entry, then reuse it for a different series.
	c.nowMs.Store(200_000)
	c.sweepOnce(200_000)
	c.addLabels(labelReplaceTestKey(s.T(), c, 2), labels.FromStrings("k", "replacement"), true)

	// Assert.
	s.True(labels.Equal(want, got.result))
	_, oldHit := c.getLabels(key)
	s.False(oldHit)
	s.Equal(1, c.len())
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepSerializesRefreshWithRemoval() {
	// Arrange: the first eviction pauses the sweep while another query wants
	// to refresh the next stale entry. The fresh tail must survive both operations.
	c := newLabelReplaceCache(100, 0, nil)
	oldest := labelReplaceTestKey(s.T(), c, 1)
	next := labelReplaceTestKey(s.T(), c, 2)
	fresh := labelReplaceTestKey(s.T(), c, 3)
	c.addLabels(oldest, labels.FromStrings("k", "oldest"), true)
	c.addLabels(next, labels.FromStrings("k", "next"), true)
	c.nowMs.Store(200_000)
	c.addLabels(fresh, labels.FromStrings("k", "fresh"), true)

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
	s.Equal(1.0, testutil.ToFloat64(c.labelsMetrics.entries))
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
		c.addLabels(labelReplaceTestKey(s.T(), c, uint64(i)), labels.FromStrings("k", strconv.Itoa(i)), true)
	}

	// Act: seven sweeps two ticks apart, survivors re-hit each time so no
	// single sweep frees a quarter.
	for round := range 7 {
		c.nowMs.Store(int64(200_000 + round*200_000))
		for i := 10 * (round + 1); i < 200; i++ {
			_, ok := c.getLabels(labelReplaceTestKey(s.T(), c, uint64(i)))
			s.Require().True(ok)
		}
		c.sweepOnce(c.nowMs.Load())
	}

	// Assert: the slow drain shrinks once the survivors drop below two
	// thirds of the capacity.
	s.Equal(130, c.len())
	s.Equal(130.0, testutil.ToFloat64(c.labelsMetrics.entries))
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
		c.addLabels(labelReplaceTestKey(s.T(), c, uint64(i)), labels.FromStrings("k", strconv.Itoa(i)), true)
	}
	c.nowMs.Store(200_000)
	for i := 100; i < 200; i++ {
		_, ok := c.getLabels(labelReplaceTestKey(s.T(), c, uint64(i)))
		s.Require().True(ok)
	}

	// Act: the sweep frees 100 entries, a quarter of the capacity or more.
	c.sweepOnce(200_000)

	// Assert: the capacity shrinks toward the surviving count plus headroom.
	s.Equal(100, c.len())
	s.Equal(100.0, testutil.ToFloat64(c.labelsMetrics.entries))
	s.Equal(110, c.labelsCap)
	s.Equal(110.0, testutil.ToFloat64(c.labelsMetrics.capacity))
	s.Equal(1.0, testutil.ToFloat64(c.shrinkResizes))
	s.Equal(0.0, testutil.ToFloat64(c.growResizes))
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSweepStaleEntryStillServed() {
	// Arrange: one stored entry, the clock far past the stale threshold.
	c := newLabelReplaceCache(10, 0, nil)
	s.Require().NotNil(c)
	key := labelReplaceTestKey(s.T(), c, 1)
	want := labels.FromStrings("k", "v")
	c.addLabels(key, want, true)
	c.nowMs.Store(200_000)

	// Act: a lookup of the stale entry.
	got, hit := c.getLabels(key)

	// Assert: a stale entry is still a valid hit — age matters only to the
	// sweep, never to serving.
	s.True(hit, "a stale entry must still be served")
	s.True(labels.Equal(want, got.result))
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
	key := labelReplaceTestKey(s.T(), c, 1)

	// Act: a lookup always misses and storing is a noop.
	_, hitFirst := c.getLabels(key)
	c.addLabels(key, labels.FromStrings("k", "v"), true)
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
	key := labelReplaceTestKey(s.T(), c, 1)

	// Act and assert: every method is nil-safe.
	_, ok := c.getLabels(key)
	s.False(ok, "nil cache must always miss")

	c.addLabels(key, labels.FromStrings("k", "v"), true)
	s.Equal(0, c.len())

	re, err := c.getOrCompileRegex("a+")
	s.Require().NoError(err)
	s.True(re.MatchString("aaa"))
	s.False(re.MatchString("b"))
}

func (s *LabelReplaceCacheSuite) TestLabelsKeyOwnsSerializationBuffer() {
	// Arrange: equal-length serializations reuse the same backing buffer.
	c := s.cache
	operation := c.acquireOperation(labelReplaceOperationKey{dst: "dst", repl: "$1", src: "src", regexStr: "(.*)"})
	defer c.releaseOperation(operation)
	first := labels.FromStrings("instance", "first", "src", "same")
	second := labels.FromStrings("instance", "other", "src", "same")
	buffer := first.Bytes(make([]byte, 0, 128))
	key := labelReplaceCacheKey{operation: operation, base: unsafe.String(unsafe.SliceData(buffer), len(buffer))}
	c.addLabels(key, first, true)

	// Act: overwrite the temporary key bytes and store a different result.
	buffer = second.Bytes(buffer)
	key.base = unsafe.String(unsafe.SliceData(buffer), len(buffer))
	c.addLabels(key, second, false)
	buffer = first.Bytes(buffer)
	key.base = unsafe.String(unsafe.SliceData(buffer), len(buffer))
	gotFirst, firstHit := c.getLabels(key)
	buffer = second.Bytes(buffer)
	key.base = unsafe.String(unsafe.SliceData(buffer), len(buffer))
	gotSecond, secondHit := c.getLabels(key)

	// Assert: neither stored key nor the returned snapshot aliases the scratch buffer.
	s.True(firstHit)
	s.True(secondHit)
	s.True(labels.Equal(first, gotFirst.result))
	s.True(gotSecond.result.IsEmpty(), "a non-match entry must not retain the source labels")
	s.True(gotFirst.matched)
	s.False(gotSecond.matched)
	s.Equal(2, c.len())
}

func (s *LabelReplaceCacheSuite) TestOperationHashCollisionPreservesCachedLabels() {
	// Arrange: distinct parameter sets deliberately share a hash bucket.
	c := newLabelReplaceCache(10, -1, nil)
	first := c.acquireOperationWithHash(labelReplaceOperationKey{dst: "first", repl: "$1"}, 42)
	second := c.acquireOperationWithHash(labelReplaceOperationKey{dst: "second", repl: "$1"}, 42)
	c.addLabels(labelReplaceCacheKey{operation: first, base: "source"}, labels.FromStrings("first", "value"), true)
	c.addLabels(labelReplaceCacheKey{operation: second, base: "source"}, labels.FromStrings("second", "value"), true)

	// Act: a colliding lookup must resolve its own identity and labelset.
	again := c.acquireOperationWithHash(first.key, 42)
	got, hit := c.getLabels(labelReplaceCacheKey{operation: again, base: "source"})
	c.releaseOperation(first)
	c.releaseOperation(second)
	c.releaseOperation(again)

	// Assert.
	s.Require().True(hit)
	s.Equal("value", got.result.Get("first"))
	s.False(got.result.Has("second"))

	// Act: sweeping both entries releases the whole collision chain.
	c.sweepOnce(200_000)

	// Assert.
	s.Empty(c.operations)
}

func (s *LabelReplaceCacheSuite) TestOperationHashCollisionReleaseKeepsOtherIdentity() {
	// Arrange.
	c := newLabelReplaceCache(10, -1, nil)
	first := c.acquireOperationWithHash(labelReplaceOperationKey{dst: "first", repl: "$1"}, 42)
	second := c.acquireOperationWithHash(labelReplaceOperationKey{dst: "second", repl: "$1"}, 42)

	// Act: releasing the bucket head must preserve the other active operation.
	c.releaseOperation(second)
	again := c.acquireOperationWithHash(first.key, 42)

	// Assert.
	s.Same(first, again)

	// Act.
	c.releaseOperation(first)
	c.releaseOperation(again)

	// Assert.
	s.Empty(c.operations)
}

func (s *LabelReplaceCacheSuite) TestOperationHashCollisionReleaseMiddleKeepsOtherIdentities() {
	// Arrange.
	c := newLabelReplaceCache(10, -1, nil)
	first := c.acquireOperationWithHash(labelReplaceOperationKey{dst: "first"}, 42)
	middle := c.acquireOperationWithHash(labelReplaceOperationKey{dst: "middle"}, 42)
	last := c.acquireOperationWithHash(labelReplaceOperationKey{dst: "last"}, 42)

	// Act: remove a non-head operation from a collision chain.
	c.releaseOperation(middle)
	firstAgain := c.acquireOperationWithHash(first.key, 42)
	lastAgain := c.acquireOperationWithHash(last.key, 42)

	// Assert.
	s.Same(first, firstAgain)
	s.Same(last, lastAgain)

	// Act.
	c.releaseOperation(first)
	c.releaseOperation(last)
	c.releaseOperation(firstAgain)
	c.releaseOperation(lastAgain)

	// Assert.
	s.Empty(c.operations)
}

func (s *LabelReplaceCacheSuite) TestOperationReleasedWithoutCachedLabels() {
	// Arrange.
	c := newLabelReplaceCache(10, -1, nil)
	operation := c.acquireOperation(labelReplaceOperationKey{dst: "dst", repl: "$1", src: "src", regexStr: "(.*)"})

	// Act: an empty evaluation stores no labels.
	c.releaseOperation(operation)

	// Assert.
	s.Empty(c.operations)
}

func (s *LabelReplaceCacheSuite) TestOperationReplacementDoesNotLeakReferences() {
	// Arrange.
	c := newLabelReplaceCache(10, -1, nil)
	operation := c.acquireOperation(labelReplaceOperationKey{dst: "dst", repl: "$1", src: "src", regexStr: "(.*)"})
	key := labelReplaceCacheKey{operation: operation, base: "source"}
	result := labels.FromStrings("dst", "value")

	// Act: two concurrent misses may eventually store the same key twice.
	c.addLabels(key, result, true)
	c.addLabels(key, result, true)
	c.releaseOperation(operation)
	c.sweepOnce(200_000)

	// Assert: duplicate insertion must not leave a phantom cached-key reference.
	s.Empty(c.operations)
	s.Equal(1.0, testutil.ToFloat64(c.labelsMetrics.evictions))
}

func (s *LabelReplaceCacheSuite) TestOperationsPreserveHitsAcrossManyDistinctCalls() {
	// Arrange: all 200 operations fit into the labels cache.
	c := newLabelReplaceCache(400, -1, nil)
	c.resize(200, c.growResizes)
	for i := range 200 {
		operation := c.acquireOperation(labelReplaceOperationKey{dst: "dst", repl: strconv.Itoa(i), src: "src"})
		key := labelReplaceCacheKey{operation: operation, base: "source"}
		c.addLabels(key, labels.FromStrings("dst", strconv.Itoa(i)), true)
		c.releaseOperation(operation)
	}

	// Act and assert: resolving the parameters again must reach the cached labels.
	for i := range 200 {
		operation := c.acquireOperation(labelReplaceOperationKey{dst: "dst", repl: strconv.Itoa(i), src: "src"})
		got, hit := c.getLabels(labelReplaceCacheKey{operation: operation, base: "source"})
		c.releaseOperation(operation)
		s.Require().True(hit)
		s.Equal(strconv.Itoa(i), got.result.Get("dst"))
	}

	// Act: removing the final cached keys releases their operation identities.
	c.sweepOnce(200_000)

	// Assert.
	s.Empty(c.operations)
}

func (s *LabelReplaceCacheSuite) TestOperationSurvivesEvictionDuringEvaluation() {
	// Arrange: a live evaluation keeps its operation while another one evicts its labels.
	c := newLabelReplaceCache(1, -1, nil)
	operation := c.acquireOperation(labelReplaceOperationKey{dst: "first", repl: "$1"})
	other := c.acquireOperation(labelReplaceOperationKey{dst: "second", repl: "$1"})
	c.addLabels(labelReplaceCacheKey{operation: operation, base: "source"}, labels.FromStrings("first", "value"), true)

	// Act.
	c.addLabels(labelReplaceCacheKey{operation: other, base: "source"}, labels.FromStrings("second", "value"), true)
	again := c.acquireOperation(operation.key)

	// Assert: the ongoing evaluation and a new one still use the same identity.
	s.Same(operation, again)

	// Act: release the evaluations and remove the last cached key.
	c.releaseOperation(again)
	c.releaseOperation(operation)
	c.releaseOperation(other)
	c.sweepOnce(200_000)

	// Assert.
	s.Empty(c.operations)
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

// labelReplaceTestKey holds an evaluation lease until test cleanup so fixture
// keys keep their operation identity even when all cached labels are evicted.
func labelReplaceTestKey(t testing.TB, cache *labelReplaceCache, identity uint64) labelReplaceCacheKey {
	t.Helper()

	operation := cache.acquireOperation(labelReplaceOperationKey{
		dst: "exported_job", repl: "$1", src: "job", regexStr: "(.*)",
	})
	t.Cleanup(func() { cache.releaseOperation(operation) })

	return labelReplaceCacheKey{operation: operation, base: strconv.FormatUint(identity, 10)}
}

// waitLabelReplaceSweep joins the admitted pass without closing the cache.
func waitLabelReplaceSweep(cache *labelReplaceCache) {
	cache.sweepMu.Lock()
	cache.sweepMu.Unlock()
}
