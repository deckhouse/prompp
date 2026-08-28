package expirationpolicy

import (
	"path/filepath"
	"slices"
	"time"

	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/prometheus/tsdb/fileutil"
)

//
// Block
//

// Block is the interface for the block.
type Block interface {
	// Deletable returns true if the block is deletable.
	Deletable() bool
	// IsDownsamplingBlock returns true if the block is a downsampling block.
	IsDownsamplingBlock() bool
	// MaxTime returns the maximum time of the block.
	MaxTime() int64
	// Size returns the size of the block.
	Size() int64
	// ULID returns the ULID of the block.
	ULID() ulid.ULID
}

//
// Catalog
//

// Catalog is the interface for the catalog.
type Catalog interface {
	// OnDiskSize returns the on-disk size of the catalog.
	OnDiskSize() int64
	// List returns the list of heads in the catalog.
	List(
		filterFn func(record *catalog.Record) bool,
		sortLess func(lhs, rhs *catalog.Record) bool,
	) []*catalog.Record
}

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
	// RetentionDuration is the time retention in milliseconds.
	RetentionDuration int64
	// MaxBytes is the maximum number of bytes to be retained in the tsdb blocks, configured 0 means disabled.
	MaxBytes int64
	// ExtraSize reports the on-disk bytes used outside of the blocks, which are
	// charged against MaxBytes. When nil the size is taken from the catalog via
	// [ExpirationPolicy.CatalogHeadsSize]; it is useful for callers that have no
	// catalog at hand, e.g. the pre-start cleanup.
	ExtraSize func() int64
}

//
// ExpirationPolicy
//

// ExpirationPolicy is the expiration policy for the [block.Block]s.
// It is used to determine which blocks should be deleted based on the time and size retention policies.
type ExpirationPolicy[TBlock Block] struct {
	dir     string
	c       Catalog
	opts    *Options
	metrics *Metrics
}

// NewExpirationPolicy creates a new [ExpirationPolicy].
func NewExpirationPolicy[TBlock Block](
	dir string,
	c Catalog,
	opts *Options,
	r prometheus.Registerer,
) *ExpirationPolicy[TBlock] {
	ep := &ExpirationPolicy[TBlock]{
		dir:     dir,
		c:       c,
		opts:    opts,
		metrics: NewMetrics(r),
	}

	// Report the configured retention constraints
	limitBytes := max(ep.opts.MaxBytes, 0)
	ep.metrics.maxBytes.Set(float64(limitBytes))
	ep.metrics.retentionDuration.Set((time.Duration(ep.opts.RetentionDuration) * time.Millisecond).Seconds())

	return ep
}

// BeyondSizeRetention returns those blocks which are beyond the size retention.
//
//revive:disable-next-line:cyclomatic // complex logic is necessary for this function
//revive:disable-next-line:function-length // complex logic is necessary for this function
func (ep *ExpirationPolicy[TBlock]) BeyondSizeRetention(
	rawBlocks, downsampledBlocks []TBlock,
	deletable map[ulid.ULID]struct{},
) {
	// Size retention is disabled or no blocks to work with.
	if (len(rawBlocks) == 0 && len(downsampledBlocks) == 0) || ep.opts.MaxBytes <= 0 {
		return
	}

	var reachedLimit bool

	// Initializing size counter with the injected extra size (heads + catalog).
	blocksSize := ep.extraSize()
	for i, blk := range rawBlocks {
		if _, ok := deletable[blk.ULID()]; ok {
			// This block is already marked for deletion, so we don't count its size.
			continue
		}

		blocksSize += blk.Size()
		if blocksSize > ep.opts.MaxBytes {
			// Add this and all following blocks for deletion.
			for _, b := range rawBlocks[i:] {
				deletable[b.ULID()] = struct{}{}
			}

			// Exclude the blocks just marked for deletion from the budget, so the
			// downsampled loop below sees the size of the set that will actually remain.
			blocksSize -= blk.Size()
			reachedLimit = true

			break
		}
	}

	for i, blk := range downsampledBlocks {
		if _, ok := deletable[blk.ULID()]; ok {
			// This block is already marked for deletion, so we don't count its size.
			continue
		}

		blocksSize += blk.Size()
		if blocksSize > ep.opts.MaxBytes {
			// Add this and all following blocks for deletion.
			for _, b := range downsampledBlocks[i:] {
				deletable[b.ULID()] = struct{}{}
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
func (ep *ExpirationPolicy[TBlock]) BeyondTimeRetention(rawBlocks []TBlock, deletable map[ulid.ULID]struct{}) {
	// Time retention is disabled or no blocks to work with.
	if len(rawBlocks) == 0 || ep.opts.RetentionDuration == 0 {
		return
	}

	for i, blk := range rawBlocks {
		// The difference between the first block and this block is greater than or equal to
		// the retention period so any blocks after that are added as deletable.
		if i > 0 && rawBlocks[0].MaxTime()-blk.MaxTime() >= ep.opts.RetentionDuration {
			for _, b := range rawBlocks[i:] {
				deletable[b.ULID()] = struct{}{}
			}

			ep.metrics.timeRetentions.Inc()
			break
		}
	}
}

// BlocksToDelete returns the blocks that should be deleted based on the retention policy
// or already compacted into a new block.
func (ep *ExpirationPolicy[TBlock]) BlocksToDelete(blocks []TBlock) map[ulid.ULID]struct{} {
	if len(blocks) == 0 {
		return nil
	}

	deletable := make(map[ulid.ULID]struct{})
	for _, blk := range blocks {
		if blk.Deletable() {
			deletable[blk.ULID()] = struct{}{}
		}
	}

	// Split the blocks into downsampled and raw blocks.
	rawBlocks, downsampledBlocks := splitBlocks(blocks)

	// Beyond time retention.
	ep.BeyondTimeRetention(rawBlocks, deletable)

	// Beyond size retention.
	ep.BeyondSizeRetention(rawBlocks, downsampledBlocks, deletable)

	return deletable
}

// CatalogHeadsSize returns the on-disk size of the catalog and all of its heads.
// It returns 0 when the policy was created without a catalog.
func (ep *ExpirationPolicy[TBlock]) CatalogHeadsSize() int64 {
	if ep.c == nil {
		return 0
	}

	catalogSize := ep.c.OnDiskSize()
	heads := ep.c.List(nil, nil)
	for _, h := range heads {
		catalogSize += headSize(filepath.Join(ep.dir, h.Dir()))
	}

	return catalogSize
}

// extraSize returns the on-disk bytes used outside of the blocks.
func (ep *ExpirationPolicy[TBlock]) extraSize() int64 {
	if ep.opts.ExtraSize != nil {
		return ep.opts.ExtraSize()
	}

	return ep.CatalogHeadsSize()
}

// headSize returns the on-disk size of a head directory.
func headSize(dir string) int64 {
	size, _ := fileutil.DirSize(dir)
	return size
}

// splitBlocks splits the blocks into downsampled and raw blocks.
func splitBlocks[TBlock Block](blocks []TBlock) (rawBlocks, downsampledBlocks []TBlock) {
	// Sort the blocks by time - newest to oldest (largest to smallest timestamp).
	// This ensures that the retentions will remove the oldest  blocks.
	slices.SortFunc(blocks, func(a, b TBlock) int {
		switch {
		case b.MaxTime() < a.MaxTime():
			return -1
		case b.MaxTime() > a.MaxTime():
			return 1
		default:
			return 0
		}
	})

	rawBlocks = make([]TBlock, 0, len(blocks)/2)
	downsampledBlocks = make([]TBlock, 0, len(blocks)/2)
	for _, blk := range blocks {
		if blk.IsDownsamplingBlock() {
			downsampledBlocks = append(downsampledBlocks, blk)
			continue
		}

		rawBlocks = append(rawBlocks, blk)
	}

	return rawBlocks, downsampledBlocks
}
