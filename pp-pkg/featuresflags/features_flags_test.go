package featuresflags

import (
	"os"
	"strings"
	"testing"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/suite"
)

const differFromDefaultMetric = "prompp_features_differ_from_default"

type ParseFeaturesSuite struct {
	suite.Suite
}

func TestParseFeaturesSuite(t *testing.T) {
	suite.Run(t, new(ParseFeaturesSuite))
}

func (s *ParseFeaturesSuite) TestEmpty() {
	s.Empty(parseFeatures(""))
}

func (s *ParseFeaturesSuite) TestTrimsSpacesAndSkipsEmptyTokens() {
	features := parseFeatures(" a = 1 ,, b ,")

	s.Equal(map[string]string{"a": "1", "b": ""}, features)
}

func (s *ParseFeaturesSuite) TestValueWithEqualSign() {
	features := parseFeatures("a=b=c")

	s.Equal(map[string]string{"a": "b=c"}, features)
}

func (s *ParseFeaturesSuite) TestDuplicateLastWins() {
	features := parseFeatures("a=1,a=2")

	s.Equal(map[string]string{"a": "2"}, features)
}

type DiffFeaturesSuite struct {
	suite.Suite
}

func TestDiffFeaturesSuite(t *testing.T) {
	suite.Run(t, new(DiffFeaturesSuite))
}

func (s *DiffFeaturesSuite) TestEqualRegardlessOfOrder() {
	diff := diffFeatures(parseFeatures("a=1,b"), parseFeatures("b, a=1"))

	s.True(diff.isEmpty())
}

func (s *DiffFeaturesSuite) TestBothEmpty() {
	diff := diffFeatures(parseFeatures(""), parseFeatures(""))

	s.True(diff.isEmpty())
}

func (s *DiffFeaturesSuite) TestAddedRemovedChanged() {
	diff := diffFeatures(parseFeatures("a=1,b,c=2"), parseFeatures("a=1,c=3,d=4"))

	s.Equal(featuresDiff{
		added:   []string{"b"},
		removed: []string{"d=4"},
		changed: []string{"c=3 -> c=2"},
	}, diff)
}

func (s *DiffFeaturesSuite) TestUnknownFeatureValuesComparedAsIs() {
	diff := diffFeatures(parseFeatures("a"), parseFeatures("a=1"))

	s.Equal([]string{"a=1 -> a"}, diff.changed)
}

func (s *DiffFeaturesSuite) TestHeadReadConcurrencyEmptyEqualsOne() {
	diff := diffFeatures(parseFeatures("head_read_concurrency"), parseFeatures("head_read_concurrency=1"))

	s.True(diff.isEmpty())
}

func (s *DiffFeaturesSuite) TestIntValuesComparedByMeaning() {
	diff := diffFeatures(
		parseFeatures("head_read_concurrency=04,head_default_number_of_shards=+8,federation_split_families=010"),
		parseFeatures("head_read_concurrency=4,head_default_number_of_shards=8,federation_split_families=10"),
	)

	s.True(diff.isEmpty())
}

func (s *DiffFeaturesSuite) TestDurationValuesComparedByMeaning() {
	diff := diffFeatures(parseFeatures("default_sample_age_limit=60m"), parseFeatures("default_sample_age_limit=1h"))

	s.True(diff.isEmpty())
}

func (s *DiffFeaturesSuite) TestDifferentDurationValuesDiffer() {
	diff := diffFeatures(parseFeatures("default_sample_age_limit=30m"), parseFeatures("default_sample_age_limit=1h"))

	s.Equal([]string{"default_sample_age_limit=1h -> default_sample_age_limit=30m"}, diff.changed)
}

func (s *DiffFeaturesSuite) TestUnparsableValuesComparedAsIs() {
	diff := diffFeatures(parseFeatures("head_default_number_of_shards=abc"), parseFeatures("head_default_number_of_shards=8"))

	s.Equal([]string{"head_default_number_of_shards=8 -> head_default_number_of_shards=abc"}, diff.changed)
}

type FeaturesDefaultSuite struct {
	suite.Suite

	registry *prometheus.Registry
}

func TestFeaturesDefaultSuite(t *testing.T) {
	suite.Run(t, new(FeaturesDefaultSuite))
}

func (s *FeaturesDefaultSuite) SetupTest() {
	s.registry = prometheus.NewRegistry()
}

func (s *FeaturesDefaultSuite) setEnv(features, defaults string) {
	s.T().Setenv(featuresEnv, features)
	s.T().Setenv(featuresDefaultEnv, defaults)
}

func (s *FeaturesDefaultSuite) readFeatures() {
	ReadPromPPFeatures(log.NewNopLogger(), noopFlagConfig{}, s.registry)
}

func (s *FeaturesDefaultSuite) requireDifferFromDefault(value string) {
	expected := `
# HELP prompp_features_differ_from_default Whether PROMPP_FEATURES differs from PROMPP_FEATURES_DEFAULT (1) or matches it (0).
# TYPE prompp_features_differ_from_default gauge
prompp_features_differ_from_default ` + value + "\n"

	s.Require().NoError(testutil.GatherAndCompare(s.registry, strings.NewReader(expected), differFromDefaultMetric))
}

func (s *FeaturesDefaultSuite) TestDefaultNotDeclared() {
	s.setEnv("unknown_a", "")
	s.Require().NoError(os.Unsetenv(featuresDefaultEnv))

	s.readFeatures()

	count, err := testutil.GatherAndCount(s.registry, differFromDefaultMetric)
	s.Require().NoError(err)
	s.Equal(0, count)
}

func (s *FeaturesDefaultSuite) TestMatchesDefault() {
	s.setEnv("unknown_a=1, unknown_b", "unknown_b,unknown_a=1")

	s.readFeatures()

	s.requireDifferFromDefault("0")
}

func (s *FeaturesDefaultSuite) TestBothEmptyMatch() {
	s.setEnv("", "")

	s.readFeatures()

	s.requireDifferFromDefault("0")
}

func (s *FeaturesDefaultSuite) TestDiffersFromDefault() {
	s.setEnv("unknown_a=1", "unknown_a=2")

	s.readFeatures()

	s.requireDifferFromDefault("1")
}

func (s *FeaturesDefaultSuite) TestEmptyFeaturesDiffersFromNonEmptyDefault() {
	s.setEnv("", "unknown_a")

	s.readFeatures()

	s.requireDifferFromDefault("1")
}

func (s *FeaturesDefaultSuite) TestEmptyDefaultDiffersFromNonEmptyFeatures() {
	s.setEnv("unknown_a", "")

	s.readFeatures()

	s.requireDifferFromDefault("1")
}

// noopFlagConfig implements FlagConfig.
type noopFlagConfig struct{}

// DisableBlockManagerStorage implements FlagConfig.
func (noopFlagConfig) DisableBlockManagerStorage() {}

// SetLabelReplaceCacheSize implements FlagConfig.
func (noopFlagConfig) SetLabelReplaceCacheSize(int) {}

// SetLabelReplaceRegexCacheSize implements FlagConfig.
func (noopFlagConfig) SetLabelReplaceRegexCacheSize(int) {}

// recordingFlagConfig implements FlagConfig and records the applied label_replace cache sizes.
type recordingFlagConfig struct {
	noopFlagConfig

	labelReplaceCacheSize      int
	labelReplaceRegexCacheSize int
}

// SetLabelReplaceCacheSize implements FlagConfig.
func (c *recordingFlagConfig) SetLabelReplaceCacheSize(size int) {
	c.labelReplaceCacheSize = size
}

// SetLabelReplaceRegexCacheSize implements FlagConfig.
func (c *recordingFlagConfig) SetLabelReplaceRegexCacheSize(size int) {
	c.labelReplaceRegexCacheSize = size
}

type LabelReplaceCacheSizeSuite struct {
	suite.Suite

	cfg *recordingFlagConfig
}

func TestLabelReplaceCacheSizeSuite(t *testing.T) {
	suite.Run(t, new(LabelReplaceCacheSizeSuite))
}

func (s *LabelReplaceCacheSizeSuite) SetupTest() {
	s.cfg = &recordingFlagConfig{}
}

func (s *LabelReplaceCacheSizeSuite) applyFeatures(features string) {
	s.T().Setenv(featuresEnv, features)
	ReadPromPPFeatures(log.NewNopLogger(), s.cfg, prometheus.NewRegistry())
}

func (s *LabelReplaceCacheSizeSuite) TestAbsentKeysKeepEngineDefaults() {
	s.applyFeatures("")

	s.Equal(0, s.cfg.labelReplaceCacheSize)
	s.Equal(0, s.cfg.labelReplaceRegexCacheSize)
}

func (s *LabelReplaceCacheSizeSuite) TestValidCapacityIsAppliedPerKey() {
	s.applyFeatures("label_replace_cache_size=1024,label_replace_regex_cache_size=32")

	s.Equal(1024, s.cfg.labelReplaceCacheSize)
	s.Equal(32, s.cfg.labelReplaceRegexCacheSize)
}

func (s *LabelReplaceCacheSizeSuite) TestZeroPassesThrough() {
	s.applyFeatures("label_replace_cache_size=0,label_replace_regex_cache_size=0")

	s.Equal(0, s.cfg.labelReplaceCacheSize)
	s.Equal(0, s.cfg.labelReplaceRegexCacheSize)
}

func (s *LabelReplaceCacheSizeSuite) TestNegativePassesThrough() {
	s.applyFeatures("label_replace_cache_size=-1,label_replace_regex_cache_size=-8")

	s.Equal(-1, s.cfg.labelReplaceCacheSize)
	s.Equal(-8, s.cfg.labelReplaceRegexCacheSize)
}

func (s *LabelReplaceCacheSizeSuite) TestInvalidValuesKeepPreviousValues() {
	s.applyFeatures("label_replace_cache_size=16,label_replace_regex_cache_size=4")
	s.applyFeatures("label_replace_cache_size=abc,label_replace_regex_cache_size=xyz")

	s.Equal(16, s.cfg.labelReplaceCacheSize)
	s.Equal(4, s.cfg.labelReplaceRegexCacheSize)
}

func (s *LabelReplaceCacheSizeSuite) TestEmptyValueKeepsPreviousValues() {
	s.applyFeatures("label_replace_cache_size=16,label_replace_regex_cache_size=4")
	s.applyFeatures("label_replace_cache_size,label_replace_regex_cache_size")

	s.Equal(16, s.cfg.labelReplaceCacheSize)
	s.Equal(4, s.cfg.labelReplaceRegexCacheSize)
}
