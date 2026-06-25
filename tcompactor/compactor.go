package tcompactor

import (
	"context"
	"fmt"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/thanos-io/objstore"
	"github.com/thanos-io/objstore/client"
	objstoretracing "github.com/thanos-io/objstore/tracing/opentracing"
	"github.com/thanos-io/thanos/pkg/block/metadata"
	"github.com/thanos-io/thanos/pkg/compact"
	"github.com/thanos-io/thanos/pkg/extprom"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// Planner returns blocks to compact.
type Planner interface {
	// Plan returns a list of blocks that should be compacted into single one.
	// The blocks can be overlapping. The provided metadata has to be ordered by minTime.
	Plan(
		ctx context.Context,
		metasByMinTime []*metadata.Meta,
		errChan chan error,
		extensions any,
	) ([]*metadata.Meta, error)
}

var _ tsdb.Compactor = (*TCompactor)(nil)

type TCompactor struct {
	lCompactor *tsdb.LeveledCompactor
	grouper    *DefaultGrouper
	planner    Planner
}

func NewThanosCompactor(
	ctx context.Context,
	reg prometheus.Registerer,
	logger log.Logger,
	ranges []int64,
	pool chunkenc.Pool,
	opts tsdb.LeveledCompactorOptions,
) (*TCompactor, error) {
	// TODO:
	// mergeFunc storage.VerticalChunkSeriesMergeFunc
	// pool downsample.NewPool()
	// compactDir      = path.Join(conf.dataDir, "compact")
	confContentYaml := []byte(`
		bucket_type: s3
		bucket_name: test
		bucket_region: us-east-1
		bucket_access_key: test
		bucket_secret_key: test
		bucket_endpoint: http://localhost:9000
		bucket_path_prefix: test
	`)

	bkt, err := client.NewBucket(logger, confContentYaml, "thanos", nil)
	if err != nil {
		return nil, fmt.Errorf("create bucket: %w", err)
	}
	insBkt := objstoretracing.WrapWithTraces(objstore.WrapWithMetrics(
		bkt,
		extprom.WrapRegistererWithPrefix("thanos_", reg),
		bkt.Name(),
	))

	blockMetaFetchConcurrency := 10
	noCompactMarkerFilter := compact.NewGatherNoCompactionMarkFilter(logger, insBkt, blockMetaFetchConcurrency)

	tsdbPlanner := compact.NewPlanner(logger, ranges, noCompactMarkerFilter)
	// tsdbPlanner.Plan()
	_ = tsdbPlanner

	// grouper := compact.NewDefaultGrouper(
	// 	logger,
	// 	insBkt,
	// 	conf.acceptMalformedIndex,
	// 	enableVerticalCompaction,
	// 	reg,
	// 	compactMetrics.blocksMarked.WithLabelValues(metadata.DeletionMarkFilename, ""),
	// 	compactMetrics.garbageCollectedBlocks,
	// 	compactMetrics.blocksMarked.WithLabelValues(metadata.NoCompactMarkFilename, metadata.OutOfOrderChunksNoCompactReason),
	// 	metadata.HashFunc(conf.hashFunc),
	// 	conf.blockFilesConcurrency,
	// 	conf.compactBlocksFetchConcurrency,
	// )
	// grouper.Groups(nil)

	lCompactor, err := tsdb.NewLeveledCompactorWithOptions(ctx, reg, logger, ranges, pool, opts)
	if err != nil {
		return nil, fmt.Errorf("create leveled compactor: %w", err)
	}

	// bCompactor, err := compact.NewBucketCompactor(
	// 	logger,
	// 	sy,
	// 	grouper,
	// 	planner,
	// 	lCompactor,
	// 	compactDir,
	// 	insBkt,
	// 	conf.compactionConcurrency,
	// 	conf.skipBlockWithOutOfOrderChunks,
	// )
	// if err != nil {
	// 	return nil, fmt.Errorf("create bucket compactor: %w", err)
	// }

	return &TCompactor{
		lCompactor: lCompactor,
		// bCompactor: bCompactor,
		// grouper:    grouper,
	}, nil
}

func (c *TCompactor) Plan(dir string) ([]string, error) {
	return c.lCompactor.Plan(dir)
}

func (c *TCompactor) Write(dest string, b tsdb.BlockReader, mint, maxt int64, base *tsdb.BlockMeta) ([]ulid.ULID, error) {
	return c.lCompactor.Write(dest, b, mint, maxt, base)
}

func (c *TCompactor) Compact(dest string, dirs []string, open []*tsdb.Block) ([]ulid.ULID, error) {
	return c.lCompactor.Compact(dest, dirs, open)
}

// Close stops the compaction loop and waits for it to finish.
func (c *TCompactor) Close() {
	// TODO: implement
}
