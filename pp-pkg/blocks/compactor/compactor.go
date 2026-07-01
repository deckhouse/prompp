package compactor

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp-pkg/blocks/block"
	"github.com/prometheus/prometheus/pp-pkg/blocks/lcompactor"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// Options configures the persisted-blocks compactor.
type Options struct {
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

// Compactor compacts persisted on-disk blocks. It does not run its own loop and
// does not reload or delete blocks: a single driver goroutine (the block
// Manager) calls Compact once per tick, right after reloading. Running compact
// and reload in that one goroutine means a compaction never races with the
// deletion of its inputs the parents created by one tick's compaction are
// loaded and deleted by the next tick's reload before the next plan is computed
// (mirroring tsdb's single-goroutine compact/reload loop).
type Compactor struct {
	dir       string
	compactor *lcompactor.LeveledCompactor
	logger    log.Logger
	metrics   *compactorMetrics
}

// NewCompactor builds a LeveledCompactor from opts. It does not start any
// background goroutine; the caller drives compaction via Compact (typically the
// block Manager's reload loop after calling Manager.SetCompactor).
func NewCompactor(
	ctx context.Context,
	dir string,
	opts *Options,
	logger log.Logger,
	r prometheus.Registerer,
) (*Compactor, error) {
	if opts == nil {
		opts = &Options{}
	}
	if logger == nil {
		logger = log.NewNopLogger()
	}

	minBlockDuration := opts.MinBlockDuration
	if minBlockDuration <= 0 {
		minBlockDuration = block.DefaultBlockDuration
	}

	rngs := lcompactor.CompactionRanges(minBlockDuration, opts.MaxBlockDuration)
	leveled, err := lcompactor.NewLeveledCompactorWithOptions(
		ctx,
		r,
		logger,
		rngs,
		chunkenc.NewPool(),
		lcompactor.LeveledCompactorOptions{
			MaxBlockChunkSegmentSize:    opts.MaxBlockChunkSegmentSize,
			EnableOverlappingCompaction: opts.EnableOverlappingCompaction,
		})
	if err != nil {
		return nil, fmt.Errorf("create compactor: %w", err)
	}

	return &Compactor{
		dir:       dir,
		compactor: leveled,
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
func (c *Compactor) Compact(open []*block.Block) (uids []ulid.ULID, err error) {
	logger := c.loggerOrNop()
	c.metrics.compactionsTriggered.Inc()

	plan, err := c.compactor.Plan(c.dir)
	if err != nil {
		c.metrics.compactionsFailed.Inc()
		return nil, fmt.Errorf("plan compaction: %w", err)
	}

	if len(plan) == 0 {
		return nil, nil
	}

	start := time.Now()

	_ = level.Info(logger).Log(
		"msg", "starting on-disk block compaction",
		"plan_len", len(plan),
		"plan", fmt.Sprintf("%v", plan),
		"open_blocks", len(open),
	)

	uids, err = c.compactor.Compact(c.dir, plan, open)
	if err != nil {
		c.metrics.compactionsFailed.Inc()
		return nil, fmt.Errorf("compact %v: %w", plan, err)
	}

	_ = level.Info(logger).Log(
		"msg", "finished on-disk block compaction",
		"plan_len", len(plan),
		"plan", fmt.Sprintf("%v", plan),
		"open_blocks", len(open),
		"result_blocks", len(uids),
		"duration", time.Since(start),
	)

	return uids, nil
}

// OverlappingBlocks returns all overlapping blocks from given meta files.
//
//revive:disable-next-line:cyclomatic // this is a complex algorithm
//revive:disable-next-line:cognitive-complexity // this is a complex algorithm
//revive:disable-next-line:function-length // this is a complex algorithm
func (*Compactor) OverlappingBlocks(open []*block.Block) (block.Overlaps, error) {
	if len(open) <= 1 {
		return nil, nil
	}

	var (
		overlaps [][]tsdb.BlockMeta
		// pending contains not ended blocks in regards to "current" timestamp.
		pending = []tsdb.BlockMeta{open[0].Meta()}
		// continuousPending helps to aggregate same overlaps to single group.
		continuousPending = true
	)

	// We have here blocks sorted by minTime.
	// We iterate over each block and treat its minTime as our "current" timestamp.
	// We check if any of the pending block finished (blocks that we have seen before,
	// but their maxTime was still ahead current timestamp).
	// If not, it means they overlap with our current block. In the same time current block is assumed pending.
	for _, b := range open[1:] {
		var newPending []tsdb.BlockMeta
		meta := b.Meta()
		for j := range pending {
			// "b.MinTime" is our current time.
			if meta.MinTime >= pending[j].MaxTime {
				continuousPending = false
				continue
			}

			// "p" overlaps with "b" and "p" is still pending.
			newPending = append(newPending, pending[j])
		}

		// Our block "b" is now pending.
		pending = append(newPending, meta) //nolint:gocritic // appendAssign: reuse at next iteration
		if len(newPending) == 0 {
			// No overlaps.
			continue
		}

		if continuousPending && len(overlaps) > 0 {
			overlaps[len(overlaps)-1] = append(overlaps[len(overlaps)-1], meta)
			continue
		}
		overlaps = append(overlaps, append(newPending, meta))
		// Start new pendings.
		continuousPending = true
	}

	// Fetch the critical overlapped time range foreach overlap groups.
	overlapGroups := block.Overlaps{}
	for _, overlap := range overlaps {
		minRange := block.TimeRange{Min: 0, Max: math.MaxInt64}
		for j := range overlap {
			if minRange.Max > overlap[j].MaxTime {
				minRange.Max = overlap[j].MaxTime
			}

			if minRange.Min < overlap[j].MinTime {
				minRange.Min = overlap[j].MinTime
			}
		}
		overlapGroups[minRange] = overlap
	}

	return overlapGroups, nil
}

// loggerOrNop returns the logger or a no-op logger if the logger is nil.
func (c *Compactor) loggerOrNop() log.Logger {
	if c.logger == nil {
		return log.NewNopLogger()
	}

	return c.logger
}

//
// metrics
//

// compactorMetrics collects metrics for the compactor.
type compactorMetrics struct {
	compactionsTriggered prometheus.Counter
	compactionsFailed    prometheus.Counter
}

// newCompactorMetrics creates new [compactorMetrics] for the compactor.
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
