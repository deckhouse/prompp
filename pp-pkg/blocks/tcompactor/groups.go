package tcompactor

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/thanos-io/thanos/pkg/block/metadata"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp-pkg/blocks/block"
	"github.com/prometheus/prometheus/pp-pkg/blocks/lcompactor"
	"github.com/prometheus/prometheus/pp-pkg/blocks/tcompactor/tblock"
	"github.com/prometheus/prometheus/tsdb"
)

// Compactor is the interface for the [lcompactor.LeveledCompactor].
type Compactor interface {
	// Compact runs compaction against the provided directories. Must
	// only be called concurrently with results of Plan().
	// Can optionally pass a list of already open blocks,
	// to avoid having to reopen them.
	// Prometheus always return one or no block. The interface allows returning more than one
	// block for downstream users to experiment with compactor.
	// When one resulting Block has 0 samples
	//  * No block is written.
	//  * The source dirs are marked Deletable.
	//  * Block is not included in the result.
	CompactWithBlockPopulatorWithWriteMetaFile(
		dest string,
		dirs []string,
		open []*block.Block,
		blockPopulator lcompactor.BlockPopulator,
		writeMetaFileFn func(logger log.Logger, dir string, meta *tsdb.BlockMeta) (int64, error),
	) ([]ulid.ULID, error)
}

//
// DefaultGrouper
//

// DefaultGrouper groups blocks by their origin labels and downsampling resolution.
type DefaultGrouper struct {
	logger                  log.Logger
	compactions             *prometheus.CounterVec
	compactionRunsStarted   *prometheus.CounterVec
	compactionRunsCompleted *prometheus.CounterVec
	compactionFailures      *prometheus.CounterVec
	verticalCompactions     *prometheus.CounterVec
}

// NewDefaultGrouper initializes a new [DefaultGrouper].
func NewDefaultGrouper(
	logger log.Logger,
	reg prometheus.Registerer,
) *DefaultGrouper {
	return &DefaultGrouper{
		logger: logger,
		compactions: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tcompact_group_compactions_total",
			Help: "Total number of group compaction attempts that resulted in a new block.",
		}, []string{"resolution"}),
		compactionRunsStarted: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tcompact_group_compaction_runs_started_total",
			Help: "Total number of group compaction attempts.",
		}, []string{"resolution"}),
		compactionRunsCompleted: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tcompact_group_compaction_runs_completed_total",
			Help: "Total number of group completed compaction runs. " +
				"This also includes compactor group runs that resulted with no compaction.",
		}, []string{"resolution"}),
		compactionFailures: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tcompact_group_compactions_failures_total",
			Help: "Total number of failed group compactions.",
		}, []string{"resolution"}),
		verticalCompactions: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_tcompact_group_vertical_compactions_total",
			Help: "Total number of group compaction attempts that resulted in a new block based on overlapping blocks.",
		}, []string{"resolution"}),
	}
}

// Groups returns the compaction groups for all blocks currently known to the syncer.
// It creates all groups from the scratch on every call.
func (g *DefaultGrouper) Groups(blocks []*block.Block) (res []*Group, err error) {
	groups := map[string]*Group{}
	for _, b := range blocks {
		meta := b.Metadata()
		groupKey := meta.Thanos.GroupKey()
		group, ok := groups[groupKey]
		if !ok {
			lbls := labels.FromMap(meta.Thanos.Labels)
			resolutionLabel := meta.Thanos.ResolutionString()
			group = NewGroup(
				log.With(g.logger, "group", fmt.Sprintf("%s@%s", resolutionLabel, lbls.String())),
				groupKey,
				lbls,
				meta.Thanos.Downsample.Resolution,
				g.compactions.WithLabelValues(resolutionLabel),
				g.compactionRunsStarted.WithLabelValues(resolutionLabel),
				g.compactionRunsCompleted.WithLabelValues(resolutionLabel),
				g.compactionFailures.WithLabelValues(resolutionLabel),
				g.verticalCompactions.WithLabelValues(resolutionLabel),
			)

			groups[groupKey] = group
			res = append(res, group)
		}

		if err := group.AppendMeta(meta); err != nil {
			return nil, fmt.Errorf("add compaction group: %w", err)
		}
	}

	sort.Slice(res, func(i, j int) bool {
		return res[i].Key() < res[j].Key()
	})

	return res, nil
}

//
// Group
//

// Group captures a set of blocks that have the same origin labels and downsampling resolution.
// Those blocks generally contain the same series and can thus efficiently be compacted.
type Group struct {
	logger                  log.Logger
	key                     string
	labels                  labels.Labels
	resolution              int64
	metasByMinTime          []*metadata.Meta
	compactions             prometheus.Counter
	compactionRunsStarted   prometheus.Counter
	compactionRunsCompleted prometheus.Counter
	compactionFailures      prometheus.Counter
	verticalCompactions     prometheus.Counter
}

// NewGroup initializes a new [Group].
func NewGroup(
	logger log.Logger,
	key string,
	lset labels.Labels,
	resolution int64,
	compactions prometheus.Counter,
	compactionRunsStarted prometheus.Counter,
	compactionRunsCompleted prometheus.Counter,
	compactionFailures prometheus.Counter,
	verticalCompactions prometheus.Counter,
) *Group {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	return &Group{
		logger:                  logger,
		key:                     key,
		labels:                  lset,
		resolution:              resolution,
		compactions:             compactions,
		compactionRunsStarted:   compactionRunsStarted,
		compactionRunsCompleted: compactionRunsCompleted,
		compactionFailures:      compactionFailures,
		verticalCompactions:     verticalCompactions,
	}
}

// AppendMeta the block with the given meta to the group.
func (cg *Group) AppendMeta(meta *metadata.Meta) error {
	if !labels.Equal(cg.labels, labels.FromMap(meta.Thanos.Labels)) {
		return errors.New("block and group labels do not match")
	}

	if cg.resolution != meta.Thanos.Downsample.Resolution {
		return errors.New("block and group resolution do not match")
	}

	cg.metasByMinTime = append(cg.metasByMinTime, meta)
	sort.Slice(cg.metasByMinTime, func(i, j int) bool {
		return cg.metasByMinTime[i].MinTime < cg.metasByMinTime[j].MinTime
	})

	return nil
}

// Key returns an identifier for the group.
func (cg *Group) Key() string {
	return cg.key
}

// Compact plans and runs a single compaction against the group.
func (cg *Group) Compact(
	ctx context.Context,
	dir string,
	planner Planner,
	comp Compactor,
	blockPopulator lcompactor.BlockPopulator,
	open []*block.Block,
) ([]ulid.ULID, error) {
	cg.compactionRunsStarted.Inc()

	compIDs, err := cg.runCompact(ctx, dir, planner, comp, blockPopulator, open)
	if err != nil {
		cg.compactionFailures.Inc()
		return compIDs, err
	}

	cg.compactionRunsCompleted.Inc()

	return compIDs, nil
}

// OverlappingBlocks returns all overlapping blocks from given meta files.
//
//revive:disable-next-line:cyclomatic // this is a complex algorithm
//revive:disable-next-line:cognitive-complexity // this is a complex algorithm
//revive:disable-next-line:function-length // this is a complex algorithm
func (cg *Group) OverlappingBlocks(overlapGroups block.Overlaps) {
	if len(cg.metasByMinTime) <= 1 {
		return
	}

	var (
		overlaps [][]tsdb.BlockMeta
		// pending contains not ended blocks in regards to "current" timestamp.
		pending = []tsdb.BlockMeta{cg.metasByMinTime[0].BlockMeta}
		// continuousPending helps to aggregate same overlaps to single group.
		continuousPending = true
	)

	// We have here blocks sorted by minTime.
	// We iterate over each block and treat its minTime as our "current" timestamp.
	// We check if any of the pending block finished (blocks that we have seen before,
	// but their maxTime was still ahead current timestamp).
	// If not, it means they overlap with our current block. In the same time current block is assumed pending.
	metas := cg.metasByMinTime[1:]
	for i := range metas {
		var newPending []tsdb.BlockMeta

		meta := metas[i].BlockMeta
		for j := range pending {
			// "meta.MinTime" is our current time.
			if meta.MinTime >= pending[j].MaxTime {
				continuousPending = false
				continue
			}

			// "p" overlaps with "b" and "p" is still pending.
			newPending = append(newPending, pending[j])
		}

		// Our block "b" is now pending.
		pending = append(newPending, meta)
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
	for _, overlap := range overlaps {
		minRange := block.TimeRange{Min: 0, Max: math.MaxInt64, Key: cg.String()}
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
}

// String returns a human readable string representation of the group.
func (cg *Group) String() string {
	return fmt.Sprintf("%d@%s", cg.resolution, cg.labels.String())
}

// runCompact plans and runs a single compaction against the group.
//
//revive:disable-next-line:function-length // running compaction is a complex task
func (cg *Group) runCompact(
	ctx context.Context,
	dir string,
	planner Planner,
	comp Compactor,
	blockPopulator lcompactor.BlockPopulator,
	open []*block.Block,
) ([]ulid.ULID, error) {
	toCompact, overlappingBlocks, err := planner.Plan(ctx, cg.metasByMinTime)
	if err != nil {
		return nil, fmt.Errorf("plan compaction: %w", err)
	}

	if len(toCompact) == 0 {
		// Nothing to do.
		return nil, nil
	}

	_ = level.Info(cg.logger).Log("msg", "compaction available and planned", "plan", fmt.Sprintf("%v", toCompact))

	toCompactDirs := make([]string, 0, len(toCompact))
	for _, m := range toCompact {
		toCompactDirs = append(toCompactDirs, filepath.Join(dir, m.ULID.String()))
	}
	sourceBlockStr := fmt.Sprintf("%v", toCompactDirs)

	begin := time.Now()
	compIDs, err := comp.CompactWithBlockPopulatorWithWriteMetaFile(
		dir,
		toCompactDirs,
		open,
		blockPopulator,
		tblock.WriteThanosMetaFileAdapter(cg.resolution, cg.labels),
	)
	if err != nil {
		return nil, fmt.Errorf("compact blocks: %w", err)
	}

	if len(compIDs) == 0 {
		// No compacted blocks means all compacted blocks are of no sample.
		// Even though no compacted blocks, there may be more work to do.
		return nil, nil
	}

	cg.compactions.Inc()
	if overlappingBlocks {
		cg.verticalCompactions.Inc()
	}

	compIDStrings := make([]string, 0, len(compIDs))
	for _, compID := range compIDs {
		compIDStrings = append(compIDStrings, compID.String())
	}

	_ = level.Info(cg.logger).Log(
		"msg", "compacted blocks",
		"new", fmt.Sprintf("%v", compIDStrings), //revive:disable-line:add-constant // to string value
		"duration", time.Since(begin),
		"overlapping_blocks", overlappingBlocks,
		"source_blocks", sourceBlockStr,
	)

	return compIDs, nil
}
