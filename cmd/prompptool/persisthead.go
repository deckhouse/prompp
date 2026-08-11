package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/grafana/regexp"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/pp-pkg/featuresflags"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/block"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
)

// shardWalFilePattern matches per-shard wal files (e.g. shard_0.wal) inside a head directory.
var shardWalFilePattern = regexp.MustCompile(`^shard_(\d+)\.wal$`)

type cmdPersistHead struct {
	headPath       string
	outputDir      string
	blockDuration  model.Duration
	numberOfShards uint16
}

func registerCmdPersistHead(cmd *cmdPersistHead, clause *kingpin.CmdClause) {
	clause.Arg("head-path", "Path to the head directory to persist (the UUID-named directory).").
		Required().
		ExistingDirVar(&cmd.headPath)

	clause.Flag("output-dir", "Directory to write TSDB blocks into. Default: parent directory of head-path.").
		StringVar(&cmd.outputDir)

	clause.Flag("storage.tsdb.min-block-duration", "Minimum duration of a data block before being persisted.").
		Default("2h").SetValue(&cmd.blockDuration)

	clause.Flag("number-of-shards", "Number of shards in the head. 0 means autodetect by shard_*.wal files.").
		Default("0").Uint16Var(&cmd.numberOfShards)
}

// Do loads a single head directly by path (without consulting head.log) and writes TSDB blocks.
func (cmd *cmdPersistHead) Do(
	ctx context.Context,
	logger log.Logger,
	registerer prometheus.Registerer,
) error {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	headPath, err := filepath.Abs(cmd.headPath)
	if err != nil {
		return err
	}

	dataDir := filepath.Dir(headPath)
	headID := filepath.Base(headPath)

	featuresflags.ReadPromPPFeatures(logger, noopFlagConfig{})

	// The loader resolves the head directory as filepath.Join(dataDir, record.ID()),
	// so the directory name must be a valid head UUID.
	id, err := uuid.Parse(headID)
	if err != nil {
		return fmt.Errorf("head directory name %q is not a valid head UUID: %w", headID, err)
	}

	numberOfShards := cmd.numberOfShards
	if numberOfShards == 0 {
		if numberOfShards, err = detectNumberOfShards(headPath); err != nil {
			return err
		}
	}
	if numberOfShards == 0 {
		return fmt.Errorf("no shard_*.wal files found in %s; pass --number-of-shards explicitly", headPath)
	}

	outputDir := cmd.outputDir
	if outputDir == "" {
		outputDir = dataDir
	}
	if err = os.MkdirAll(outputDir, 0o755); err != nil { //nolint:gosec,mnd // standard data dir permissions
		return fmt.Errorf("failed to create output dir %s: %w", outputDir, err)
	}

	level.Info(logger).Log(
		"msg", "persisting head",
		"id", id.String(),
		"head_path", headPath,
		"output_dir", outputDir,
		"shards", numberOfShards,
	)

	now := time.Now().UnixMilli()
	record := catalog.NewRecordWithData(
		id,
		numberOfShards,
		now, // createdAt
		now, // updatedAt
		0,   // deletedAt
		false,
		0, // referenceCount
		catalog.StatusRotated,
		nil, // lastAppendedSegmentID
	)

	// LoadReadOnly always returns a head, even on error; a non-fatal error means the
	// head is (partially) corrupted but we can still persist whatever decoded.
	loader := storage.NewLoader(dataDir, 0, registerer, time.Duration(0))
	h, err := loader.LoadReadOnly(record, 0)
	if err != nil && !errors.Is(err, cppbridge.ErrInvalidEncoderVersion) {
		level.Warn(logger).Log("msg", "head is corrupted, persisting decoded data only", "id", id.String(), "err", err)
	}
	defer func() {
		if closeErr := h.Close(); closeErr != nil {
			level.Error(logger).Log("msg", "failed to close head", "id", id.String(), "err", closeErr)
		}
	}()

	bw := block.NewWriter[*shard.Shard](
		outputDir,
		block.DefaultChunkSegmentSize,
		cppbridge.NoDownsampling,
		time.Duration(cmd.blockDuration),
		0, // no retention filtering: persist all heads regardless of age
		clockwork.NewRealClock(),
		registerer,
	)

	total := 0
	for sd := range h.RangeShards() {
		if err = ctx.Err(); err != nil {
			return err
		}

		writtenBlocks, writeErr := bw.Write(sd, numberOfShards)
		if writeErr != nil {
			return fmt.Errorf("failed to write tsdb block [id: %s, dir: %s]: %w", id.String(), headID, writeErr)
		}

		for i := range writtenBlocks {
			total++
			level.Info(logger).Log(
				"msg", "block written",
				"ulid", writtenBlocks[i].Meta.ULID.String(),
				"dir", writtenBlocks[i].Dir,
				"min_time", writtenBlocks[i].Meta.MinTime,
				"max_time", writtenBlocks[i].Meta.MaxTime,
				"series", writtenBlocks[i].Meta.Stats.NumSeries,
				"samples", writtenBlocks[i].Meta.Stats.NumSamples,
			)
		}
	}

	level.Info(logger).Log("msg", "head persisted", "id", id.String(), "blocks", total)

	return nil
}

// detectNumberOfShards counts contiguous shard_<n>.wal files in the head directory.
func detectNumberOfShards(headPath string) (uint16, error) {
	entries, err := os.ReadDir(headPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read head dir %s: %w", headPath, err)
	}

	maxShardID := -1
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		m := shardWalFilePattern.FindStringSubmatch(entry.Name())
		if m == nil {
			continue
		}
		shardID, convErr := strconv.Atoi(m[1])
		if convErr != nil {
			continue
		}
		if shardID > maxShardID {
			maxShardID = shardID
		}
	}

	if maxShardID < 0 {
		return 0, nil
	}

	return uint16(maxShardID + 1), nil //nolint:gosec // shard count fits uint16
}

//
// noopFlagConfig
//

// noopFlagConfig is a no-op implementation of the FlagConfig interface, used when no feature flags are set.
type noopFlagConfig struct{}

// DisableBlockManagerStorage is a no-op implementation of the FlagConfig interface, used when no feature flags are set.
func (noopFlagConfig) DisableBlockManagerStorage() {}
