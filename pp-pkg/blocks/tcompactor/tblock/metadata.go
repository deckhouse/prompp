package tblock

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/go-kit/log"
	"github.com/thanos-io/thanos/pkg/block/metadata"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp-pkg/blocks/block"
	"github.com/prometheus/prometheus/tsdb"
)

// WriteThanosMetaFileAdapter is a function that writes the Thanos meta file to the directory.
//
//revive:disable-next-line:flag-parameter // this is not a flag, but a parameter
func WriteThanosMetaFileAdapter(
	ctx context.Context,
	resolution int64,
	ls labels.Labels,
	acceptMalformedIndex bool,
) func(logger log.Logger, dir string, meta *tsdb.BlockMeta) (int64, error) {
	return func(logger log.Logger, dir string, meta *tsdb.BlockMeta) (int64, error) {
		if meta.Compaction.IsCorrupted() || meta.Compaction.Failed {
			rmeta, _, err := block.ReadFromDir(dir)
			if err != nil {
				return 0, err
			}

			rmeta.BlockMeta = *meta

			return block.WriteThanosMetaFile(logger, dir, rmeta)
		}

		// Ensure the output block is valid.
		stats, err := GatherIndexHealthStats(
			ctx,
			logger,
			filepath.Join(dir, block.IndexFilename),
			meta.MinTime,
			meta.MaxTime,
		)
		if !acceptMalformedIndex && errors.Join(err, stats.AnyErr()) != nil {
			return 0, fmt.Errorf("invalid result block %s: %w", dir, errors.Join(err, stats.AnyErr()))
		}

		return block.WriteThanosMetaFile(
			logger,
			dir,
			&metadata.Meta{
				BlockMeta: *meta,
				Thanos: metadata.Thanos{
					Labels:       ls.Map(),
					Downsample:   metadata.ThanosDownsample{Resolution: resolution},
					Source:       metadata.CompactorSource,
					SegmentFiles: GetSegmentFiles(dir),
					IndexStats: metadata.IndexStats{
						ChunkMaxSize:  stats.ChunkMaxSize,
						SeriesMaxSize: stats.SeriesMaxSize,
					},
				},
			},
		)
	}
}
