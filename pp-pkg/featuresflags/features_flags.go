package featuresflags

import (
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
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
)

// FlagConfig is an interface that allows for the configuration of feature flags in the system.
type FlagConfig interface {
	// DisableBlockManagerStorage disables the storage of blocks in the block manager.
	DisableBlockManagerStorage()
}

// ReadPromPPFeatures reads the PROMPP_FEATURES environment variable
// and applies the specified feature flags to the system. Unknown options are
// reported and ignored.
//
//revive:disable-next-line:cyclomatic // complex logic is necessary for this function
//revive:disable-next-line:function-length // complex logic is necessary for this function
func ReadPromPPFeatures(logger log.Logger, cfg FlagConfig) {
	features := os.Getenv("PROMPP_FEATURES")
	var cppFeatures cppbridge.FeatureFlags
	defer func() {
		cppbridge.InitializeFeatureFlags(cppFeatures)
	}()
	if features == "" {
		return
	}

	logger = log.With(logger, "component", "PROMPP_FEATURES")
	for feature := range strings.SplitSeq(features, ",") {
		fname, fvalue, _ := strings.Cut(feature, "=")
		switch strings.TrimSpace(fname) {
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

		default:
			_ = level.Warn(logger).Log(msgStr, "Unknown PROMPP_FEATURES option.", "option", strings.TrimSpace(fname))
		}
	}

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
			msgStr, "Error parsing head_numbehead_default_number_of_shardsr_of_shards value",
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
