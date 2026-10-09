package featuresflags

import (
	"maps"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/pp-pkg/handler"
	"github.com/prometheus/prometheus/pp-pkg/handler/processor"
	pp_pkg_tsdb "github.com/prometheus/prometheus/pp-pkg/tsdb"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/block"
	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/pp/go/storage/remotewriter"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	prom_runtime "github.com/prometheus/prometheus/util/runtime"
	"github.com/prometheus/prometheus/web"
)

const (
	// msgStr is the key used in structured logging for the message field.
	msgStr = "msg"

	// defaultNumberOfShardsStr is the key used in structured logging for the default number of shards field.
	defaultNumberOfShardsStr = "default_number_of_shards"

	// errStr is the key used in structured logging for the error field.
	errStr = "err"

	// featuresEnv is the environment variable with the applied feature flags.
	featuresEnv = "PROMPP_FEATURES"

	// featuresDefaultEnv is the environment variable with the default feature flags to compare with.
	featuresDefaultEnv = "PROMPP_FEATURES_DEFAULT"
)

// FlagConfig is an interface that allows for the configuration of feature flags in the system.
type FlagConfig interface {
	// DisableBlockManagerStorage disables the storage of blocks in the block manager.
	DisableBlockManagerStorage()

	// SetLabelReplaceCacheSize sets the capacity of the label_replace result cache
	// (0 keeps the engine default, a negative value disables the cache).
	SetLabelReplaceCacheSize(size int)

	// SetLabelReplaceRegexCacheSize sets the capacity of the label_replace compiled-regex cache
	// (0 keeps the engine default, a negative value disables the cache).
	SetLabelReplaceRegexCacheSize(size int)
}

// ReadPromPPFeatures reads the PROMPP_FEATURES environment variable
// and applies the specified feature flags to the system. Unknown options are
// reported and ignored. If PROMPP_FEATURES_DEFAULT is set, the applied features
// are compared with it and the result is exposed via the registerer.
func ReadPromPPFeatures(logger log.Logger, cfg FlagConfig, registerer prometheus.Registerer) {
	var cppFeatures cppbridge.FeatureFlags
	defer func() {
		cppbridge.InitializeFeatureFlags(cppFeatures)
	}()

	logger = log.With(logger, "component", featuresEnv)
	features := parseFeatures(os.Getenv(featuresEnv))
	for _, fname := range slices.Sorted(maps.Keys(features)) {
		applyFeature(logger, cfg, &cppFeatures, fname, features[fname])
	}

	checkFeaturesDefault(logger, registerer, features)
}

// parseFeatures parses a comma-separated list of key[=value] features into a map,
// so that feature sets can be compared regardless of order and whitespace.
func parseFeatures(raw string) map[string]string {
	features := make(map[string]string)
	for feature := range strings.SplitSeq(raw, ",") {
		fname, fvalue, _ := strings.Cut(feature, "=")
		if fname = strings.TrimSpace(fname); fname == "" {
			continue
		}

		features[fname] = strings.TrimSpace(fvalue)
	}

	return features
}

// applyFeature applies a single feature flag to the system.
//
//revive:disable-next-line:cyclomatic // complex logic is necessary for this function
//revive:disable-next-line:function-length // complex logic is necessary for this function
func applyFeature(logger log.Logger, cfg FlagConfig, cppFeatures *cppbridge.FeatureFlags, fname, fvalue string) {
	switch fname {
	case "head_read_concurrency":
		setHeadReadConcurrency(logger, fvalue)

	case "head_default_number_of_shards":
		setHeadDefaultNumberOfShards(logger, fvalue)

	case "disable_commits_on_remote_write":
		setDisableCommitsOnRemoteWrite(logger)

	case "disable_block_compaction":
		pp_pkg_tsdb.BlockCompactionDisabled = true
		_ = level.Info(logger).Log(msgStr, "Prometheus compaction disabled.")

	case "federation_split_families":
		setFederationSplitFamilies(logger, fvalue)

	case "default_sample_age_limit":
		setDefaultSampleAgeLimit(logger, fvalue)

	case "disable_instant_query_feature":
		querier.InstantQueryFeature = false
		_ = level.Info(logger).Log(msgStr, "Instant query feature is disabled.")

	case "disable_remote_write_http2":
		remotewriter.HTTP2Enabled = false
		_ = level.Info(logger).Log(msgStr, "HTTP/2 for remote write is disabled.")

	case "disable_shrink_shard_copier":
		storage.ShrinkShardCopier = false
		_ = level.Info(logger).Log(msgStr, "Shrink shard copier is disabled.")

	case "disable_block_manager":
		cfg.DisableBlockManagerStorage()
		_ = level.Info(logger).Log(
			msgStr, "Block-manager historical storage is disabled; using pre-PR-377 TSDB storage.",
		)

	case "disable_coredumps":
		setDisableCoredumps(logger)

	case "select_func_optimization":
		setSelectFuncOptimization(logger, fvalue)

	case "enable_block_shard_labels":
		block.EnableBlockShardLabels = true
		_ = level.Info(logger).Log(msgStr, "Block shard labels are enabled.")

	case "enable_madvise_random":
		fileutil.EnabledMADVRANDOM = true
		_ = level.Info(logger).Log(msgStr, "MADV_RANDOM for mmaped files is enabled.")

	case "disable_scraper_full_utf8":
		cppFeatures.DisableScraperFullUTF8()
		_ = level.Info(logger).Log(msgStr, "Whole-input UTF-8 validation for scraper is disabled.")

	case "enable_skip_no_samples_series":
		cppFeatures.SkipNoSamplesSeries()
		_ = level.Info(logger).Log(
			msgStr, "Skipping series with no samples instead of throwing an error is enabled.",
		)

	case "enable_wal_writer_v2":
		storage.EnableWalWriterV2()
		_ = level.Info(logger).Log(
			msgStr, "Wal Writer V2 is enabled.",
		)

	case "label_replace_cache_size":
		setLabelReplaceCacheSize(logger, cfg, fvalue)

	case "label_replace_regex_cache_size":
		setLabelReplaceRegexCacheSize(logger, cfg, fvalue)

	default:
		_ = level.Warn(logger).Log(msgStr, "Unknown PROMPP_FEATURES option.", "option", fname)
	}
}

// formatFeature formats a feature back to its key[=value] form.
func formatFeature(fname, fvalue string) string {
	if fvalue == "" {
		return fname
	}

	return fname + "=" + fvalue
}

// setHeadReadConcurrency sets the concurrency level for reading from the head based on the provided feature value.
func setHeadReadConcurrency(logger log.Logger, fvalue string) {
	var (
		v   = 1
		err error
	)

	if fvalue = strings.TrimSpace(fvalue); fvalue != "" {
		v, err = strconv.Atoi(fvalue)
		if err != nil {
			_ = level.Error(logger).Log(
				msgStr, "Error parsing head_read_concurrency value",
				errStr, err,
			)
			return
		}
	}

	head.ExtraWorkers = v
	_ = level.Info(logger).Log(
		msgStr, "Concurrency reading is enabled.",
		"extra", v,
	)
}

// setHeadDefaultNumberOfShards sets the default number of shards for the head based on the provided feature value.
func setHeadDefaultNumberOfShards(logger log.Logger, fvalue string) {
	fvalue = strings.TrimSpace(fvalue)
	if fvalue == "" {
		_ = level.Error(logger).Log(
			msgStr, "The default number of shards is empty, no changes.",
			defaultNumberOfShardsStr, storage.DefaultNumberOfShards,
		)

		return
	}

	v, err := strconv.Atoi(fvalue)
	switch {
	case err != nil:
		_ = level.Error(logger).Log(
			msgStr, "Error parsing head_default_number_of_shards value",
			defaultNumberOfShardsStr, storage.DefaultNumberOfShards,
			errStr, err,
		)

	case v > math.MaxUint16:
		_ = level.Error(logger).Log(
			msgStr, "The default number of shards is overflow(max 65535), no changes.",
			defaultNumberOfShardsStr, storage.DefaultNumberOfShards,
		)

	case v < 1:
		_ = level.Error(logger).Log(
			msgStr, "The default number of shards is incorrect(min 1), no changes.",
			defaultNumberOfShardsStr, storage.DefaultNumberOfShards,
		)

	default:
		storage.DefaultNumberOfShards = uint16(v)
		_ = level.Info(logger).Log(
			msgStr, "Changed default number of shards.",
			defaultNumberOfShardsStr, storage.DefaultNumberOfShards,
		)
	}
}

// setDisableCommitsOnRemoteWrite disables commits on remote write and logs the action.
func setDisableCommitsOnRemoteWrite(logger log.Logger) {
	processor.AlwaysCommit = false
	handler.OTLPAlwaysCommit = false
	_ = level.Info(logger).Log(msgStr, "Disabled commits on remote write.")
}

// setFederationSplitFamilies sets the federation split families page size based on the provided feature value.
func setFederationSplitFamilies(logger log.Logger, fvalue string) {
	fvalue = strings.TrimSpace(fvalue)
	if fvalue == "" {
		_ = level.Error(logger).Log(
			msgStr, "The federation_split_families should be setted with number.",
		)
		return
	}

	v, err := strconv.Atoi(fvalue)
	if err != nil {
		_ = level.Error(logger).Log(
			msgStr, "Error parsing federation_split_families value",
			errStr, err,
		)
		return
	}

	_ = level.Info(logger).Log(
		msgStr, "Split federation families with pages.",
		"pages", v,
	)
	web.FederationSplitFamiliesPageSize = v
}

// setDefaultSampleAgeLimit sets the default sample age limit for remote write based on the provided feature value.
func setDefaultSampleAgeLimit(logger log.Logger, fvalue string) {
	fvalue = strings.TrimSpace(fvalue)
	defaultSampleAgeLimit, err := model.ParseDuration(fvalue)
	if err != nil {
		_ = level.Error(logger).Log(
			msgStr, "Error parsing default_sample_age_limit value",
			errStr, err,
		)
		return
	}

	_ = level.Info(logger).Log(
		msgStr, "default_sample_age_limit is set.",
		"limit", fvalue,
	)

	remotewriter.DefaultSampleAgeLimit = defaultSampleAgeLimit
}

// setDisableCoredumps disables core dumps for the application and logs the result.
func setDisableCoredumps(logger log.Logger) {
	if err := prom_runtime.DisableCoreDumps(); err != nil {
		_ = level.Error(logger).Log(msgStr, "Failed to disable core dumps.", "err", err)

		return
	}

	_ = level.Info(logger).Log(msgStr, "Core dumps are disabled (RLIMIT_CORE=0).")
}

// setSelectFuncOptimization sets the select function optimization for the querier based on the provided feature value.
func setSelectFuncOptimization(logger log.Logger, fvalue string) {
	if err := querier.SetSelectFuncOptimize(strings.TrimSpace(fvalue)); err != nil {
		_ = level.Error(logger).Log(
			msgStr, "Error parsing select_func_optimization value",
			errStr, err,
		)

		return
	}

	_ = level.Info(logger).Log(
		msgStr, "Select function optimization is set.",
		"optimization", fvalue,
	)
}

// setLabelReplaceCacheSize sets the label_replace result cache capacity based on the provided feature value.
func setLabelReplaceCacheSize(logger log.Logger, cfg FlagConfig, fvalue string) {
	v, err := strconv.Atoi(strings.TrimSpace(fvalue))
	if err != nil {
		_ = level.Error(logger).Log(
			msgStr, "Error parsing label_replace_cache_size value",
			errStr, err,
		)
		return
	}

	cfg.SetLabelReplaceCacheSize(v)
	_ = level.Info(logger).Log(
		msgStr, "Label replace cache size is set.",
		"size", v,
	)
}

// setLabelReplaceRegexCacheSize sets the label_replace compiled-regex cache capacity
// based on the provided feature value.
func setLabelReplaceRegexCacheSize(logger log.Logger, cfg FlagConfig, fvalue string) {
	v, err := strconv.Atoi(strings.TrimSpace(fvalue))
	if err != nil {
		_ = level.Error(logger).Log(
			msgStr, "Error parsing label_replace_regex_cache_size value",
			errStr, err,
		)
		return
	}

	cfg.SetLabelReplaceRegexCacheSize(v)
	_ = level.Info(logger).Log(
		msgStr, "Label replace regex cache size is set.",
		"size", v,
	)
}

//
// featuresDiff
//

// featuresDiff describes the difference between the applied and the default feature sets.
type featuresDiff struct {
	// added contains features present only in the applied set.
	added []string
	// removed contains features present only in the default set.
	removed []string
	// changed contains features present in both sets with different values.
	changed []string
}

// isEmpty reports whether the feature sets are equal.
func (d featuresDiff) isEmpty() bool {
	return len(d.added) == 0 && len(d.removed) == 0 && len(d.changed) == 0
}

// checkFeaturesDefault compares the applied features with PROMPP_FEATURES_DEFAULT, if it is set,
// and reports the result via the prompp_features_differ_from_default gauge.
func checkFeaturesDefault(logger log.Logger, registerer prometheus.Registerer, features map[string]string) {
	rawDefault, ok := os.LookupEnv(featuresDefaultEnv)
	if !ok {
		return
	}

	differFromDefault := util.NewUnconflictRegisterer(registerer).NewGauge(prometheus.GaugeOpts{
		Name: "prompp_features_differ_from_default",
		Help: "Whether PROMPP_FEATURES differs from PROMPP_FEATURES_DEFAULT (1) or matches it (0).",
	})

	diff := diffFeatures(features, parseFeatures(rawDefault))
	if diff.isEmpty() {
		differFromDefault.Set(0)
		return
	}

	differFromDefault.Set(1)
	_ = level.Warn(logger).Log(
		msgStr, "PROMPP_FEATURES differs from PROMPP_FEATURES_DEFAULT.",
		"added", strings.Join(diff.added, ","),
		"removed", strings.Join(diff.removed, ","),
		"changed", strings.Join(diff.changed, ","),
	)
}

// diffFeatures returns the difference between the applied and the default feature sets.
func diffFeatures(features, defaults map[string]string) featuresDiff {
	var diff featuresDiff
	for _, fname := range slices.Sorted(maps.Keys(features)) {
		fvalue := features[fname]
		defaultValue, ok := defaults[fname]
		switch {
		case !ok:
			diff.added = append(diff.added, formatFeature(fname, features[fname]))
		case normalizeFeatureValue(fname, defaultValue) != normalizeFeatureValue(fname, fvalue):
			diff.changed = append(diff.changed, formatFeature(fname, defaultValue)+" -> "+formatFeature(fname, fvalue))
		}
	}

	for _, fname := range slices.Sorted(maps.Keys(defaults)) {
		if _, ok := features[fname]; !ok {
			diff.removed = append(diff.removed, formatFeature(fname, defaults[fname]))
		}
	}

	return diff
}

// normalizeFeatureValue returns the canonical form of the feature value, so that values
// with the same meaning (e.g. 1h and 60m) are compared as equal. Unknown features and
// unparsable values are returned as is.
func normalizeFeatureValue(fname, fvalue string) string {
	switch fname {
	case "head_read_concurrency":
		if fvalue == "" {
			return "1"
		}

		return normalizeIntValue(fvalue)

	case "head_default_number_of_shards", "federation_split_families",
		"label_replace_cache_size", "label_replace_regex_cache_size":
		return normalizeIntValue(fvalue)

	case "default_sample_age_limit":
		d, err := model.ParseDuration(fvalue)
		if err != nil {
			return fvalue
		}

		return d.String()

	default:
		return fvalue
	}
}

// normalizeIntValue returns the canonical form of the integer value, or the value as is if it is not an integer.
func normalizeIntValue(fvalue string) string {
	v, err := strconv.Atoi(fvalue)
	if err != nil {
		return fvalue
	}

	return strconv.Itoa(v)
}
