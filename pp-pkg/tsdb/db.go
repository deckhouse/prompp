package tsdb

import (
	"path/filepath"
	"slices"
	"time"

	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/fileutil"
)

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
	m := &Metrics{
		timeRetentions: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_time_retentions_total",
			Help: "The number of times that blocks were deleted because the maximum time limit was exceeded.",
		}),
		sizeRetentions: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_size_retentions_total",
			Help: "The number of times that blocks were deleted because the maximum number of bytes was exceeded.",
		}),
		maxBytes: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_retention_limit_bytes",
			Help: "Max number of bytes to be retained in the tsdb blocks, configured 0 means disabled",
		}),
		retentionDuration: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_retention_limit_seconds",
			Help: "How long to retain samples in storage.",
		}),
	}

	if r != nil {
		r.MustRegister(m.timeRetentions, m.sizeRetentions, m.maxBytes, m.retentionDuration)
	}

	return m
}

// NewBlocksToDelete returns a filter which decides time based and size based
// retention. It does not depend on a *tsdb.DB, so it can be used by components
// that own no DB instance. It creates and registers its own retention metrics
// (counters + limit gauges) via r and reports the configured constraints.
//
// extraSize reports bytes used outside of the given blocks (e.g. heads + catalog)
// and may be nil.
func NewBlocksToDelete(
	retentionDuration, maxBytes int64,
	extraSize func() int64,
	r prometheus.Registerer,
) tsdb.BlocksToDeleteFunc {
	m := NewMetrics(r)

	// Report the configured retention constraints, mirroring tsdb.DB.open.
	limitBytes := maxBytes
	if limitBytes < 0 {
		limitBytes = 0
	}
	m.maxBytes.Set(float64(limitBytes))
	m.retentionDuration.Set((time.Duration(retentionDuration) * time.Millisecond).Seconds())

	return func(blocks []*tsdb.Block) map[ulid.ULID]struct{} {
		return deletableBlocks(retentionDuration, maxBytes, extraSize, m.timeRetentions, m.sizeRetentions, blocks)
	}
}

// deletableBlocks returns all currently loaded blocks past retention policy or already compacted into a new block.
func deletableBlocks(
	retentionDuration, maxBytes int64,
	extraSize func() int64,
	timeRetentions, sizeRetentions prometheus.Counter,
	blocks []*tsdb.Block,
) map[ulid.ULID]struct{} {
	deletable := make(map[ulid.ULID]struct{})

	// Sort the blocks by time - newest to oldest (largest to smallest timestamp).
	// This ensures that the retentions will remove the oldest  blocks.
	slices.SortFunc(blocks, func(a, b *tsdb.Block) int {
		switch {
		case b.Meta().MaxTime < a.Meta().MaxTime:
			return -1
		case b.Meta().MaxTime > a.Meta().MaxTime:
			return 1
		default:
			return 0
		}
	})

	for _, block := range blocks {
		if block.Meta().Compaction.Deletable {
			deletable[block.Meta().ULID] = struct{}{}
		}
	}

	for ulid := range BeyondTimeRetention(retentionDuration, timeRetentions, blocks) {
		deletable[ulid] = struct{}{}
	}

	for ulid := range BeyondSizeRetention(maxBytes, extraSize, sizeRetentions, blocks) {
		deletable[ulid] = struct{}{}
	}

	return deletable
}

// BeyondTimeRetention returns those blocks which are beyond the time retention.
func BeyondTimeRetention(
	retentionDuration int64,
	timeRetentions prometheus.Counter,
	blocks []*tsdb.Block,
) (deletable map[ulid.ULID]struct{}) {
	// Time retention is disabled or no blocks to work with.
	if len(blocks) == 0 || retentionDuration == 0 {
		return
	}

	deletable = make(map[ulid.ULID]struct{})
	for i, block := range blocks {
		// The difference between the first block and this block is greater than or equal to
		// the retention period so any blocks after that are added as deletable.
		if i > 0 && blocks[0].Meta().MaxTime-block.Meta().MaxTime >= retentionDuration {
			for _, b := range blocks[i:] {
				deletable[b.Meta().ULID] = struct{}{}
			}
			if timeRetentions != nil {
				timeRetentions.Inc()
			}
			break
		}
	}
	return deletable
}

// BeyondSizeRetention returns those blocks which are beyond the size retention.
func BeyondSizeRetention(
	maxBytes int64,
	extraSize func() int64,
	sizeRetentions prometheus.Counter,
	blocks []*tsdb.Block,
) (deletable map[ulid.ULID]struct{}) {
	// Size retention is disabled or no blocks to work with.
	if len(blocks) == 0 || maxBytes <= 0 {
		return
	}

	deletable = make(map[ulid.ULID]struct{})

	// Initializing size counter with the injected extra size (heads + catalog).
	var blocksSize int64
	if extraSize != nil {
		blocksSize = extraSize()
	}
	for i, block := range blocks {
		blocksSize += block.Size()
		if blocksSize > maxBytes {
			// Add this and all following blocks for deletion.
			for _, b := range blocks[i:] {
				deletable[b.Meta().ULID] = struct{}{}
			}
			if sizeRetentions != nil {
				sizeRetentions.Inc()
			}
			break
		}
	}
	return deletable
}

// CatalogHeadsExtraSize adapts [CatalogHeadsSize] into an extraSize function
// (func() int64) suitable for passing to [NewBlocksToDelete].
func CatalogHeadsExtraSize(dir string, catalog *catalog.Catalog) func() int64 {
	return func() int64 {
		return CatalogHeadsSize(dir, catalog)
	}
}

// CatalogHeadsSize returns the on-disk size of the catalog and all of its heads.
// It is useful to build the extraSize function passed to NewBlocksToDelete.
func CatalogHeadsSize(dir string, catalog *catalog.Catalog) (catalogSize int64) {
	catalogSize += catalog.OnDiskSize()
	heads := catalog.List(nil, nil)
	for _, h := range heads {
		catalogSize += headSize(filepath.Join(dir, h.Dir()))
	}
	return catalogSize
}

func headSize(dir string) int64 {
	size, _ := fileutil.DirSize(dir)
	return size
}
