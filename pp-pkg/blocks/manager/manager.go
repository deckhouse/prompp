package manager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp-pkg/blocks/block"
	"github.com/prometheus/prometheus/pp-pkg/blocks/upsampler"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/fileutil"
)

var (
	_ storage.Queryable      = (*Manager)(nil)
	_ storage.ChunkQueryable = (*Manager)(nil)
)

const (
	tmpForDeletionBlockDirSuffix = ".tmp-for-deletion"
	reloadBlocksInterval         = time.Minute
	blockDurationMinuteMS        = int64(time.Minute / time.Millisecond)
)

//
// BlocksToDeleteFunc
//

// BlocksToDeleteFunc is a function that returns a map of block IDs to delete.
// It is used to determine which blocks to delete based on the block manager's retention policy.
type BlocksToDeleteFunc func(blocks []*block.Block) map[ulid.ULID]struct{}

// Options configures block reload, mirroring the relevant tsdb.Options fields.
type Options struct {
	// RetentionDuration is the time retention in milliseconds, used for the corrupted-block outdated check.
	RetentionDuration int64
	// DownsamplingMS is the downsampling duration in milliseconds, used for the downsampling block check.
	DownsamplingMS int64
	// CorruptedRetentionDuration is the duration of the retention for corrupted blocks.
	CorruptedRetentionDuration time.Duration
	// EnableOverlappingCompaction enables warning about overlapping blocks on reload.
	EnableOverlappingCompaction bool
}

// needDownsampling checks if the delta is greater than the downsampling duration.
func (o *Options) needDownsampling(delta int64) bool {
	return o.DownsamplingMS > 0 && delta > o.RetentionDuration
}

// Manager reloads and applies retention to persisted blocks on disk.
type Manager struct {
	dir            string
	opts           *Options
	blocksToDelete BlocksToDeleteFunc
	logger         log.Logger
	chunkPool      chunkenc.Pool
	metrics        *metrics
	lsObserver     LocalStorageObserver

	mtx       sync.RWMutex
	blocks    []*block.Block
	compactor compactionRunner

	stopc          chan struct{}
	stoppedc       chan struct{}
	stopOnce       sync.Once
	deleteBlocksWG sync.WaitGroup
}

// compactionRunner runs a single compaction pass over the on-disk blocks,
// reporting whether a compaction was performed and the ULIDs of the blocks it
// created (so the driver can remove them if the following reload fails).
type compactionRunner interface {
	// Compact creates a new block in the compactor's directory from the blocks in the provided directories.
	Compact(open []*block.Block) ([]ulid.ULID, error)

	// OverlappingBlocks returns all overlapping blocks from given blocks.
	OverlappingBlocks(blocks []*block.Block) (block.Overlaps, error)
}

// LocalStorageObserver is the observer of the local storage.
type LocalStorageObserver interface {
	// Observe is the function to observe the local storage.
	Observe(ctx context.Context)
}

// NewManager init new [Manager] and starts its periodic reload loop.
//
// blocksToDelete is the retention filter (e.g. built via pp-pkg/tsdb.NewBlocksToDelete);
// it may be nil, in which case no blocks are deleted by retention.
func NewManager(
	dir string,
	opts *Options,
	compactor compactionRunner,
	blocksToDelete BlocksToDeleteFunc,
	chunkPool chunkenc.Pool,
	lsObserver LocalStorageObserver,
	logger log.Logger,
	r prometheus.Registerer,
) (*Manager, error) {
	if opts == nil {
		opts = &Options{}
	}
	if logger == nil {
		logger = log.NewNopLogger()
	}

	if lsObserver == nil {
		lsObserver = noopLocalStorageObserver{}
	}

	m := &Manager{
		dir:            dir,
		opts:           opts,
		compactor:      compactor,
		blocksToDelete: blocksToDelete,
		logger:         logger,
		chunkPool:      chunkPool,
		lsObserver:     lsObserver,
		stopc:          make(chan struct{}),
		stoppedc:       make(chan struct{}),
	}
	m.metrics = newMetrics(m, r)

	// Best-effort cleanup of leftover tmp block dirs (e.g. *.tmp-for-creation) that
	// may remain after a crash during compaction or persist. Unlike tsdb.DB.Open, the
	// block Manager never loads these dirs, so without this they would leak on disk.
	if err := tsdb.RemoveBestEffortTmpDirs(logger, dir); err != nil {
		_ = level.Warn(logger).Log("msg", "failed to remove leftover tmp block dirs", "dir", dir, "err", err)
	}

	if err := m.reloadBlocks(); err != nil {
		return nil, fmt.Errorf("initial reload blocks: %w", err)
	}
	m.logLoadedBlocks()

	_ = level.Info(logger).Log("msg", "Block manager started", "dir", dir)
	go m.loop()
	return m, nil
}

func (m *Manager) loop() {
	defer func() {
		close(m.stoppedc)
	}()

	ticker := time.NewTicker(reloadBlocksInterval)
	defer ticker.Stop()
	baseCtx := context.Background()

	for {
		select {
		case <-ticker.C:
			m.reloadAndCompact()

			ctx, cancel := context.WithTimeout(baseCtx, reloadBlocksInterval/2)
			m.lsObserver.Observe(ctx)
			cancel()

		case <-m.stopc:
			return
		}
	}
}

// reloadAndCompact reloads blocks and then compacts repeatedly until there is
// nothing left to compact, reloading between passes. Everything runs in this one
// goroutine, so a compaction never races with the deletion of its inputs: after
// each compaction the reload loads the new block and deletes the now-obsolete
// parents before the next plan is computed (mirroring tsdb's single-goroutine
// compact/reload loop). Compacting until exhaustion within a single tick avoids
// waiting a full ticker interval per compaction step.
func (m *Manager) reloadAndCompact() {
	if err := m.reloadBlocks(); err != nil {
		//revive:disable-next-line:add-constant // this is log
		_ = level.Error(m.logger).Log("msg", "periodic reload blocks failed", "err", err)
	}

	for {
		compacted, errCompact := m.compactor.Compact(m.blocks)
		if errCompact != nil {
			//revive:disable-next-line:add-constant // this is log
			_ = level.Error(m.logger).Log("msg", "compaction failed", "err", errCompact)
		}
		if len(compacted) == 0 {
			return
		}
		// Reload to load the freshly created block and delete the obsolete
		// parents before planning the next compaction. If the reload fails,
		// remove the freshly compacted block(s) so a half-applied compaction
		// does not leave orphaned blocks on disk (mirroring tsdb).
		if err := m.reloadBlocks(); err != nil {
			_ = level.Error(m.logger).Log("msg", "reload blocks after compaction failed", "err", err)
			m.deleteCompactedBlocks(compacted)
			return
		}

		if errCompact != nil {
			return
		}
	}
}

// Close stops the reload loop and waits for it to finish.
func (m *Manager) Close() {
	m.stopOnce.Do(func() {
		close(m.stopc)
	})
	<-m.stoppedc
	m.deleteBlocksWG.Wait()

	_ = level.Info(m.logger).Log("msg", "Block manager closed")

	m.mtx.Lock()
	defer m.mtx.Unlock()
	for _, b := range m.blocks {
		if err := b.Close(); err != nil {
			_ = level.Warn(m.logger).Log("msg", "Closing block failed", "err", err, "block", b.Meta().ULID)
		}
	}

	m.blocks = nil
}

// Querier returns a new querier over the persisted blocks overlapping the time
// range [mint, maxt]. It implements [storage.Queryable].
//
//revive:disable-next-line:cyclomatic // complex logic is necessary for this function
func (m *Manager) Querier(mint, maxt int64) (_ storage.Querier, err error) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()

	blockQueriers := make([]storage.Querier, 0, len(m.blocks))
	defer func() {
		if err != nil {
			// If we fail, all previously opened queriers must be closed.
			for _, q := range blockQueriers {
				_ = q.Close()
			}
		}
	}()

	needDownsampling := m.opts.needDownsampling(maxt - mint)
	resolutionMS := m.opts.DownsamplingMS
	if needDownsampling {
		// If we need downsampling, we need to reduce the mint for the resolution query for each block for upsampler.
		mint -= resolutionMS
	}
	for _, b := range m.blocks {
		if m.skipBlock(b, mint, maxt, needDownsampling) {
			continue
		}

		q, err := tsdb.NewBlockQuerier(b, mint, maxt)
		if err != nil {
			return nil, fmt.Errorf("open querier for block %s: %w", b, err)
		}

		resolutionMS = max(resolutionMS, b.Metadata().Thanos.Downsample.Resolution)
		blockQueriers = append(blockQueriers, q)
	}

	q := storage.NewMergeQuerier(blockQueriers, nil, storage.ChainedSeriesMerge)
	if needDownsampling {
		return upsampler.NewResolutionQuerier(q, resolutionMS), nil
	}

	return q, nil
}

// ChunkQuerier returns a new chunk querier over the persisted blocks overlapping
// the time range [mint, maxt]. It implements [storage.ChunkQueryable].
func (m *Manager) ChunkQuerier(mint, maxt int64) (_ storage.ChunkQuerier, err error) {
	m.mtx.RLock()
	defer m.mtx.RUnlock()

	blockQueriers := make([]storage.ChunkQuerier, 0, len(m.blocks))
	defer func() {
		if err != nil {
			// If we fail, all previously opened queriers must be closed.
			for _, q := range blockQueriers {
				_ = q.Close()
			}
		}
	}()

	needDownsampling := m.opts.needDownsampling(maxt - mint)
	for _, b := range m.blocks {
		if m.skipBlock(b, mint, maxt, needDownsampling) {
			continue
		}

		q, err := tsdb.NewBlockChunkQuerier(b, mint, maxt)
		if err != nil {
			return nil, fmt.Errorf("open chunk querier for block %s: %w", b, err)
		}

		blockQueriers = append(blockQueriers, q)
	}

	return storage.NewMergeChunkQuerier(
		blockQueriers,
		nil,
		storage.NewCompactingChunkSeriesMerger(storage.ChainedSeriesMerge),
	), nil
}

// Blocks returns a snapshot of the currently loaded blocks. It implements [BlockSource].
func (m *Manager) Blocks() []*block.Block {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	return slices.Clone(m.blocks)
}

// logLoadedBlocks logs the set of currently loaded blocks, mirroring the
// "Found healthy block" output of legacy tsdb so operators can see the on-disk
// block layout at startup.
func (m *Manager) logLoadedBlocks() {
	m.mtx.RLock()
	defer m.mtx.RUnlock()

	for _, b := range m.blocks {
		meta := b.Metadata()
		_ = level.Info(m.logger).Log(
			"msg", "Found healthy block",
			"mint", meta.MinTime,
			"maxt", meta.MaxTime,
			"ulid", meta.ULID,
			"duration_minutes", normalizeBlockDurationMinutes(meta.MaxTime-meta.MinTime),
			"resolution", meta.Thanos.Downsample.Resolution,
		)
	}
}

// reloadBlocks reloads blocks from disk and deletes the ones past retention.
//
//revive:disable-next-line:cyclomatic // ported from tsdb.DB.reloadBlocks.
//revive:disable-next-line:cognitive-complexity // ported from tsdb.DB.reloadBlocks.
//revive:disable-next-line:function-length // ported from tsdb.DB.reloadBlocks.
func (m *Manager) reloadBlocks() (err error) {
	defer func() {
		if err != nil {
			m.metrics.reloadsFailed.Inc()
		}
		m.metrics.reloads.Inc()
	}()

	m.mtx.Lock()
	defer m.mtx.Unlock()

	loadable, corrupted, err := block.OpenBlocks(m.logger, m.dir, m.blocks, m.chunkPool)
	if err != nil {
		return err
	}

	var deletableULIDs map[ulid.ULID]struct{}
	if m.blocksToDelete != nil {
		deletableULIDs = m.blocksToDelete(loadable)
	}
	deletable := make(map[ulid.ULID]*block.Block, len(deletableULIDs))

	// Mark all parents of loaded blocks as deletable (no matter if they exists).
	// This makes it resilient against the process crashing towards the end of a compaction,
	// but before deletions. By doing that, we can pick up the deletion where it left off during a crash.
	for _, blk := range loadable {
		if _, ok := deletableULIDs[blk.Meta().ULID]; ok {
			deletable[blk.Meta().ULID] = blk
		}

		for _, b := range blk.Meta().Compaction.Parents {
			if _, ok := corrupted[b.ULID]; ok {
				delete(corrupted, b.ULID)
				_ = level.Warn(m.logger).Log(
					//revive:disable-next-line:line-length-limit // this is log
					"msg", "Found corrupted block, but replaced by compacted one so it's safe to delete. This should not happen with atomic deletes.",
					"block", b.ULID,
				)
			}

			deletable[b.ULID] = nil
		}
	}

	m.metrics.corruptedBlocks.Set(float64(len(corrupted)))
	for uid, cerr := range corrupted {
		// check if the block is outdated, if it is, delete the block.
		isOutdated := m.isOutdatedBlock(
			uid,
			min(
				time.Duration(m.opts.RetentionDuration)*time.Millisecond,
				m.opts.CorruptedRetentionDuration,
			),
		)

		if isOutdated {
			deletable[uid] = nil
		}

		_ = level.Warn(m.logger).Log(
			"msg", "corrupted block",
			"ulid", uid.String(),
			"err", cerr,
			"isOutdated", isOutdated,
		)
	}

	var (
		toLoad               = make([]*block.Block, 0, len(loadable))
		blocksSize           int64
		blocksByDurationMins = map[int64]int{}
	)
	// All deletable blocks should be unloaded.
	// NOTE: We need to loop through loadable one more time
	// as there might be loadable ready to be removed (replaced by compacted block).
	for _, blk := range loadable {
		if _, ok := deletable[blk.Meta().ULID]; ok {
			deletable[blk.Meta().ULID] = blk
			continue
		}

		toLoad = append(toLoad, blk)
		blocksSize += blk.Size()
		durationMinutes := normalizeBlockDurationMinutes(blk.Meta().MaxTime - blk.Meta().MinTime)
		blocksByDurationMins[durationMinutes]++
	}
	m.metrics.blocksBytes.Set(float64(blocksSize))
	m.metrics.loadedBlocksByDuration.Reset()
	for durationMinutes, count := range blocksByDurationMins {
		m.metrics.loadedBlocksByDuration.WithLabelValues(strconv.FormatInt(durationMinutes, 10)).Set(float64(count))
	}

	slices.SortFunc(toLoad, func(a, b *block.Block) int {
		switch {
		case a.Meta().MinTime < b.Meta().MinTime:
			return -1
		case a.Meta().MinTime > b.Meta().MinTime:
			return 1
		default:
			return 0
		}
	})

	// Swap new blocks first for subsequently created readers to be seen.
	oldBlocks := m.blocks
	m.blocks = toLoad

	// Only check overlapping blocks when overlapping compaction is enabled.
	if m.opts.EnableOverlappingCompaction {
		overlaps, errOverlaps := m.compactor.OverlappingBlocks(toLoad)
		if errOverlaps != nil {
			_ = level.Error(m.logger).Log("msg", "get overlapping blocks failed", "err", errOverlaps)
		} else if len(overlaps) > 0 {
			_ = level.Warn(m.logger).Log(
				"msg", "Overlapping blocks found during reloadBlocks",
				"detail", overlaps.String(),
			)
		}
	}

	// Append blocks to old, deletable blocks, so we can close them.
	for _, b := range oldBlocks {
		if _, ok := deletable[b.Meta().ULID]; ok {
			deletable[b.Meta().ULID] = b
		}
	}

	m.renameForDeletionBlocks(deletable)
	if len(deletable) > 0 {
		m.deleteBlocksWG.Add(1)
		go m.closeAndDeleteBlocks(deletable)
	}

	return nil
}

// renameForDeletionBlocks renames the given blocks for deletion.
func (m *Manager) renameForDeletionBlocks(blocks map[ulid.ULID]*block.Block) {
	for uid := range blocks {
		from := filepath.Join(m.dir, uid.String())
		switch _, err := os.Stat(from); {
		case os.IsNotExist(err):
			// Noop.
			continue
		case err != nil:
			_ = level.Warn(m.logger).Log(
				"msg", "get stat block failed",
				"from", from,
				"err", err,
			)
			continue
		}

		// Replace atomically to avoid partial block when process would crash during deletion.
		tmpToDelete := filepath.Join(m.dir, fmt.Sprintf("%s%s", uid, tmpForDeletionBlockDirSuffix))
		if err := fileutil.Replace(from, tmpToDelete); err != nil {
			_ = level.Warn(m.logger).Log(
				"msg", "replace of obsolete block for deletion failed",
				"block_id", uid,
				"err", err,
			)
		}
	}
}

// closeAndDeleteBlocks closes the given blocks and deletes the temporary files for deletion.
func (m *Manager) closeAndDeleteBlocks(blocks map[ulid.ULID]*block.Block) {
	defer m.deleteBlocksWG.Done()

	for uid, blk := range blocks {
		if blk != nil {
			if err := blk.Close(); err != nil {
				//revive:disable-next-line:add-constant // this is log
				_ = level.Warn(m.logger).Log("msg", "Closing block failed", "err", err, "block", uid)
			}
		}

		tmpToDelete := filepath.Join(m.dir, fmt.Sprintf("%s%s", uid, tmpForDeletionBlockDirSuffix))
		switch _, err := os.Stat(tmpToDelete); {
		case os.IsNotExist(err):
			// Noop.
			continue
		case err != nil:
			_ = level.Warn(m.logger).Log("msg", "stat temporary file for deletion failed", "err", err, "block", uid)
			continue
		}

		if err := os.RemoveAll(tmpToDelete); err != nil {
			_ = level.Warn(m.logger).Log("msg", "delete temporary file for deletion failed", "err", err, "block", uid)
			continue
		}

		_ = level.Info(m.logger).Log("msg", "Deleting obsolete block", "block", uid)
	}
}

// deleteCompactedBlocks removes the given block directories from disk. It is used
// to clean up freshly compacted blocks when the reload that would have loaded
// them fails, so a half-applied compaction does not leave orphaned blocks behind
// (mirroring tsdb).
func (m *Manager) deleteCompactedBlocks(uids []ulid.ULID) {
	for _, uid := range uids {
		if err := os.RemoveAll(filepath.Join(m.dir, uid.String())); err != nil {
			_ = level.Error(m.logger).Log("msg", "delete compacted block after failed reload", "block", uid, "err", err)
		}
	}
}

// isOutdatedBlock checks if a block is outdated based on its ULID and retention duration.
func (*Manager) isOutdatedBlock(id ulid.ULID, retentionDuration time.Duration) bool {
	return id.Time() < uint64(time.Now().Add(-retentionDuration).UnixMilli()) // #nosec G115 // no overflow
}

func (*Manager) skipBlock(b *block.Block, mint, maxt int64, needDownsampling bool) bool {
	return !b.OverlapsClosedInterval(mint, maxt) ||
		(needDownsampling && !b.IsDownsamplingBlock()) ||
		(!needDownsampling && b.IsDownsamplingBlock())
}

// normalizeBlockDurationMinutes normalizes a block duration in milliseconds to minutes.
func normalizeBlockDurationMinutes(durationMS int64) int64 {
	if durationMS <= 0 {
		return 0
	}

	//revive:disable-next-line:add-constant // half minute rounding
	return (durationMS + blockDurationMinuteMS/2) / blockDurationMinuteMS
}

//
// metrics
//

// metrics collects metrics for the block manager.
type metrics struct {
	loadedBlocks           prometheus.GaugeFunc
	loadedBlocksByDuration *prometheus.GaugeVec
	symbolTableSize        prometheus.GaugeFunc
	reloads                prometheus.Counter
	reloadsFailed          prometheus.Counter
	corruptedBlocks        prometheus.Gauge
	blocksBytes            prometheus.Gauge
}

// newMetrics creates new [metrics] for the block manager.
//
//revive:disable-next-line:function-length // constructor metrics
func newMetrics(manager *Manager, r prometheus.Registerer) *metrics {
	m := &metrics{
		loadedBlocks: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_blocks_loaded",
			Help: "Number of currently loaded data blocks.",
		}, func() float64 {
			manager.mtx.RLock()
			defer manager.mtx.RUnlock()
			return float64(len(manager.blocks))
		}),
		loadedBlocksByDuration: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_blocks_loaded_by_duration",
			Help: "Number of currently loaded blocks grouped by block duration in minutes.",
		}, []string{"duration_minutes"}),
		symbolTableSize: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_symbol_table_size_bytes",
			Help: "Size of symbol table in memory for loaded blocks.",
		}, func() float64 {
			manager.mtx.RLock()
			defer manager.mtx.RUnlock()
			var symTblSize uint64
			for _, b := range manager.blocks {
				symTblSize += b.GetSymbolTableSize()
			}
			return float64(symTblSize)
		}),
		reloads: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_reloads_total",
			Help: "Number of times the database reloaded block data from disk.",
		}),
		reloadsFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_reloads_failures_total",
			Help: "Number of times the database failed to reloadBlocks block data from disk.",
		}),
		corruptedBlocks: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_corrupted_blocks",
			Help: "The number of corrupted blocks.",
		}),
		blocksBytes: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_storage_blocks_bytes",
			Help: "The number of bytes that are currently used for local storage by all blocks.",
		}),
	}

	if r != nil {
		r.MustRegister(
			m.loadedBlocks,
			m.loadedBlocksByDuration,
			m.symbolTableSize,
			m.reloads,
			m.reloadsFailed,
			m.corruptedBlocks,
			m.blocksBytes,
		)
	}

	return m
}

//
// noopLocalStorageObserver
//

// noopLocalStorageObserver is the noop implementation of the [LocalStorageObserver].
type noopLocalStorageObserver struct{}

// Observe is the function to observe the local storage.
func (noopLocalStorageObserver) Observe(context.Context) {}
