package tsdb

import (
	"context"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// BlockCompactionDisabled default compaction flag value.
var BlockCompactionDisabled = false

type NonBlockCompactor struct {
	compactor *tsdb.LeveledCompactor
}

func (c *NonBlockCompactor) Plan(dir string) ([]string, error) {
	return nil, nil
}

func (c *NonBlockCompactor) Write(dest string, b tsdb.BlockReader, mint, maxt int64, base *tsdb.BlockMeta) ([]ulid.ULID, error) {
	return c.compactor.Write(dest, b, mint, maxt, base)
}

func (c *NonBlockCompactor) Compact(dest string, dirs []string, open []*tsdb.Block) ([]ulid.ULID, error) {
	return nil, nil
}

func CreateNonBlockCompactor(ctx context.Context, r prometheus.Registerer, l log.Logger, ranges []int64, pool chunkenc.Pool, opts *tsdb.Options) (tsdb.Compactor, error) {
	compactor, err := tsdb.NewLeveledCompactorWithOptions(ctx, r, l, ranges, pool, tsdb.LeveledCompactorOptions{
		MaxBlockChunkSegmentSize:    opts.MaxBlockChunkSegmentSize,
		EnableOverlappingCompaction: opts.EnableOverlappingCompaction,
	})
	if err != nil {
		return nil, err
	}

	return &NonBlockCompactor{
		compactor: compactor,
	}, nil
}
