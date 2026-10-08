package promql_test

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/teststorage"
)

// labelReplaceEvalMillis is the shared evaluation timestamp (in
// milliseconds) for every query and sample in this file.
const labelReplaceEvalMillis int64 = 10 * 60 * 1000

// Metric family names and the "cache" label value of the label_replace cache.
const (
	labelReplaceCacheHitsFamily   = "prometheus_engine_label_replace_cache_hits_total"
	labelReplaceCacheMissesFamily = "prometheus_engine_label_replace_cache_misses_total"
	labelReplaceCacheLabelsID     = "labels"
)

// LabelReplaceCacheSuite covers the label_replace cache at the query level:
// key correctness, reuse between queries, semantic contracts and the disabled
// mode. Every method shares one Engine per test via SetupTest.
type LabelReplaceCacheSuite struct {
	suite.Suite

	env *labelReplaceTestEnv
}

func TestLabelReplaceCache(t *testing.T) {
	suite.Run(t, new(LabelReplaceCacheSuite))
}

func (s *LabelReplaceCacheSuite) SetupTest() {
	s.env = newLabelReplaceTestEnv(s.T(), 0)
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheDisabled() {
	t := s.T()
	// Arrange: one engine with the cache enabled, one with it disabled.
	series := []labels.Labels{labels.FromStrings(labels.MetricName, "disabled_metric", "src", "value")}
	enabled := newLabelReplaceTestEnv(t, 0)
	disabled := newLabelReplaceTestEnv(t, -1)
	enabled.appendSamples(t, series...)
	disabled.appendSamples(t, series...)
	qs := `label_replace(disabled_metric, "dst", "value-$1", "src", "(.*)")`

	// Act: two runs per engine.
	enabledFirst := enabled.mustRun(t, qs)
	enabledSecond := enabled.mustRun(t, qs)
	disabledFirst := disabled.mustRun(t, qs)
	disabledSecond := disabled.mustRun(t, qs)

	// Assert: identical bytes with and without the cache.
	s.Require().Equal(enabledFirst.String(), disabledFirst.String())
	s.Require().Equal(enabledFirst.String(), enabledSecond.String())
	s.Require().Equal(disabledFirst.String(), disabledSecond.String())

	// Assert: only the enabled engine accrues cache hits. Counters are
	// monotonic, so a final zero on the disabled engine proves no growth.
	s.Require().NotZero(enabled.cacheSum(t, labelReplaceCacheHitsFamily, labelReplaceCacheLabelsID))
	s.Require().Zero(disabled.cacheSum(t, labelReplaceCacheHitsFamily, labelReplaceCacheLabelsID))
	s.Require().Zero(disabled.cacheSum(t, labelReplaceCacheMissesFamily, labelReplaceCacheLabelsID))
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheKey() {
	t := s.T()
	// Arrange: identical dst/repl/src/regex/srcVal but different labels.
	s.env.appendSamples(
		t,
		labels.FromStrings(labels.MetricName, "key_metric", "instance", "a", "src", "value"),
		labels.FromStrings(labels.MetricName, "key_metric", "instance", "b", "src", "value"),
	)
	qs := `label_replace(key_metric, "dst", "value-$1", "src", "(.*)")`

	// Act: the first run fills the cache, the second one reuses it.
	first := s.env.mustRun(t, qs)
	hitsFirst := s.env.cacheSum(t, labelReplaceCacheHitsFamily, labelReplaceCacheLabelsID)
	second := s.env.mustRun(t, qs)

	// Assert: both runs keep each series' own labelset.
	for _, res := range []*promql.Result{first, second} {
		vec, err := res.Vector()
		s.Require().NoError(err)
		s.Require().Len(vec, 2)
		byInstance := make(map[string]labels.Labels, len(vec))
		for _, sample := range vec {
			byInstance[sample.Metric.Get("instance")] = sample.Metric
		}

		s.Require().Len(byInstance, 2, "series must not inherit each other's labels from the cache")

		for _, instance := range []string{"a", "b"} {
			metric := byInstance[instance]
			s.Require().Equal(instance, metric.Get("instance"))
			s.Require().Equal("value", metric.Get("src"))
			s.Require().Equal("value-value", metric.Get("dst"))
		}
	}

	s.Require().Zero(hitsFirst)
	hitsSecond := s.env.cacheSum(t, labelReplaceCacheHitsFamily, labelReplaceCacheLabelsID)
	s.Require().Greater(hitsSecond, hitsFirst, "the second run must be served from the cache")
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheReuseSecondRunHitsAndIdenticalResult() {
	t := s.T()
	// Arrange.
	s.env.appendSamples(
		t,
		labels.FromStrings(labels.MetricName, "reuse_metric", "instance", "a", "src", "value"),
		labels.FromStrings(labels.MetricName, "reuse_metric", "instance", "b", "src", "value"),
	)
	qs := `label_replace(reuse_metric, "dst", "value-$1", "src", "(.*)")`

	// Act.
	first := s.env.mustRun(t, qs)
	hitsFirst := s.env.cacheSum(t, labelReplaceCacheHitsFamily, labelReplaceCacheLabelsID)
	missesFirst := s.env.cacheSum(t, labelReplaceCacheMissesFamily, labelReplaceCacheLabelsID)
	second := s.env.mustRun(t, qs)

	// Assert.
	s.Require().Zero(hitsFirst)
	s.Equal(first.String(), second.String(), "the second run must be byte-identical")
	hitsSecond := s.env.cacheSum(t, labelReplaceCacheHitsFamily, labelReplaceCacheLabelsID)
	missesSecond := s.env.cacheSum(t, labelReplaceCacheMissesFamily, labelReplaceCacheLabelsID)
	s.Greater(hitsSecond, hitsFirst, "the second run must produce labelset-cache hits")
	s.Equal(missesFirst, missesSecond, "the second run must not miss again")
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheReuseSubqueryPopulatesEngineCache() {
	t := s.T()
	// Arrange.
	s.env.appendSamples(t, labels.FromStrings(labels.MetricName, "reuse_sub_metric", "src", "value"))
	subqueryQS := `label_replace(reuse_sub_metric, "dst", "value-$1", "src", "(.*)")[3m:1m]`
	directQS := `label_replace(reuse_sub_metric, "dst", "value-$1", "src", "(.*)")`

	// Act: the subquery runs first, the direct query afterwards.
	subqueryRes := s.env.mustRun(t, subqueryQS)
	hitsAfterSubquery := s.env.cacheSum(t, labelReplaceCacheHitsFamily, labelReplaceCacheLabelsID)
	directRes := s.env.mustRun(t, directQS)
	hitsAfterDirect := s.env.cacheSum(t, labelReplaceCacheHitsFamily, labelReplaceCacheLabelsID)

	// Assert: both paths produce the same replacement...
	subqueryMatrix, err := subqueryRes.Matrix()
	s.Require().NoError(err)
	s.Require().Len(subqueryMatrix, 1)
	s.Require().Equal("value-value", subqueryMatrix[0].Metric.Get("dst"))

	directVec, err := directRes.Vector()
	s.Require().NoError(err)
	s.Require().Len(directVec, 1)
	s.Require().Equal("value-value", directVec[0].Metric.Get("dst"))

	// ...and the direct query hits the cache the subquery filled: the cache
	// belongs to the Engine, not to a single query.
	s.Greater(hitsAfterDirect, hitsAfterSubquery, "the subquery must populate the shared engine cache")
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSemanticsDropNameOnCacheHit() {
	t := s.T()
	// Arrange: the unary minus input carries DropName=true, so replacing
	// dst=__name__ must reset DropName on the cache-hit path as well.
	s.env.appendSamples(t, labels.FromStrings(labels.MetricName, "sem_dropname", "job", "sem"))
	qs := `label_replace(-sem_dropname, "__name__", "sem_renamed", "__name__", "(.*)")`

	// Act.
	first := s.env.mustRun(t, qs)
	hitsFirst := s.env.cacheSum(t, labelReplaceCacheHitsFamily, labelReplaceCacheLabelsID)
	second := s.env.mustRun(t, qs)

	// Assert.
	for _, res := range []*promql.Result{first, second} {
		vec, err := res.Vector()
		s.Require().NoError(err)
		s.Require().Len(vec, 1)
		s.Require().False(vec[0].DropName, "dst=__name__ must yield DropName=false")
		s.Require().Equal("sem_renamed", vec[0].Metric.Get(labels.MetricName))
	}

	hitsSecond := s.env.cacheSum(t, labelReplaceCacheHitsFamily, labelReplaceCacheLabelsID)
	s.Greater(hitsSecond, hitsFirst, "the second run must be served from the cache")
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSemanticsInvalidRegexWithWarmCache() {
	t := s.T()
	// Arrange: fill the labelset cache and force full-hit runs first.
	s.env.appendSamples(t, labels.FromStrings(labels.MetricName, "sem_invalid", "src", "value"))
	warmQS := `label_replace(sem_invalid, "dst", "value-$1", "src", "(.*)")`
	s.env.mustRun(t, warmQS)
	s.env.mustRun(t, warmQS)
	hitsWarm := s.env.cacheSum(t, labelReplaceCacheHitsFamily, labelReplaceCacheLabelsID)

	// Act: the same series with an invalid regex must still fail.
	invalidQS := `label_replace(sem_invalid, "dst", "value-$1", "src", "(.*")`
	first := s.env.run(t, invalidQS)
	second := s.env.run(t, invalidQS)

	// Assert.
	s.Require().Greater(hitsWarm, 0.0, "the labelset cache must be warm before the failing runs")
	s.Require().EqualError(first.Err, "invalid regular expression in label_replace(): (.*")
	s.Require().EqualError(second.Err, "invalid regular expression in label_replace(): (.*")
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSemanticsNonMatchingRegex() {
	t := s.T()
	// Arrange.
	s.env.appendSamples(t, labels.FromStrings(labels.MetricName, "sem_nonmatch", "job", "sem"))
	replaceQS := `label_replace(sem_nonmatch, "dst", "$1", "missing_src", "(.+)")`

	// Act.
	plain := s.env.mustRun(t, `sem_nonmatch`)
	first := s.env.mustRun(t, replaceQS)
	second := s.env.mustRun(t, replaceQS)

	// Assert: a non-matching regex leaves labelset and DropName untouched.
	plainVec, err := plain.Vector()
	s.Require().NoError(err)

	firstVec, err := first.Vector()
	s.Require().NoError(err)

	secondVec, err := second.Vector()
	s.Require().NoError(err)
	s.Equal(plainVec, firstVec)
	s.Equal(plainVec, secondVec)
}

func (s *LabelReplaceCacheSuite) TestLabelReplaceCacheSemanticsWarningsOnCacheHit() {
	t := s.T()
	// Arrange: quantile_over_time(2, ...) yields an invalid-quantile warning
	// while evaluating the argument of label_replace.
	s.env.appendSamples(t, labels.FromStrings(labels.MetricName, "sem_warn", "job", "sem"))
	qs := `label_replace(quantile_over_time(2, sem_warn[1m]), "warn_dst", "v", "missing_src", "(.*)")`

	// Act.
	first := s.env.mustRun(t, qs)
	hitsFirst := s.env.cacheSum(t, labelReplaceCacheHitsFamily, labelReplaceCacheLabelsID)
	second := s.env.mustRun(t, qs)

	// Assert.
	s.Require().True(annotationsContain(first.Warnings, "quantile value should be between 0 and 1"))
	s.Require().True(
		annotationsContain(second.Warnings, "quantile value should be between 0 and 1"),
		"argument warnings must be returned even on a full cache hit",
	)
	s.Equal(first.String(), second.String())

	hitsSecond := s.env.cacheSum(t, labelReplaceCacheHitsFamily, labelReplaceCacheLabelsID)
	s.Greater(hitsSecond, hitsFirst, "the second run must be served from the cache")
}

// labelReplaceTestEnv bundles the engine, its metrics registry and the
// storage a single test works against.
type labelReplaceTestEnv struct {
	engine   *promql.Engine
	registry *prometheus.Registry
	storage  *teststorage.TestStorage
}

// newLabelReplaceTestEnv builds an engine with its own registry (for the
// cache metrics) and storage; both are torn down with the test. cacheSize is
// EngineOpts.LabelReplaceCacheSize: 0 selects the default, a negative value
// disables the cache.
func newLabelReplaceTestEnv(t testing.TB, cacheSize int) *labelReplaceTestEnv {
	t.Helper()

	registry := prometheus.NewRegistry()
	engine := promql.NewEngine(promql.EngineOpts{
		Logger:                   nil,
		Reg:                      registry,
		MaxSamples:               10000,
		Timeout:                  10 * time.Second,
		LookbackDelta:            5 * time.Minute,
		NoStepSubqueryIntervalFn: func(int64) int64 { return int64(time.Minute / time.Millisecond) },
		LabelReplaceCacheSize:    cacheSize,
	})
	storage := teststorage.New(t)
	t.Cleanup(func() { _ = storage.Close() })

	return &labelReplaceTestEnv{engine: engine, registry: registry, storage: storage}
}

// appendSamples writes one sample per label set at the shared evaluation
// timestamp.
func (env *labelReplaceTestEnv) appendSamples(t testing.TB, series ...labels.Labels) {
	t.Helper()

	app := env.storage.Appender(t.Context())
	for _, lbls := range series {
		_, err := app.Append(0, lbls, labelReplaceEvalMillis, 1)
		require.NoError(t, err)
	}

	require.NoError(t, app.Commit())
}

// cacheSum sums one label_replace cache metric family for a single "cache"
// label value; it returns 0 when the family is not registered at all.
func (env *labelReplaceTestEnv) cacheSum(t testing.TB, family, cacheLabel string) float64 {
	t.Helper()

	mfs, err := env.registry.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() != family {
			continue
		}

		var sum float64
		for _, metric := range mf.GetMetric() {
			for _, pair := range metric.GetLabel() {
				if pair.GetName() == "cache" && pair.GetValue() == cacheLabel {
					sum += metric.GetCounter().GetValue() + metric.GetGauge().GetValue()
				}
			}
		}

		return sum
	}

	return 0
}

// mustRun runs a query and requires it to succeed.
func (env *labelReplaceTestEnv) mustRun(t testing.TB, qs string) *promql.Result {
	t.Helper()

	res := env.run(t, qs)
	require.NoError(t, res.Err)

	return res
}

// run executes an instant query at the shared evaluation timestamp without
// asserting on the query error.
func (env *labelReplaceTestEnv) run(t testing.TB, qs string) *promql.Result {
	t.Helper()

	q, err := env.engine.NewInstantQuery(t.Context(), env.storage, nil, qs, time.UnixMilli(labelReplaceEvalMillis))
	require.NoError(t, err)

	return q.Exec(t.Context())
}

// annotationsContain reports whether any warning message contains substr.
func annotationsContain(ws annotations.Annotations, substr string) bool {
	for msg := range ws {
		if strings.Contains(msg, substr) {
			return true
		}
	}

	return false
}
