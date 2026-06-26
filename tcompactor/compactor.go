package tcompactor

import (
	"context"
	"fmt"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/thanos-io/thanos/pkg/block/metadata"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// Planner returns blocks to compact.
type Planner interface {
	// Plan returns a list of blocks that should be compacted into single one.
	// The blocks can be overlapping. The provided metadata has to be ordered by minTime.
	Plan(ctx context.Context, metasByMinTime []*metadata.Meta) ([]*metadata.Meta, bool, error)
}

//
// TCompactor
//

type TCompactor struct {
	lCompactor *tsdb.LeveledCompactor
	grouper    *DefaultGrouper
	planner    Planner
}

func NewTCompactor(
	ctx context.Context,
	reg prometheus.Registerer,
	logger log.Logger,
	ranges []int64,
	pool chunkenc.Pool,
	opts tsdb.LeveledCompactorOptions,
) (*TCompactor, error) {
	// TODO:
	// mergeFunc storage.VerticalChunkSeriesMergeFunc
	// pool downsample.NewPool()
	// compactDir      = path.Join(conf.dataDir, "compact")

	lCompactor, err := tsdb.NewLeveledCompactorWithOptions(ctx, reg, logger, ranges, pool, opts)
	if err != nil {
		return nil, fmt.Errorf("create leveled compactor: %w", err)
	}

	return &TCompactor{
		lCompactor: lCompactor,
		// bCompactor: bCompactor,
		// grouper:    grouper,
	}, nil
}

func (c *TCompactor) Plan(dir string) ([]string, error) {
	return c.lCompactor.Plan(dir)
}

func (c *TCompactor) Write(dest string, b tsdb.BlockReader, mint, maxt int64, base *tsdb.BlockMeta) ([]ulid.ULID, error) {
	return c.lCompactor.Write(dest, b, mint, maxt, base)
}

// // Compact creates a new block in the compactor's directory from the blocks in the provided directories.
// //
// //  1. The compactor builds a plan for the compact itself.
// //  2. The compactor compacts the blocks in the plan.
// //  3. The compactor returns the ULIDs of the compacted blocks.
// func (c *TCompactor) Compact(dest string, open []*tsdb.Block) ([]ulid.ULID, error) {
// 	// TODO: convert open to map[ulid.ULID]*metadata.Meta
// 	c.grouper.Groups(open)
// 	return c.lCompactor.Compact(dest, dirs, open)
// }

// Close stops the compaction loop and waits for it to finish.
func (c *TCompactor) Close() {
	// TODO: implement
}

// TODO: compactor meta write
