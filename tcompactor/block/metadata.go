package block

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/thanos-io/thanos/pkg/block/metadata"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/fileutil"
)

const (
	// metaFilename is the filename of the block meta file.
	metaFilename = "meta.json"

	// metaVersion1 is the version of the block meta file.
	metaVersion1 = 1
)

func WriteThanosMetaFile(
	ctx context.Context,
	resolution int64,
	labels labels.Labels,
	acceptMalformedIndex bool,
) func(logger log.Logger, dir string, meta *tsdb.BlockMeta) (int64, error) {
	return func(logger log.Logger, dir string, meta *tsdb.BlockMeta) (int64, error) {
		index := filepath.Join(dir, IndexFilename)

		// Ensure the output block is valid.
		stats, err := GatherIndexHealthStats(ctx, logger, index, meta.MinTime, meta.MaxTime)
		if !acceptMalformedIndex && errors.Join(err, stats.AnyErr()) != nil {
			return 0, fmt.Errorf("invalid result block %s: %w", dir, errors.Join(err, stats.AnyErr()))
		}

		meta.Version = metaVersion1
		tmeta := &metadata.Meta{
			BlockMeta: *meta,
			Thanos: metadata.Thanos{
				Labels:       labels.Map(),
				Downsample:   metadata.ThanosDownsample{Resolution: resolution},
				Source:       metadata.CompactorSource,
				SegmentFiles: GetSegmentFiles(dir),
				IndexStats:   metadata.IndexStats{ChunkMaxSize: stats.ChunkMaxSize, SeriesMaxSize: stats.SeriesMaxSize},
			},
		}

		// Make any changes to the file appear atomic.
		path := filepath.Join(dir, metaFilename)
		tmp := path + ".tmp"
		defer func() {
			if err := os.RemoveAll(tmp); err != nil {
				level.Error(logger).Log("msg", "remove tmp file", "err", err.Error())
			}
		}()

		f, err := os.Create(tmp)
		if err != nil {
			return 0, err
		}

		jsonMeta, err := json.MarshalIndent(tmeta, "", "\t")
		if err != nil {
			return 0, errors.Join(err, f.Close())
		}

		n, err := f.Write(jsonMeta)
		if err != nil {
			return 0, errors.Join(err, f.Close())
		}

		// Force the kernel to persist the file on disk to avoid data loss if the host crashes.
		if err := f.Sync(); err != nil {
			return 0, errors.Join(err, f.Close())
		}

		if err := f.Close(); err != nil {
			return 0, err
		}

		return int64(n), fileutil.Replace(tmp, path)
	}
}
