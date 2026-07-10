package expirationpolicy

import (
	"path/filepath"
	"slices"

	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp-pkg/blocks/block"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/prometheus/tsdb/fileutil"
)

//
// Metrics
//

// Metrics holds the retention constraints and counters for a BlocksToDeleteFunc
// that owns no *tsdb.DB. They mirror the corresponding tsdb dbMetrics so the
// DB-free path reports the same series.
type Metrics struct {
	timeRetentions    prometheus.Counter
	sizeRetentions    prometheus.Counter
	maxBytes          prometheus.Gauge
	retentionDuration prometheus.Gauge
}

// NewMetrics creates the retention metrics and registers them when r is not nil.
func NewMetrics(r prometheus.Registerer) *Metrics {
	factory := util.NewUnconflictRegisterer(r)
	return &Metrics{
		timeRetentions: factory.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_time_retentions_total",
			Help: "The number of times that blocks were deleted because the maximum time limit was exceeded.",
		}),
		sizeRetentions: factory.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_size_retentions_total",
			Help: "The number of times that blocks were deleted because the maximum number of bytes was exceeded.",
		}),
		maxBytes: factory.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_retention_limit_bytes",
			Help: "Max number of bytes to be retained in the tsdb blocks, configured 0 means disabled",
		}),
		retentionDuration: factory.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_retention_limit_seconds",
			Help: "How long to retain samples in storage.",
		}),
	}
}

//
// Options
//

// Options is the configuration for the expiration policy.
type Options struct {
	// RetentionDuration is the time retention in milliseconds, used for the corrupted-block outdated check.
	RetentionDuration int64
	// DownsamplingMS is the downsampling duration in milliseconds, used for the downsampling block check.
	DownsamplingMS int64
	// MaxBytes is the maximum number of bytes to be retained in the tsdb blocks, configured 0 means disabled.
	MaxBytes int64
}

//
// ExpirationPolicy
//

// ExpirationPolicy is the expiration policy for the [block.Block]s.
// It is used to determine which blocks should be deleted based on the time and size retention policies.
type ExpirationPolicy struct {
	dir     string
	c       *catalog.Catalog
	opts    *Options
	metrics *Metrics
}

// NewExpirationPolicy creates a new [ExpirationPolicy].
func NewExpirationPolicy(
	dir string,
	c *catalog.Catalog,
	opts *Options,
	r prometheus.Registerer,
) *ExpirationPolicy {
	return &ExpirationPolicy{
		dir:     dir,
		c:       c,
		opts:    opts,
		metrics: NewMetrics(r),
	}
}

// BeyondSizeRetention returns those blocks which are beyond the size retention.
//
//revive:disable-next-line:cyclomatic // complex logic is necessary for this function
func (ep *ExpirationPolicy) BeyondSizeRetention(
	rawBlocks, downsampledBlocks []*block.Block,
	deletable map[ulid.ULID]struct{},
) {
	// Size retention is disabled or no blocks to work with.
	if (len(rawBlocks) == 0 && len(downsampledBlocks) == 0) || ep.opts.MaxBytes <= 0 {
		return
	}

	var reachedLimit bool

	// Initializing size counter with the injected extra size (heads + catalog).
	blocksSize := ep.CatalogHeadsSize()
	for i, blk := range rawBlocks {
		blocksSize += blk.Size()
		if blocksSize > ep.opts.MaxBytes {
			// Add this and all following blocks for deletion.
			for _, b := range rawBlocks[i:] {
				deletable[b.Meta().ULID] = struct{}{}
			}

			reachedLimit = true

			break
		}
	}

	for i, blk := range downsampledBlocks {
		blocksSize += blk.Size()
		if blocksSize > ep.opts.MaxBytes {
			// Add this and all following blocks for deletion.
			for _, b := range downsampledBlocks[i:] {
				deletable[b.Meta().ULID] = struct{}{}
			}

			reachedLimit = true

			break
		}
	}

	if reachedLimit {
		ep.metrics.sizeRetentions.Inc()
	}
}

// BeyondTimeRetention returns those blocks which are beyond the time retention.
func (ep *ExpirationPolicy) BeyondTimeRetention(rawBlocks []*block.Block, deletable map[ulid.ULID]struct{}) bool {
	// Time retention is disabled or no blocks to work with.
	if len(rawBlocks) == 0 || ep.opts.RetentionDuration == 0 {
		return false
	}

	for i, blk := range rawBlocks {
		// The difference between the first block and this block is greater than or equal to
		// the retention period so any blocks after that are added as deletable.
		if i > 0 && rawBlocks[0].Meta().MaxTime-blk.Meta().MaxTime >= ep.opts.RetentionDuration {
			for _, b := range rawBlocks[i:] {
				deletable[b.Meta().ULID] = struct{}{}
			}

			ep.metrics.timeRetentions.Inc()
			return true
		}
	}

	return false
}

// BlocksToDelete returns the blocks that should be deleted based on the retention policy
// or already compacted into a new block.
func (ep *ExpirationPolicy) BlocksToDelete(blocks []*block.Block) map[ulid.ULID]struct{} {
	if len(blocks) == 0 {
		return nil
	}

	deletable := make(map[ulid.ULID]struct{})
	for _, blk := range blocks {
		if blk.Meta().Compaction.Deletable {
			deletable[blk.Meta().ULID] = struct{}{}
		}
	}

	// Split the blocks into downsampled and raw blocks.
	rawBlocks, downsampledBlocks := splitBlocks(blocks)

	// Beyond time retention.
	if ep.BeyondTimeRetention(rawBlocks, deletable) {
		return deletable
	}

	// Beyond size retention.
	ep.BeyondSizeRetention(rawBlocks, downsampledBlocks, deletable)

	return deletable
}

// CatalogHeadsSize returns the on-disk size of the catalog and all of its heads.
// It is useful to build the extraSize function passed to NewBlocksToDelete.
func (ep *ExpirationPolicy) CatalogHeadsSize() int64 {
	catalogSize := ep.c.OnDiskSize()
	heads := ep.c.List(nil, nil)
	for _, h := range heads {
		catalogSize += headSize(filepath.Join(ep.dir, h.Dir()))
	}

	return catalogSize
}

// headSize returns the on-disk size of a head directory.
func headSize(dir string) int64 {
	size, _ := fileutil.DirSize(dir)
	return size
}

// splitBlocks splits the blocks into downsampled and raw blocks.
func splitBlocks(blocks []*block.Block) (downsampledBlocks, rawBlocks []*block.Block) {
	// Sort the blocks by time - newest to oldest (largest to smallest timestamp).
	// This ensures that the retentions will remove the oldest  blocks.
	slices.SortFunc(blocks, func(a, b *block.Block) int {
		switch {
		case b.Meta().MaxTime < a.Meta().MaxTime:
			return -1
		case b.Meta().MaxTime > a.Meta().MaxTime:
			return 1
		default:
			return 0
		}
	})

	downsampledBlocks = make([]*block.Block, 0, len(blocks)/2)
	rawBlocks = make([]*block.Block, 0, len(blocks)/2)
	for _, blk := range blocks {
		if blk.IsDownsamplingBlock() {
			downsampledBlocks = append(downsampledBlocks, blk)
			continue
		}

		rawBlocks = append(rawBlocks, blk)
	}

	return rawBlocks, downsampledBlocks
}
