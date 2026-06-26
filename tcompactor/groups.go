package tcompactor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/thanos-io/thanos/pkg/block/metadata"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tcompactor/block"
	"github.com/prometheus/prometheus/tsdb"
)

// Compactor is the interface for the [tsdb.LeveledCompactor].
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
	CompactWithBlockPopulator(
		dest string,
		dirs []string,
		open []*tsdb.Block,
		blockPopulator tsdb.BlockPopulator,
	) ([]ulid.ULID, error)
}

//
// DefaultGrouper
//

// DefaultGrouper groups blocks by their origin labels and downsampling resolution.
type DefaultGrouper struct {
	logger                   log.Logger
	compactions              *prometheus.CounterVec
	compactionRunsStarted    *prometheus.CounterVec
	compactionRunsCompleted  *prometheus.CounterVec
	compactionFailures       *prometheus.CounterVec
	verticalCompactions      *prometheus.CounterVec
	acceptMalformedIndex     bool
	enableVerticalCompaction bool
}

// NewDefaultGrouper initializes a new [DefaultGrouper].
func NewDefaultGrouper(
	logger log.Logger,
	reg prometheus.Registerer,
	acceptMalformedIndex bool,
	enableVerticalCompaction bool,
) *DefaultGrouper {
	return &DefaultGrouper{
		logger: logger,
		compactions: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "prometeus_tcompact_group_compactions_total",
			Help: "Total number of group compaction attempts that resulted in a new block.",
		}, []string{"resolution"}),
		compactionRunsStarted: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "prometeus_tcompact_group_compaction_runs_started_total",
			Help: "Total number of group compaction attempts.",
		}, []string{"resolution"}),
		compactionRunsCompleted: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "prometeus_tcompact_group_compaction_runs_completed_total",
			Help: "Total number of group completed compaction runs. " +
				"This also includes compactor group runs that resulted with no compaction.",
		}, []string{"resolution"}),
		compactionFailures: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "prometeus_tcompact_group_compactions_failures_total",
			Help: "Total number of failed group compactions.",
		}, []string{"resolution"}),
		verticalCompactions: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "prometeus_tcompact_group_vertical_compactions_total",
			Help: "Total number of group compaction attempts that resulted in a new block based on overlapping blocks.",
		}, []string{"resolution"}),
		acceptMalformedIndex:     acceptMalformedIndex,
		enableVerticalCompaction: enableVerticalCompaction,
	}
}

// Groups returns the compaction groups for all blocks currently known to the syncer.
// It creates all groups from the scratch on every call.
func (g *DefaultGrouper) Groups(blocks map[ulid.ULID]*metadata.Meta) (res []*Group, err error) {
	groups := map[string]*Group{}
	for _, m := range blocks {
		groupKey := m.Thanos.GroupKey()
		group, ok := groups[groupKey]
		if !ok {
			lbls := labels.FromMap(m.Thanos.Labels)
			resolutionLabel := m.Thanos.ResolutionString()
			group = NewGroup(
				log.With(g.logger, "group", fmt.Sprintf("%s@%v", resolutionLabel, lbls.String()), "groupKey", groupKey),
				groupKey,
				lbls,
				m.Thanos.Downsample.Resolution,
				g.compactions.WithLabelValues(resolutionLabel),
				g.compactionRunsStarted.WithLabelValues(resolutionLabel),
				g.compactionRunsCompleted.WithLabelValues(resolutionLabel),
				g.compactionFailures.WithLabelValues(resolutionLabel),
				g.verticalCompactions.WithLabelValues(resolutionLabel),
				g.acceptMalformedIndex,
				g.enableVerticalCompaction,
			)

			groups[groupKey] = group
			res = append(res, group)
		}

		if err := group.AppendMeta(m); err != nil {
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
	logger                   log.Logger
	key                      string
	labels                   labels.Labels
	resolution               int64
	mtx                      sync.Mutex // TODO: not need
	metasByMinTime           []*metadata.Meta
	compactions              prometheus.Counter
	compactionRunsStarted    prometheus.Counter
	compactionRunsCompleted  prometheus.Counter
	compactionFailures       prometheus.Counter
	verticalCompactions      prometheus.Counter
	acceptMalformedIndex     bool
	enableVerticalCompaction bool
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
	acceptMalformedIndex bool,
	enableVerticalCompaction bool,
) *Group {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	return &Group{
		logger:                   logger,
		key:                      key,
		labels:                   lset,
		resolution:               resolution,
		compactions:              compactions,
		compactionRunsStarted:    compactionRunsStarted,
		compactionRunsCompleted:  compactionRunsCompleted,
		compactionFailures:       compactionFailures,
		verticalCompactions:      verticalCompactions,
		acceptMalformedIndex:     acceptMalformedIndex,
		enableVerticalCompaction: enableVerticalCompaction,
	}
}

// AppendMeta the block with the given meta to the group.
func (cg *Group) AppendMeta(meta *metadata.Meta) error {
	cg.mtx.Lock()
	defer cg.mtx.Unlock()

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
	blockPopulator tsdb.BlockPopulator,
) ([]ulid.ULID, error) {
	cg.compactionRunsStarted.Inc()

	compIDs, err := cg.runCompact(ctx, dir, planner, comp, blockPopulator)
	if err != nil {
		cg.compactionFailures.Inc()
		return compIDs, err
	}

	cg.compactionRunsCompleted.Inc()

	return compIDs, nil
}

// runCompact plans and runs a single compaction against the group.
func (cg *Group) runCompact(
	ctx context.Context,
	dir string,
	planner Planner,
	comp Compactor,
	blockPopulator tsdb.BlockPopulator,
) ([]ulid.ULID, error) {
	cg.mtx.Lock()
	defer cg.mtx.Unlock()

	toCompact, overlappingBlocks, err := planner.Plan(ctx, cg.metasByMinTime)
	if err != nil {
		return nil, fmt.Errorf("plan compaction: %w", err)
	}

	if len(toCompact) == 0 {
		// Nothing to do.
		return nil, nil
	}

	level.Info(cg.logger).Log("msg", "compaction available and planned", "plan", fmt.Sprintf("%v", toCompact))

	// Once we have a plan we need to download the actual data.
	groupCompactionBegin := time.Now()

	toCompactDirs := make([]string, 0, len(toCompact))
	for _, m := range toCompact {
		toCompactDirs = append(toCompactDirs, filepath.Join(dir, m.ULID.String()))
	}
	sourceBlockStr := fmt.Sprintf("%v", toCompactDirs)

	begin := time.Now()
	compIDs, err := comp.CompactWithBlockPopulator(dir, toCompactDirs, nil, blockPopulator)
	if err != nil {
		return nil, fmt.Errorf("compact blocks: %w", err)
	}

	// TODO: handle empty compIDs
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
	compIDStrs := fmt.Sprintf("%v", compIDStrings)
	level.Info(cg.logger).Log(
		"msg", "compacted blocks",
		"new", compIDStrs,
		"duration", time.Since(begin),
		"overlapping_blocks", overlappingBlocks,
		"blocks", sourceBlockStr,
	)

	for _, compID := range compIDs {
		bdir := filepath.Join(dir, compID.String())
		index := filepath.Join(bdir, block.IndexFilename)

		if err := os.Remove(filepath.Join(bdir, "tombstones")); err != nil {
			return nil, fmt.Errorf("remove tombstones: %w", err)
		}

		newMeta, err := metadata.ReadFromDir(bdir)
		if err != nil {
			return nil, fmt.Errorf("read new meta: %w", err)
		}

		// Ensure the output block is valid.
		stats, err := block.GatherIndexHealthStats(ctx, cg.logger, index, newMeta.MinTime, newMeta.MaxTime)
		if !cg.acceptMalformedIndex && errors.Join(err, stats.AnyErr()) != nil {
			return nil, fmt.Errorf("invalid result block %s: %w", bdir, errors.Join(err, stats.AnyErr()))
		}

		if _, err = metadata.InjectThanos(
			cg.logger,
			bdir,
			metadata.Thanos{
				Labels:       cg.labels.Map(),
				Downsample:   metadata.ThanosDownsample{Resolution: cg.resolution},
				Source:       metadata.CompactorSource,
				SegmentFiles: block.GetSegmentFiles(bdir),
				IndexStats:   metadata.IndexStats{ChunkMaxSize: stats.ChunkMaxSize, SeriesMaxSize: stats.SeriesMaxSize},
			},
			nil,
		); err != nil {
			return nil, fmt.Errorf("failed to finalize the block %s: %w", bdir, err)
		}
		// TODO: log info
	}

	level.Info(cg.logger).Log(
		"msg", "finished compacting blocks",
		"duration", time.Since(groupCompactionBegin),
		"result_blocks", compIDStrs,
		"source_blocks", sourceBlockStr,
	)

	return compIDs, nil
}
