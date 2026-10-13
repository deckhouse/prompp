package tblock

import (
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
	resolution int64,
	ls labels.Labels,
) func(logger log.Logger, dir string, meta *tsdb.BlockMeta) (int64, error) {
	return func(logger log.Logger, dir string, meta *tsdb.BlockMeta) (int64, error) {
		if meta.Compaction.IsCorrupted() || meta.Compaction.Failed || meta.Compaction.Deletable {
			rmeta, _, err := block.ReadFromDir(dir)
			if err != nil {
				return 0, err
			}

			rmeta.BlockMeta = *meta

			return block.WriteThanosMetaFile(logger, dir, rmeta)
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
				},
			},
		)
	}
}
