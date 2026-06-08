package block

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/fileutil"
)

var (
	_ storage.Queryable      = (*Manager)(nil)
	_ storage.ChunkQueryable = (*Manager)(nil)
	_ BlockSource            = (*Manager)(nil)
)

const (
	tmpForDeletionBlockDirSuffix = ".tmp-for-deletion"
	reloadBlocksInterval         = time.Minute
)

// Options configures block reload, mirroring the relevant tsdb.Options fields.
type Options struct {
	// RetentionDuration is the time retention in milliseconds, used for the corrupted-block outdated check.
	RetentionDuration int64
	// CorruptedRetentionDuration is the duration of the retention for corrupted blocks.
	CorruptedRetentionDuration time.Duration
	// EnableOverlappingCompaction enables warning about overlapping blocks on reload.
	EnableOverlappingCompaction bool
}

// Manager reloads and applies retention to persisted blocks on disk.
type Manager struct {
	dir            string
	opts           *Options
	blocksToDelete tsdb.BlocksToDeleteFunc
	logger         log.Logger
	chunkPool      chunkenc.Pool
	metrics        *metrics

	mtx    sync.RWMutex
	blocks []*tsdb.Block

	stopc    chan struct{}
	stoppedc chan struct{}
	stopOnce sync.Once
}

// NewManager init new [Manager] and starts its periodic reload loop.
//
// blocksToDelete is the retention filter (e.g. built via pp-pkg/tsdb.NewBlocksToDelete);
// it may be nil, in which case no blocks are deleted by retention.
func NewManager(
	dir string,
	opts *Options,
	blocksToDelete tsdb.BlocksToDeleteFunc,
	logger log.Logger,
	r prometheus.Registerer,
) *Manager {
	if opts == nil {
		opts = &Options{}
	}
	if logger == nil {
		logger = log.NewNopLogger()
	}

	m := &Manager{
		dir:            dir,
		opts:           opts,
		blocksToDelete: blocksToDelete,
		logger:         logger,
		chunkPool:      chunkenc.NewPool(),
		metrics:        newMetrics(r),
		stopc:          make(chan struct{}),
		stoppedc:       make(chan struct{}),
	}
	go m.loop()
	return m
}

func (m *Manager) loop() {
	defer func() {
		close(m.stoppedc)
	}()

	ticker := time.NewTicker(reloadBlocksInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := m.reloadBlocks(); err != nil {
				level.Error(m.logger).Log("msg", "periodic reload blocks failed", "err", err)
			}

		case <-m.stopc:
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
}

// Querier returns a new querier over the persisted blocks overlapping the time
// range [mint, maxt]. It implements [storage.Queryable].
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

	for _, b := range m.blocks {
		if !b.OverlapsClosedInterval(mint, maxt) {
			continue
		}
		q, err := tsdb.NewBlockQuerier(b, mint, maxt)
		if err != nil {
			return nil, fmt.Errorf("open querier for block %s: %w", b, err)
		}
		blockQueriers = append(blockQueriers, q)
	}

	return storage.NewMergeQuerier(blockQueriers, nil, storage.ChainedSeriesMerge), nil
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

	for _, b := range m.blocks {
		if !b.OverlapsClosedInterval(mint, maxt) {
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

// Blocks returns a snapshot of the currently loaded blocks. It implements
// [BlockSource].
func (m *Manager) Blocks() []*tsdb.Block {
	m.mtx.RLock()
	defer m.mtx.RUnlock()
	return slices.Clone(m.blocks)
}

// reloadBlocks reloads blocks from disk and deletes the ones past retention.
//
//revive:disable-next-line:cyclomatic // ported from tsdb.DB.reloadBlocks.
func (m *Manager) reloadBlocks() (err error) {
	defer func() {
		if err != nil {
			m.metrics.reloadsFailed.Inc()
		}
		m.metrics.reloads.Inc()
	}()

	m.mtx.Lock()
	defer m.mtx.Unlock()

	loadable, corrupted, err := tsdb.OpenBlocks(m.logger, m.dir, m.blocks, m.chunkPool)
	if err != nil {
		return err
	}

	var deletableULIDs map[ulid.ULID]struct{}
	if m.blocksToDelete != nil {
		deletableULIDs = m.blocksToDelete(loadable)
	}
	deletable := make(map[ulid.ULID]*tsdb.Block, len(deletableULIDs))

	// Mark all parents of loaded blocks as deletable (no matter if they exists). This makes it resilient against the process
	// crashing towards the end of a compaction but before deletions. By doing that, we can pick up the deletion where it left off during a crash.
	for _, block := range loadable {
		if _, ok := deletableULIDs[block.Meta().ULID]; ok {
			deletable[block.Meta().ULID] = block
		}
		for _, b := range block.Meta().Compaction.Parents {
			if _, ok := corrupted[b.ULID]; ok {
				delete(corrupted, b.ULID)
				level.Warn(m.logger).Log("msg", "Found corrupted block, but replaced by compacted one so it's safe to delete. This should not happen with atomic deletes.", "block", b.ULID)
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

		level.Warn(m.logger).Log(
			"msg", "corrupted block",
			"ulid", uid.String(),
			"err", cerr,
			"isOutdated", isOutdated,
		)
	}

	var (
		toLoad     []*tsdb.Block
		blocksSize int64
	)
	// All deletable blocks should be unloaded.
	// NOTE: We need to loop through loadable one more time as there might be loadable ready to be removed (replaced by compacted block).
	for _, block := range loadable {
		if _, ok := deletable[block.Meta().ULID]; ok {
			deletable[block.Meta().ULID] = block
			continue
		}

		toLoad = append(toLoad, block)
		blocksSize += block.Size()
	}
	m.metrics.blocksBytes.Set(float64(blocksSize))

	slices.SortFunc(toLoad, func(a, b *tsdb.Block) int {
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
		blockMetas := make([]tsdb.BlockMeta, 0, len(toLoad))
		for _, b := range toLoad {
			blockMetas = append(blockMetas, b.Meta())
		}
		if overlaps := tsdb.OverlappingBlocks(blockMetas); len(overlaps) > 0 {
			level.Warn(m.logger).Log("msg", "Overlapping blocks found during reloadBlocks", "detail", overlaps.String())
		}
	}

	// Append blocks to old, deletable blocks, so we can close them.
	for _, b := range oldBlocks {
		if _, ok := deletable[b.Meta().ULID]; ok {
			deletable[b.Meta().ULID] = b
		}
	}
	if err := m.deleteBlocks(deletable); err != nil {
		return fmt.Errorf("delete %v blocks: %w", len(deletable), err)
	}
	return nil
}

func (m *Manager) deleteBlocks(blocks map[ulid.ULID]*tsdb.Block) error {
	for uid, block := range blocks {
		if block != nil {
			if err := block.Close(); err != nil {
				level.Warn(m.logger).Log("msg", "Closing block failed", "err", err, "block", uid)
			}
		}

		toDelete := filepath.Join(m.dir, uid.String())
		switch _, err := os.Stat(toDelete); {
		case os.IsNotExist(err):
			// Noop.
			continue
		case err != nil:
			return fmt.Errorf("stat dir %v: %w", toDelete, err)
		}

		// Replace atomically to avoid partial block when process would crash during deletion.
		tmpToDelete := filepath.Join(m.dir, fmt.Sprintf("%s%s", uid, tmpForDeletionBlockDirSuffix))
		if err := fileutil.Replace(toDelete, tmpToDelete); err != nil {
			return fmt.Errorf("replace of obsolete block for deletion %s: %w", uid, err)
		}
		if err := os.RemoveAll(tmpToDelete); err != nil {
			return fmt.Errorf("delete obsolete block %s: %w", uid, err)
		}
		level.Info(m.logger).Log("msg", "Deleting obsolete block", "block", uid)
	}

	return nil
}

func (m *Manager) isOutdatedBlock(id ulid.ULID, retentionDuration time.Duration) bool {
	return id.Time() < uint64(time.Now().Add(-retentionDuration).UnixMilli())
}

//
// metrics
//

type metrics struct {
	reloads         prometheus.Counter
	reloadsFailed   prometheus.Counter
	corruptedBlocks prometheus.Gauge
	blocksBytes     prometheus.Gauge
}

func newMetrics(r prometheus.Registerer) *metrics {
	m := &metrics{
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
			m.reloads,
			m.reloadsFailed,
			m.corruptedBlocks,
			m.blocksBytes,
		)
	}

	return m
}
