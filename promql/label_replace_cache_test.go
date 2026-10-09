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
	s.True(labels.Equal(got, newest))
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
	s.Equal(1.0, testutil.ToFloat64(s.cache.hits.WithLabelValues(labelReplaceCacheLabelsID)))
	s.Equal(1.0, testutil.ToFloat64(s.cache.misses.WithLabelValues(labelReplaceCacheLabelsID)))
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
	s.NoError(err)
	s.True(re.MatchString("aaa"))
	s.False(re.MatchString("b"))

	c.register(nil)
	reg := prometheus.NewRegistry()
	c.register(reg)
	mfs, err := reg.Gather()
	s.NoError(err)
	s.Empty(mfs, "nil cache must not register any metrics")
}

func (s *LabelReplaceCacheSuite) TestRegex() {
	// Arrange.
	hits := s.cache.hits.WithLabelValues(labelReplaceCacheRegexID)
	misses := s.cache.misses.WithLabelValues(labelReplaceCacheRegexID)
	entries := s.cache.entries.WithLabelValues(labelReplaceCacheRegexID)

	// Act: first call compiles, the second one is served from the cache.
	re1, err := s.cache.getOrCompileRegex("(.+)")
	s.NoError(err)
	re2, err := s.cache.getOrCompileRegex("(.+)")

	// Assert: same text yields the identical compiled regex.
	s.NoError(err)
	s.Same(re1, re2)
	s.Equal(1.0, testutil.ToFloat64(misses))
	s.Equal(1.0, testutil.ToFloat64(hits))
	s.Equal(1.0, testutil.ToFloat64(entries))

	// Act: an invalid regex fails twice and never grows the cache.
	_, err = s.cache.getOrCompileRegex("a(")
	s.Error(err)
	_, err = s.cache.getOrCompileRegex("a(")
	s.Error(err)

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
	s.NoError(err)
	s.True(re.MatchString("aaa"))
	s.False(re.MatchString("b"))
	_, err = c.getOrCompileRegex("a(")
	s.Error(err)

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
	s.NoError(err)
	_, err = c.getOrCompileRegex("b+")
	s.NoError(err)

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
