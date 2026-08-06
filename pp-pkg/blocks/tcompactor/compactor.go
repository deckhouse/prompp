package tcompactor

import (
	"context"
	"fmt"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/thanos-io/thanos/pkg/block/metadata"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp-pkg/blocks/block"
	"github.com/prometheus/prometheus/pp-pkg/blocks/lcompactor"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// Planner returns blocks to compact.
type Planner interface {
	// Plan returns a list of blocks that should be compacted into single one.
	// The blocks can be overlapping. The provided metadata has to be ordered by minTime.
	Plan(ctx context.Context, metasByMinTime []*metadata.Meta) ([]*metadata.Meta, bool, error)
}

//
// Options
//

// Options are the options for the [TCompactor].
type Options struct {
	// TsdbOptions are the options for the [lcompactor.LeveledCompactor].
	TsdbOptions lcompactor.LeveledCompactorOptions

	// MinBlockDuration is the smallest block range, used to derive the
	// exponential compaction ranges. If zero, tsdb.DefaultBlockDuration is used.
	MinBlockDuration int64

	// MaxBlockDuration limits the largest compaction range. If zero, no limit is
	// applied and all exponential ranges are used.
	MaxBlockDuration int64
}

//
// TCompactor
//

// TCompactor is the compactor copied from the Thanos compactor.
type TCompactor struct {
	ctx            context.Context
	dir            string
	lCompactor     *lcompactor.LeveledCompactor
	grouper        *DefaultGrouper
	planner        Planner
	blockPopulator lcompactor.BlockPopulator
	metrics        *metrics
}

// NewTCompactor creates a new [TCompactor].
func NewTCompactor(
	ctx context.Context,
	logger log.Logger,
	dir string,
	opts Options,
	chunkPool chunkenc.Pool,
	reg prometheus.Registerer,
) (*TCompactor, error) {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	minBlockDuration := opts.MinBlockDuration
	if minBlockDuration <= 0 {
		minBlockDuration = block.DefaultBlockDuration
	}

	rngs := lcompactor.CompactionRanges(minBlockDuration, opts.MaxBlockDuration)
	lCompactor, err := lcompactor.NewLeveledCompactorWithOptions(
		ctx,
		reg,
		logger,
		rngs,
		chunkPool,
		opts.TsdbOptions,
	)
	if err != nil {
		return nil, fmt.Errorf("create leveled compactor: %w", err)
	}

	planner, err := NewPlanner(logger, rngs, NoopNoCompactionMark{}, opts.TsdbOptions.EnableOverlappingCompaction)
	if err != nil {
		return nil, fmt.Errorf("create planner: %w", err)
	}

	return &TCompactor{
		ctx:            ctx,
		dir:            dir,
		lCompactor:     lCompactor,
		grouper:        NewDefaultGrouper(logger, reg),
		planner:        planner,
		blockPopulator: lcompactor.DefaultBlockPopulator{},
		metrics:        newMetrics(reg),
	}, nil
}

// Compact creates a new block in the compactor's directory from the blocks in the provided directories.
//
//  1. Groups the blocks by their time ranges and labels.
//  2. The compactor builds a plan for the compact itself.
//  3. The compactor compacts the blocks in the plan.
//  4. The compactor returns the ULIDs of the compacted blocks.
func (c *TCompactor) Compact(open []*block.Block) ([]ulid.ULID, error) {
	c.metrics.compactionsTriggered.Inc()
	groups, err := c.grouper.Groups(open)
	if err != nil {
		c.metrics.compactionsFailed.Inc()
		return nil, fmt.Errorf("group blocks: %w", err)
	}

	res := make([]ulid.ULID, 0, len(groups))
	for _, group := range groups {
		compIDs, err := group.Compact(c.ctx, c.dir, c.planner, c.lCompactor, c.blockPopulator, open)
		if err != nil {
			c.metrics.compactionsFailed.Inc()
			return res, fmt.Errorf("compact group: %w", err)
		}

		res = append(res, compIDs...)
	}

	return res, nil
}

// OverlappingBlocks returns all overlapping blocks from given meta files.
//
//revive:disable-next-line:cyclomatic // this is a complex algorithm
//revive:disable-next-line:cognitive-complexity // this is a complex algorithm
func (c *TCompactor) OverlappingBlocks(open []*block.Block) (block.Overlaps, error) {
	if len(open) <= 1 {
		return nil, nil
	}

	groups, err := c.grouper.Groups(open)
	if err != nil {
		return nil, fmt.Errorf("group blocks: %w", err)
	}

	overlaps := block.Overlaps{}
	for _, group := range groups {
		group.OverlappingBlocks(overlaps)
	}

	return overlaps, nil
}

//
// metrics
//

// metrics collects metrics for the compactor.
type metrics struct {
	compactionsTriggered prometheus.Counter
	compactionsFailed    prometheus.Counter
}

// newMetrics creates new [metrics] for the compactor.
func newMetrics(r prometheus.Registerer) *metrics {
	m := &metrics{
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
