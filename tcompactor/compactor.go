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
// Options
//

// Options are the options for the [TCompactor].
type Options struct {
	// TsdbOptions are the options for the [tsdb.LeveledCompactor].
	TsdbOptions tsdb.LeveledCompactorOptions

	// MinBlockDuration is the smallest block range, used to derive the
	// exponential compaction ranges. If zero, tsdb.DefaultBlockDuration is used.
	MinBlockDuration int64

	// MaxBlockDuration limits the largest compaction range. If zero, no limit is
	// applied and all exponential ranges are used.
	MaxBlockDuration int64

	// AcceptMalformedIndex allows the compactor to accept blocks with malformed index.
	AcceptMalformedIndex bool
}

//
// TCompactor
//

// TCompactor is the compactor copied from the Thanos compactor.
type TCompactor struct {
	ctx            context.Context
	dir            string
	lCompactor     *tsdb.LeveledCompactor
	grouper        *DefaultGrouper
	planner        Planner
	blockPopulator tsdb.BlockPopulator
}

// NewTCompactor creates a new [TCompactor].
func NewTCompactor(
	ctx context.Context,
	logger log.Logger,
	dir string,
	opts Options,
	reg prometheus.Registerer,
) (*TCompactor, error) {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	minBlockDuration := opts.MinBlockDuration
	if minBlockDuration <= 0 {
		minBlockDuration = tsdb.DefaultBlockDuration
	}

	rngs := compactionRanges(minBlockDuration, opts.MaxBlockDuration)
	lCompactor, err := tsdb.NewLeveledCompactorWithOptions(
		ctx,
		reg,
		logger,
		rngs,
		chunkenc.NewPool(),
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
		grouper:        NewDefaultGrouper(logger, reg, opts.AcceptMalformedIndex),
		planner:        planner,
		blockPopulator: tsdb.DefaultBlockPopulator{},
	}, nil
}

// Compact creates a new block in the compactor's directory from the blocks in the provided directories.
//
//  1. The compactor builds a plan for the compact itself.
//  2. The compactor compacts the blocks in the plan.
//  3. The compactor returns the ULIDs of the compacted blocks.
func (c *TCompactor) Compact(open []*tsdb.Block) ([]ulid.ULID, error) {
	metas := make(map[ulid.ULID]*metadata.Meta, len(open))
	for _, b := range open {
		meta := b.Meta()
		metas[meta.ULID] = &metadata.Meta{
			BlockMeta: meta,
			Thanos:    metadata.Thanos{Labels: make(map[string]string)},
		}
	}

	groups, err := c.grouper.Groups(metas)
	if err != nil {
		return nil, fmt.Errorf("group blocks: %w", err)
	}

	res := make([]ulid.ULID, 0, len(groups))
	for _, group := range groups {
		compIDs, err := group.Compact(c.ctx, c.dir, c.planner, c.lCompactor, c.blockPopulator, open)
		if err != nil {
			return res, fmt.Errorf("compact group: %w", err)
		}

		res = append(res, compIDs...)
	}

	return res, nil
}

// Close stops the compaction loop and waits for it to finish.
func (c *TCompactor) Close() {
	// TODO: implement
}

// compactionRanges returns the compaction ranges for the given min and max block duration.
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

// TODO: block manager metadata.Meta

// func readMetaFile(dir string) (*BlockMeta, int64, error) {
// 	b, err := os.ReadFile(filepath.Join(dir, metaFilename))
// 	if err != nil {
// 		return nil, 0, err
// 	}
// 	var m BlockMeta

// 	if err := json.Unmarshal(b, &m); err != nil {
// 		return nil, 0, err
// 	}
// 	if m.Version != metaVersion1 {
// 		return nil, 0, fmt.Errorf("unexpected meta file version %d", m.Version)
// 	}

// 	return &m, int64(len(b)), nil
// }

// // Read the block meta from the given reader.
// func Read(rc io.ReadCloser) (_ *Meta, err error) {
// 	defer runutil.ExhaustCloseWithErrCapture(&err, rc, "close meta JSON")

// 	var m Meta
// 	if err = json.NewDecoder(rc).Decode(&m); err != nil {
// 		return nil, err
// 	}

// 	if m.Version != TSDBVersion1 {
// 		return nil, errors.Errorf("unexpected meta file version %d", m.Version)
// 	}

// 	version := m.Thanos.Version
// 	if version == 0 {
// 		// For compatibility.
// 		version = ThanosVersion1
// 	}

// 	if version != ThanosVersion1 {
// 		return nil, errors.Errorf("unexpected meta file Thanos section version %d", m.Version)
// 	}

// 	if m.Thanos.Labels == nil {
// 		// To avoid extra nil checks, allocate map here if empty.
// 		m.Thanos.Labels = make(map[string]string)
// 	}
// 	return &m, nil
// }
