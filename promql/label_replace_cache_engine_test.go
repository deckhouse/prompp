package promql

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/suite"
)

// NewEngineLabelReplaceCacheSuite covers the label_replace cache wiring done
// in NewEngine: default capacity, disabled mode, and metric registration.
type NewEngineLabelReplaceCacheSuite struct {
	suite.Suite
}

func TestNewEngineLabelReplaceCache(t *testing.T) {
	suite.Run(t, new(NewEngineLabelReplaceCacheSuite))
}

func (s *NewEngineLabelReplaceCacheSuite) TestZeroSizeCreatesCacheWithDefaultCapacity() {
	// Arrange.
	opts := EngineOpts{LabelReplaceCacheSize: 0}

	// Act.
	ng := NewEngine(opts)

	// Assert.
	s.Require().NotNil(ng.labelReplaceCache)
	s.Equal(0, ng.labelReplaceCache.len())
}

func (s *NewEngineLabelReplaceCacheSuite) TestNegativeSizeLeavesCacheDisabled() {
	// Arrange.
	opts := EngineOpts{LabelReplaceCacheSize: -1}

	// Act.
	ng := NewEngine(opts)

	// Assert.
	s.Nil(ng.labelReplaceCache)
}

func (s *NewEngineLabelReplaceCacheSuite) TestCacheMetricsRegisteredWithRegistry() {
	// Arrange.
	reg := prometheus.NewRegistry()
	opts := EngineOpts{Reg: reg, LabelReplaceCacheSize: 0}

	// Act.
	ng := NewEngine(opts)

	// Seed one child per counter vec: a fresh engine pre-creates only the
	// entries gauge children, and childless vecs are dropped from Gather.
	s.Require().NotNil(ng.labelReplaceCache)
	ng.labelReplaceCache.hits.WithLabelValues(labelReplaceCacheLabelsID)
	ng.labelReplaceCache.misses.WithLabelValues(labelReplaceCacheLabelsID)
	ng.labelReplaceCache.evictions.WithLabelValues(labelReplaceCacheLabelsID)

	// Assert: every cache metric family is visible in the gather.
	for _, name := range []string{
		"prometheus_engine_label_replace_cache_hits_total",
		"prometheus_engine_label_replace_cache_misses_total",
		"prometheus_engine_label_replace_cache_evictions_total",
		"prometheus_engine_label_replace_cache_entries",
	} {
		count, err := testutil.GatherAndCount(reg, name)
		s.Require().NoError(err)
		s.NotZero(count, "family %s must be registered", name)
	}
}

func (s *NewEngineLabelReplaceCacheSuite) TestNilRegistryBuildsWorkingEngine() {
	// Arrange: Reg nil must not panic and must still yield a usable engine.
	opts := EngineOpts{Reg: nil, LabelReplaceCacheSize: 0}

	// Act.
	ng := NewEngine(opts)

	// Assert.
	s.Require().NotNil(ng)
	s.NotNil(ng.labelReplaceCache)
}
