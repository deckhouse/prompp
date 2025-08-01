package main

import (
	"context"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"os"
	"path/filepath"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/block"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/relabeler/head"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/catalog"
)

type cmdWALPPToBlock struct {
	blockDuration        model.Duration
	updateCatalog        bool
	removeConvertedHeads bool
}

func registerCmdWALPPToBlock(cmd *cmdWALPPToBlock, clause *kingpin.CmdClause) {
	clause.Flag("storage.tsdb.min-block-duration", "Minimum duration of a data block before being persisted. For use in testing.").
		Default("2h").SetValue(&cmd.blockDuration)

	clause.Flag(
		"update-catalog",
		"Update catalog after conversion. Default true.",
	).Default("true").Hidden().BoolVar(&cmd.updateCatalog)

	clause.Flag(
		"remove-converted-heads",
		"After conversion, delete all converted heads. Default true.",
	).Default("true").Hidden().BoolVar(&cmd.removeConvertedHeads)
}

func (cmd *cmdWALPPToBlock) Do(
	ctx context.Context,
	workingDir string,
	logger log.Logger,
	registerer prometheus.Registerer,
) error {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	var err error
	if workingDir, err = filepath.Abs(workingDir); err != nil {
		return err
	}

	level.Debug(logger).Log("msg", "read file log")
	fileLog, err := catalog.NewFileLogV2(filepath.Join(workingDir, "head.log"))
	if err != nil {
		return fmt.Errorf("failed init file log reader: %w", err)
	}

	level.Debug(logger).Log("msg", "read catalog log")
	clock := clockwork.NewRealClock()
	headCatalog, err := catalog.New(clock, fileLog, catalog.DefaultIDGenerator{}, catalog.DefaultMaxLogFileSize, registerer)
	if err != nil {
		return fmt.Errorf("failed init head catalog: %w", err)
	}
	headRecords, err := headCatalog.List(
		func(record *catalog.Record) bool {
			return record.DeletedAt() == 0 &&
				(record.Status() == catalog.StatusNew || record.Status() == catalog.StatusActive || record.Status() == catalog.StatusRotated)
		},
		func(lhs, rhs *catalog.Record) bool {
			return lhs.CreatedAt() < rhs.CreatedAt()
		},
	)
	if err != nil {
		return fmt.Errorf("failed listed head catalog: %w", err)
	}
	level.Debug(logger).Log("msg", "catalog records", "len", len(headRecords))

	var inputRelabelerConfig []*config.InputRelabelerConfig
	bw := block.NewBlockWriter(workingDir, block.DefaultChunkSegmentSize, time.Duration(cmd.blockDuration), registerer)
	for _, headRecord := range headRecords {
		if err := ctx.Err(); err != nil {
			return err
		}
		level.Debug(logger).Log("msg", "load head", "id", headRecord.ID(), "dir", headRecord.Dir())
		h, _, _, err := head.Load(
			headRecord.ID(),
			0,
			filepath.Join(workingDir, headRecord.Dir()),
			inputRelabelerConfig,
			headRecord.NumberOfShards(),
			0,
			head.NoOpLastAppendedSegmentIDSetter{},
			registerer,
		)
		if err != nil {
			level.Error(logger).Log(
				"msg", "failed to load head",
				"id", headRecord.ID(),
				"dir", headRecord.Dir(),
				"err", err,
			)
			return err
		}
		h.Stop()

		level.Debug(logger).Log("msg", "write block", "id", headRecord.ID(), "dir", headRecord.Dir())

		tBlockWrite := h.CreateTask(
			relabeler.BlockWrite,
			func(shard relabeler.Shard) error {
				shard.LSSLock()
				defer shard.LSSUnlock()

				_, err := bw.Write(shard.DataStorage().Raw(), shard.UnloadedDataStorage(), shard.LSS().Raw(), cppbridge.UnlimitedLsIdBatchSize)
				return err
			},
			relabeler.ForLSSTask,
		)
		h.Enqueue(tBlockWrite)
		if err = tBlockWrite.Wait(); err != nil {
			return fmt.Errorf("failed to write tsdb block [id: %s, dir: %s]: %w", headRecord.ID(), headRecord.Dir(), err)
		}

		if cmd.updateCatalog {
			level.Debug(logger).Log("msg", "set status persisted block", "id", headRecord.ID(), "dir", headRecord.Dir())
			if _, setStatusErr := headCatalog.SetStatus(headRecord.ID(), catalog.StatusPersisted); setStatusErr != nil {
				return fmt.Errorf("failed to set catalog status Persisted: %w", setStatusErr)
			}
		}

		if err = h.Close(); err != nil {
			level.Error(logger).Log(
				"msg", "failed close head",
				"id", headRecord.ID(),
				"dir", headRecord.Dir(),
				"err", err,
			)
		}
	}

	if cmd.removeConvertedHeads {
		if err := cmd.clearing(ctx, workingDir, headCatalog, logger); err != nil {
			return fmt.Errorf("failed clearing catalog: %w", err)
		}

		level.Debug(logger).Log("msg", "run compact catalog")
		if err := headCatalog.Compact(); err != nil {
			return fmt.Errorf("compact catalog: %w", err)
		}
	}

	return fileLog.Close()
}

func (cmd *cmdWALPPToBlock) clearing(
	ctx context.Context,
	workingDir string,
	headCatalog *catalog.Catalog,
	logger log.Logger,
) error {
	level.Debug(logger).Log("msg", "catalog clearing: started")
	defer func() {
		level.Debug(logger).Log("msg", "catalog clearing: ended")
	}()

	records, err := headCatalog.List(
		func(record *catalog.Record) bool {
			return record.DeletedAt() == 0 && record.Status() == catalog.StatusPersisted
		},
		func(lhs, rhs *catalog.Record) bool {
			return lhs.CreatedAt() < rhs.CreatedAt()
		},
	)
	if err != nil {
		return fmt.Errorf("failed listed head catalog: %w", err)
	}

	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return err
		}

		if record.DeletedAt() != 0 {
			continue
		}

		level.Debug(logger).Log("msg", "catalog clearing", "head", record.ID())

		if record.Corrupted() {
			level.Debug(logger).Log(
				"msg", "remove corrupted head",
				"head", record.ID(),
			)
		}

		if err = os.RemoveAll(filepath.Join(workingDir, record.Dir())); err != nil {
			level.Error(logger).Log(
				"msg", "failed to delete head directory",
				"id", record.ID(),
				"dir", record.Dir(),
				"err", err,
			)
			continue
		}

		if err = headCatalog.Delete(record.ID()); err != nil {
			level.Error(logger).Log(
				"msg", "failed to delete head record",
				"id", record.ID(),
				"dir", record.Dir(),
				"err", err,
			)
			continue
		}

		level.Debug(logger).Log(
			"msg", "catalog clearing: started",
			"head", record.ID(),
			"state", "removed",
		)
	}

	return nil
}
