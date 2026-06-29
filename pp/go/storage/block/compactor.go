package block

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// CompactorOptions configures the persisted-blocks compactor.
type CompactorOptions struct {
	// MinBlockDuration is the smallest block range, used to derive the
	// exponential compaction ranges. If zero, tsdb.DefaultBlockDuration is used.
	MinBlockDuration int64
	// MaxBlockDuration limits the largest compaction range. If zero, no limit is
	// applied and all exponential ranges are used.
	MaxBlockDuration int64
	// MaxBlockChunkSegmentSize is the max block chunk segment size.
	MaxBlockChunkSegmentSize int64
	// EnableOverlappingCompaction enables compaction of overlapping blocks.
	EnableOverlappingCompaction bool
}

// BlockSource provides the compactor with the currently loaded blocks. It is
// implemented by Manager.
type BlockSource interface {
	// Blocks returns a snapshot of the currently loaded blocks (the open
	// argument for Compact).
	Blocks() []*tsdb.Block
}

// Compactor compacts persisted on-disk blocks. It does not run its own loop and
// does not reload or delete blocks: a single driver goroutine (the block
// Manager) calls Compact once per tick, right after reloading. Running compact
// and reload in that one goroutine means a compaction never races with the
// deletion of its inputs the parents created by one tick's compaction are
// loaded and deleted by the next tick's reload before the next plan is computed
// (mirroring tsdb's single-goroutine compact/reload loop).
type Compactor struct {
	dir       string
	compactor tsdb.Compactor
	source    BlockSource
	logger    log.Logger
	metrics   *compactorMetrics
}

// NewCompactor builds a LeveledCompactor from opts. It does not start any
// background goroutine; the caller drives compaction via Compact (typically the
// block Manager's reload loop after calling Manager.SetCompactor).
func NewCompactor(
	ctx context.Context,
	dir string,
	opts *CompactorOptions,
	source BlockSource,
	logger log.Logger,
	r prometheus.Registerer,
) (*Compactor, error) {
	if opts == nil {
		opts = &CompactorOptions{}
	}
	if logger == nil {
		logger = log.NewNopLogger()
	}

	minBlockDuration := opts.MinBlockDuration
	if minBlockDuration <= 0 {
		minBlockDuration = tsdb.DefaultBlockDuration
	}

	rngs := compactionRanges(minBlockDuration, opts.MaxBlockDuration)
	leveled, err := tsdb.NewLeveledCompactorWithOptions(ctx, r, logger, rngs, chunkenc.NewPool(), tsdb.LeveledCompactorOptions{
		MaxBlockChunkSegmentSize:    opts.MaxBlockChunkSegmentSize,
		EnableOverlappingCompaction: opts.EnableOverlappingCompaction,
	})
	if err != nil {
		return nil, fmt.Errorf("create compactor: %w", err)
	}

	return &Compactor{
		dir:       dir,
		compactor: leveled,
		source:    source,
		logger:    logger,
		metrics:   newCompactorMetrics(r),
	}, nil
}

// Compact runs a single compaction pass: it plans one group of eligible on-disk
// blocks and compacts them. It reports whether a compaction was performed (so the
// driver can immediately reload and compact again until nothing is left) and the
// ULIDs of the blocks it created (so the driver can remove them if the following
// reload fails). It does NOT reload or delete blocks; the driver reloads between
// passes, which loads the new block and deletes the now-obsolete parents before
// the next plan. Compact must be driven by a single goroutine so it never races
// with block deletion.
func (c *Compactor) Compact() (uids []ulid.ULID, compacted bool, err error) {
	logger := c.loggerOrNop()
	c.metrics.compactionsTriggered.Inc()

	plan, err := c.compactor.Plan(c.dir)
	if err != nil {
		c.metrics.compactionsFailed.Inc()
		return nil, false, fmt.Errorf("plan compaction: %w", err)
	}
	if len(plan) == 0 {
		return nil, false, nil
	}

	openBlocks := c.source.Blocks()
	start := time.Now()
	level.Info(logger).Log(
		"msg", "starting on-disk block compaction",
		"plan_len", len(plan),
		"plan", fmt.Sprintf("%v", plan),
		"open_blocks", len(openBlocks),
	)

	uids, err = c.compactor.Compact(c.dir, plan, openBlocks)
	if err != nil {
		c.metrics.compactionsFailed.Inc()
		return nil, false, fmt.Errorf("compact %v: %w", plan, err)
	}
	level.Info(logger).Log(
		"msg", "finished on-disk block compaction",
		"plan_len", len(plan),
		"plan", fmt.Sprintf("%v", plan),
		"open_blocks", len(openBlocks),
		"result_blocks", len(uids),
		"duration", time.Since(start),
	)
	return uids, true, nil
}

func (c *Compactor) loggerOrNop() log.Logger {
	if c.logger == nil {
		return log.NewNopLogger()
	}
	return c.logger
}

func compactionRanges(minBlockDuration, maxBlockDuration int64) []int64 {
	if maxBlockDuration > 0 && maxBlockDuration < minBlockDuration {
		maxBlockDuration = minBlockDuration
	}

	rngs := tsdb.ExponentialBlockRanges(minBlockDuration, 10, 3)
	if maxBlockDuration <= 0 {
		return rngs
	}

	for i, v := range rngs {
		if v > maxBlockDuration {
			return rngs[:i]
		}
	}
	return rngs
}

//
// metrics
//

type compactorMetrics struct {
	compactionsTriggered prometheus.Counter
	compactionsFailed    prometheus.Counter
}

func newCompactorMetrics(r prometheus.Registerer) *compactorMetrics {
	m := &compactorMetrics{
		compactionsTriggered: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_compactions_triggered_total",
			Help: "Total number of triggered compactions for the partition.",
		}),
		compactionsFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_compactions_failed_total",
			Help: "Total number of compactions that failed for the partition.",
		}),
	}

	if r != nil {
		r.MustRegister(
			m.compactionsTriggered,
			m.compactionsFailed,
		)
	}

	return m
}
