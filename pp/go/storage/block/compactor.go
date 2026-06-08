package block

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

const compactionInterval = time.Minute

// CompactorOptions configures the persisted-blocks compactor.
type CompactorOptions struct {
	// MinBlockDuration is the smallest block range, used to derive the
	// exponential compaction ranges. If zero, tsdb.DefaultBlockDuration is used.
	MinBlockDuration int64
	// MaxBlockChunkSegmentSize is the max block chunk segment size.
	MaxBlockChunkSegmentSize int64
	// EnableOverlappingCompaction enables compaction of overlapping blocks.
	EnableOverlappingCompaction bool
	// CompactionInterval is the period of the background compaction loop.
	// If zero, compactionInterval is used.
	CompactionInterval time.Duration
}

// BlockSource provides the compactor with the currently loaded blocks.
// It is implemented by Manager.
type BlockSource interface {
	// Blocks returns a snapshot of the currently loaded blocks (the open
	// argument for Compact).
	Blocks() []*tsdb.Block
}

// Compactor periodically compacts persisted on-disk blocks. It does not reload
// blocks itself: the new block is loaded and the compacted parents are deleted
// by the periodic reload loop of the block source (e.g. Manager).
type Compactor struct {
	dir       string
	compactor tsdb.Compactor
	source    BlockSource
	interval  time.Duration
	logger    log.Logger
	metrics   *compactorMetrics

	stopc    chan struct{}
	stoppedc chan struct{}
	stopOnce sync.Once
}

// NewCompactor builds a LeveledCompactor from opts and starts the background
// compaction loop.
func NewCompactor(
	ctx context.Context,
	dir string,
	opts *CompactorOptions,
	source BlockSource,
	logger log.Logger,
	r prometheus.Registerer,
) (*Compactor, error) {
	if opts == nil {
		opts = &CompactorOptions{}
	}
	if logger == nil {
		logger = log.NewNopLogger()
	}

	minBlockDuration := opts.MinBlockDuration
	if minBlockDuration <= 0 {
		minBlockDuration = tsdb.DefaultBlockDuration
	}
	interval := opts.CompactionInterval
	if interval <= 0 {
		interval = compactionInterval
	}

	rngs := tsdb.ExponentialBlockRanges(minBlockDuration, 10, 3)
	leveled, err := tsdb.NewLeveledCompactorWithOptions(ctx, r, logger, rngs, chunkenc.NewPool(), tsdb.LeveledCompactorOptions{
		MaxBlockChunkSegmentSize:    opts.MaxBlockChunkSegmentSize,
		EnableOverlappingCompaction: opts.EnableOverlappingCompaction,
	})
	if err != nil {
		return nil, fmt.Errorf("create compactor: %w", err)
	}

	c := &Compactor{
		dir:       dir,
		compactor: leveled,
		source:    source,
		interval:  interval,
		logger:    logger,
		metrics:   newCompactorMetrics(r),
		stopc:     make(chan struct{}),
		stoppedc:  make(chan struct{}),
	}
	go c.loop()
	return c, nil
}

func (c *Compactor) loop() {
	defer func() {
		close(c.stoppedc)
	}()

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.metrics.compactionsTriggered.Inc()
			if err := c.compactBlocks(); err != nil {
				c.metrics.compactionsFailed.Inc()
				level.Error(c.logger).Log("msg", "compaction failed", "err", err)
			}

		case <-c.stopc:
			return
		}
	}
}

// Close stops the compaction loop and waits for it to finish.
func (c *Compactor) Close() {
	c.stopOnce.Do(func() {
		close(c.stopc)
	})
	<-c.stoppedc
}

// compactBlocks compacts at most one planned group of eligible on-disk blocks.
// It does not reload blocks: the periodic reload loop of the block source loads
// the new block and deletes the compacted parents.
func (c *Compactor) compactBlocks() error {
	plan, err := c.compactor.Plan(c.dir)
	if err != nil {
		return fmt.Errorf("plan compaction: %w", err)
	}
	if len(plan) == 0 {
		return nil
	}

	select {
	case <-c.stopc:
		return nil
	default:
	}

	if _, err := c.compactor.Compact(c.dir, plan, c.source.Blocks()); err != nil {
		return fmt.Errorf("compact %s: %w", plan, err)
	}
	return nil
}

//
// metrics
//

type compactorMetrics struct {
	compactionsTriggered prometheus.Counter
	compactionsFailed    prometheus.Counter
}

func newCompactorMetrics(r prometheus.Registerer) *compactorMetrics {
	m := &compactorMetrics{
		compactionsTriggered: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_compactions_triggered_total",
			Help: "Total number of triggered compactions for the partition.",
		}),
		compactionsFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "prometheus_tsdb_compactions_failed_total",
			Help: "Total number of compactions that failed for the partition.",
		}),
	}

	if r != nil {
		r.MustRegister(
			m.compactionsTriggered,
			m.compactionsFailed,
		)
	}

	return m
}
