package tcompactor

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/thanos-io/thanos/pkg/block/metadata"

	"github.com/prometheus/prometheus/model/labels"
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
	bw                       BlockWorker
	compactions              *prometheus.CounterVec
	compactionRunsStarted    *prometheus.CounterVec
	compactionRunsCompleted  *prometheus.CounterVec
	compactionFailures       *prometheus.CounterVec
	verticalCompactions      *prometheus.CounterVec
	blocksMarkedForNoCompact prometheus.Counter
	hashFunc                 metadata.HashFunc
	acceptMalformedIndex     bool
	enableVerticalCompaction bool
}

// NewDefaultGrouper initializes a new [DefaultGrouper].
func NewDefaultGrouper(
	logger log.Logger,
	bw BlockWorker,
	reg prometheus.Registerer,
	blocksMarkedForNoCompact prometheus.Counter,
	hashFunc metadata.HashFunc,
	blockFilesConcurrency int,
	compactBlocksFetchConcurrency int,
	acceptMalformedIndex bool,
	enableVerticalCompaction bool,
) *DefaultGrouper {
	return &DefaultGrouper{
		bw:     bw,
		logger: logger,
		compactions: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "prometeus_compact_group_compactions_total",
			Help: "Total number of group compaction attempts that resulted in a new block.",
		}, []string{"resolution"}),
		compactionRunsStarted: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "prometeus_compact_group_compaction_runs_started_total",
			Help: "Total number of group compaction attempts.",
		}, []string{"resolution"}),
		compactionRunsCompleted: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "prometeus_compact_group_compaction_runs_completed_total",
			Help: "Total number of group completed compaction runs. " +
				"This also includes compactor group runs that resulted with no compaction.",
		}, []string{"resolution"}),
		compactionFailures: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "prometeus_compact_group_compactions_failures_total",
			Help: "Total number of failed group compactions.",
		}, []string{"resolution"}),
		verticalCompactions: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "prometeus_compact_group_vertical_compactions_total",
			Help: "Total number of group compaction attempts that resulted in a new block based on overlapping blocks.",
		}, []string{"resolution"}),
		blocksMarkedForNoCompact: blocksMarkedForNoCompact,
		hashFunc:                 hashFunc,
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
				g.bw,
				groupKey,
				lbls,
				m.Thanos.Downsample.Resolution,
				g.compactions.WithLabelValues(resolutionLabel),
				g.compactionRunsStarted.WithLabelValues(resolutionLabel),
				g.compactionRunsCompleted.WithLabelValues(resolutionLabel),
				g.compactionFailures.WithLabelValues(resolutionLabel),
				g.verticalCompactions.WithLabelValues(resolutionLabel),
				g.blocksMarkedForNoCompact,
				g.hashFunc,
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
	bw                       BlockWorker
	key                      string
	labels                   labels.Labels
	resolution               int64
	mtx                      sync.Mutex
	metasByMinTime           []*metadata.Meta
	compactions              prometheus.Counter
	compactionRunsStarted    prometheus.Counter
	compactionRunsCompleted  prometheus.Counter
	compactionFailures       prometheus.Counter
	verticalCompactions      prometheus.Counter
	blocksMarkedForNoCompact prometheus.Counter
	hashFunc                 metadata.HashFunc
	acceptMalformedIndex     bool
	enableVerticalCompaction bool
}

// NewGroup initializes a new [Group].
func NewGroup(
	logger log.Logger,
	bw BlockWorker,
	key string,
	lset labels.Labels,
	resolution int64,
	compactions prometheus.Counter,
	compactionRunsStarted prometheus.Counter,
	compactionRunsCompleted prometheus.Counter,
	compactionFailures prometheus.Counter,
	verticalCompactions prometheus.Counter,
	blocksMarkedForNoCompact prometheus.Counter,
	hashFunc metadata.HashFunc,
	acceptMalformedIndex bool,
	enableVerticalCompaction bool,
) *Group {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	return &Group{
		logger:                   logger,
		bw:                       bw,
		key:                      key,
		labels:                   lset,
		resolution:               resolution,
		compactions:              compactions,
		compactionRunsStarted:    compactionRunsStarted,
		compactionRunsCompleted:  compactionRunsCompleted,
		compactionFailures:       compactionFailures,
		verticalCompactions:      verticalCompactions,
		blocksMarkedForNoCompact: blocksMarkedForNoCompact,
		hashFunc:                 hashFunc,
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

// func (cg *Group) compact(
// 	ctx context.Context,
// 	dir string,
// 	planner Planner,
// 	comp Compactor,
// 	compactionLifecycleCallback CompactionLifecycleCallback,
// 	errChan chan error,
// ) (bool, []ulid.ULID, error) {
// 	cg.mtx.Lock()
// 	defer cg.mtx.Unlock()

// 	// Check for overlapped blocks.
// 	overlappingBlocks := false
// 	if err := cg.areBlocksOverlapping(nil); err != nil {
// 		// TODO(bwplotka): It would really nice if we could still check for other overlaps than replica. In fact this should be checked
// 		// in syncer itself. Otherwise with vertical compaction enabled we will sacrifice this important check.
// 		if !cg.enableVerticalCompaction {
// 			return false, nil, halt(errors.Wrap(err, "pre compaction overlap check"))
// 		}

// 		overlappingBlocks = true
// 	}

// 	var toCompact []*metadata.Meta
// 	if err := tracing.DoInSpanWithErr(ctx, "compaction_planning", func(ctx context.Context) (e error) {
// 		toCompact, e = planner.Plan(ctx, cg.metasByMinTime, errChan, cg.extensions)
// 		return e
// 	}); err != nil {
// 		return false, nil, errors.Wrap(err, "plan compaction")
// 	}
// 	if len(toCompact) == 0 {
// 		// Nothing to do.
// 		return false, nil, nil
// 	}

// 	level.Info(cg.logger).Log("msg", "compaction available and planned", "plan", fmt.Sprintf("%v", toCompact))

// 	// Once we have a plan we need to download the actual data.
// 	groupCompactionBegin := time.Now()
// 	begin := groupCompactionBegin

// 	if err := compactionLifecycleCallback.PreCompactionCallback(ctx, cg.logger, cg, toCompact); err != nil {
// 		return false, nil, errors.Wrapf(err, "failed to run pre compaction callback for plan: %s", fmt.Sprintf("%v", toCompact))
// 	}
// 	level.Info(cg.logger).Log("msg", "finished running pre compaction callback; downloading blocks", "duration", time.Since(begin), "duration_ms", time.Since(begin).Milliseconds(), "plan", fmt.Sprintf("%v", toCompact))

// 	begin = time.Now()
// 	g, errCtx := errgroup.WithContext(ctx)
// 	g.SetLimit(cg.compactBlocksFetchConcurrency)

// 	toCompactDirs := make([]string, 0, len(toCompact))
// 	for _, m := range toCompact {
// 		bdir := filepath.Join(dir, m.ULID.String())
// 		func(ctx context.Context, meta *metadata.Meta) {
// 			g.Go(func() error {
// 				start := time.Now()
// 				if err := tracing.DoInSpanWithErr(ctx, "compaction_block_download", func(ctx context.Context) error {
// 					return block.Download(ctx, cg.logger, cg.bkt, meta.ULID, bdir, objstore.WithFetchConcurrency(cg.blockFilesConcurrency))
// 				}, opentracing.Tags{"block.id": meta.ULID}); err != nil {
// 					return retry(errors.Wrapf(err, "download block %s", meta.ULID))
// 				}
// 				level.Debug(cg.logger).Log("msg", "downloaded block", "block", meta.ULID.String(), "duration", time.Since(start), "duration_ms", time.Since(start).Milliseconds())

// 				start = time.Now()
// 				// Ensure all input blocks are valid.
// 				var stats block.HealthStats
// 				if err := tracing.DoInSpanWithErr(ctx, "compaction_block_health_stats", func(ctx context.Context) (e error) {
// 					stats, e = block.GatherIndexHealthStats(ctx, cg.logger, filepath.Join(bdir, block.IndexFilename), meta.MinTime, meta.MaxTime)
// 					return e
// 				}, opentracing.Tags{"block.id": meta.ULID}); err != nil {
// 					return errors.Wrapf(err, "gather index issues for block %s", bdir)
// 				}

// 				if err := stats.CriticalErr(); err != nil {
// 					return halt(errors.Wrapf(err, "block with not healthy index found %s; Compaction level %v; Labels: %v", bdir, meta.Compaction.Level, meta.Thanos.Labels))
// 				}

// 				if err := stats.OutOfOrderChunksErr(); err != nil {
// 					return outOfOrderChunkError(errors.Wrapf(err, "blocks with out-of-order chunks are dropped from compaction:  %s", bdir), meta.ULID)
// 				}

// 				if err := stats.Issue347OutsideChunksErr(); err != nil {
// 					return issue347Error(errors.Wrapf(err, "invalid, but reparable block %s", bdir), meta.ULID)
// 				}

// 				if err := stats.OutOfOrderLabelsErr(); !cg.acceptMalformedIndex && err != nil {
// 					return errors.Wrapf(err,
// 						"block id %s, try running with --debug.accept-malformed-index", meta.ULID)
// 				}
// 				level.Debug(cg.logger).Log("msg", "verified block", "block", meta.ULID.String(), "duration", time.Since(start), "duration_ms", time.Since(start).Milliseconds())
// 				return nil
// 			})
// 		}(errCtx, m)

// 		toCompactDirs = append(toCompactDirs, bdir)
// 	}
// 	sourceBlockStr := fmt.Sprintf("%v", toCompactDirs)

// 	if err := g.Wait(); err != nil {
// 		return false, nil, err
// 	}

// 	level.Info(cg.logger).Log("msg", "downloaded and verified blocks; compacting blocks", "duration", time.Since(begin), "duration_ms", time.Since(begin).Milliseconds(), "plan", sourceBlockStr)

// 	begin = time.Now()
// 	var compIDs []ulid.ULID
// 	if err := tracing.DoInSpanWithErr(ctx, "compaction", func(ctx context.Context) (e error) {
// 		populateBlockFunc, e := compactionLifecycleCallback.GetBlockPopulator(ctx, cg.logger, cg)
// 		if e != nil {
// 			return e
// 		}
// 		compIDs, e = comp.CompactWithBlockPopulator(dir, toCompactDirs, nil, populateBlockFunc)
// 		return e
// 	}); err != nil {
// 		return false, nil, halt(errors.Wrapf(err, "compact blocks %v", toCompactDirs))
// 	}
// 	if len(compIDs) == 0 {
// 		// No compacted blocks means all compacted blocks are of no sample.
// 		level.Info(cg.logger).Log("msg", "no compacted blocks, deleting source blocks", "blocks", sourceBlockStr)
// 		for _, meta := range toCompact {
// 			if meta.Stats.NumSamples == 0 {
// 				if err := cg.deleteBlock(meta.ULID, filepath.Join(dir, meta.ULID.String()), blockDeletableChecker); err != nil {
// 					level.Warn(cg.logger).Log("msg", "failed to mark for deletion an empty block found during compaction", "block", meta.ULID)
// 				}
// 			}
// 		}
// 		// Even though no compacted blocks, there may be more work to do.
// 		return true, nil, nil
// 	}
// 	cg.compactions.Inc()
// 	if overlappingBlocks {
// 		cg.verticalCompactions.Inc()
// 	}
// 	compIDStrings := make([]string, 0, len(compIDs))
// 	for _, compID := range compIDs {
// 		compIDStrings = append(compIDStrings, compID.String())
// 	}
// 	compIDStrs := fmt.Sprintf("%v", compIDStrings)
// 	level.Info(cg.logger).Log("msg", "compacted blocks", "new", compIDStrs,
// 		"duration", time.Since(begin), "duration_ms", time.Since(begin).Milliseconds(), "overlapping_blocks", overlappingBlocks, "blocks", sourceBlockStr)

// 	for _, compID := range compIDs {
// 		bdir := filepath.Join(dir, compID.String())
// 		index := filepath.Join(bdir, block.IndexFilename)

// 		if err := os.Remove(filepath.Join(bdir, "tombstones")); err != nil {
// 			return false, nil, errors.Wrap(err, "remove tombstones")
// 		}

// 		newMeta, err := metadata.ReadFromDir(bdir)
// 		if err != nil {
// 			return false, nil, errors.Wrap(err, "read new meta")
// 		}

// 		var stats block.HealthStats
// 		// Ensure the output block is valid.
// 		err = tracing.DoInSpanWithErr(ctx, "compaction_verify_index", func(ctx context.Context) error {
// 			stats, err = block.GatherIndexHealthStats(ctx, cg.logger, index, newMeta.MinTime, newMeta.MaxTime)
// 			if err != nil {
// 				return err
// 			}
// 			return stats.AnyErr()
// 		})
// 		if !cg.acceptMalformedIndex && err != nil {
// 			return false, nil, halt(errors.Wrapf(err, "invalid result block %s", bdir))
// 		}

// 		thanosMeta := metadata.Thanos{
// 			Labels:       cg.labels.Map(),
// 			Downsample:   metadata.ThanosDownsample{Resolution: cg.resolution},
// 			Source:       metadata.CompactorSource,
// 			SegmentFiles: block.GetSegmentFiles(bdir),
// 			Extensions:   cg.extensions,
// 		}
// 		if stats.ChunkMaxSize > 0 {
// 			thanosMeta.IndexStats.ChunkMaxSize = stats.ChunkMaxSize
// 		}
// 		if stats.SeriesMaxSize > 0 {
// 			thanosMeta.IndexStats.SeriesMaxSize = stats.SeriesMaxSize
// 		}
// 		newMeta, err = metadata.InjectThanos(cg.logger, bdir, thanosMeta, nil)
// 		if err != nil {
// 			return false, nil, errors.Wrapf(err, "failed to finalize the block %s", bdir)
// 		}
// 		// Ensure the output block is not overlapping with anything else,
// 		// unless vertical compaction is enabled.
// 		if !cg.enableVerticalCompaction {
// 			if err := cg.areBlocksOverlapping(newMeta, toCompact...); err != nil {
// 				return false, nil, halt(errors.Wrapf(err, "resulted compacted block %s overlaps with something", bdir))
// 			}
// 		}

// 		begin = time.Now()

// 		err = tracing.DoInSpanWithErr(ctx, "compaction_block_upload", func(ctx context.Context) error {
// 			return block.Upload(ctx, cg.logger, cg.bkt, bdir, cg.hashFunc, objstore.WithUploadConcurrency(cg.blockFilesConcurrency))
// 		})
// 		if err != nil {
// 			return false, nil, retry(errors.Wrapf(err, "upload of %s failed", compID))
// 		}
// 		level.Info(cg.logger).Log("msg", "uploaded block", "result_block", compID, "duration", time.Since(begin), "duration_ms", time.Since(begin).Milliseconds())
// 		level.Info(cg.logger).Log("msg", "running post compaction callback", "result_block", compID)
// 		if err := compactionLifecycleCallback.PostCompactionCallback(ctx, cg.logger, cg, compID); err != nil {
// 			return false, nil, retry(errors.Wrapf(err, "failed to run post compaction callback for result block %s", compID))
// 		}
// 		level.Info(cg.logger).Log("msg", "finished running post compaction callback", "result_block", compID)
// 	}

// 	// Mark for deletion the blocks we just compacted from the group and bucket so they do not get included
// 	// into the next planning cycle.
// 	// Eventually the block we just uploaded should get synced into the group again (including sync-delay).
// 	for _, meta := range toCompact {
// 		if err := tracing.DoInSpanWithErr(ctx, "compaction_block_delete", func(ctx context.Context) error {
// 			return cg.deleteBlock(meta.ULID, filepath.Join(dir, meta.ULID.String()), blockDeletableChecker)
// 		}, opentracing.Tags{"block.id": meta.ULID}); err != nil {
// 			return false, nil, retry(errors.Wrapf(err, "mark old block for deletion from bucket"))
// 		}
// 		cg.groupGarbageCollectedBlocks.Inc()
// 	}

// 	level.Info(cg.logger).Log("msg", "finished compacting blocks", "duration", time.Since(groupCompactionBegin),
// 		"duration_ms", time.Since(groupCompactionBegin).Milliseconds(), "result_blocks", compIDStrs, "source_blocks", sourceBlockStr)
// 	return true, compIDs, nil
// }
