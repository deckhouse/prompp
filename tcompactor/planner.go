package tcompactor

import (
	"context"
	"fmt"
	"io"
	"maps"
	"math"
	"path/filepath"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/thanos-io/objstore"
	"github.com/thanos-io/thanos/pkg/block/metadata"

	"github.com/prometheus/prometheus/tcompactor/block"
)

//
// NoCompactionMarkFilter
//

// NoCompactionMarkFilter is a filter that returns block ids that were marked for no compaction.
type NoCompactionMarkFilter interface {
	// NoCompactMarkedBlocks returns block ids that were marked for no compaction.
	NoCompactMarkedBlocks() map[ulid.ULID]*metadata.NoCompactMark
}

//
// BlockWorker
//

// BlockWorker is a worker that can work with blocks.
type BlockWorker interface {
	// Attributes returns information about the specified object.
	Attributes(ctx context.Context, name string) (objstore.ObjectAttributes, error)

	// Exists checks if the given object exists in the destination.
	Exists(ctx context.Context, name string) (bool, error)

	// Upload uploads the given object to the destination.
	Upload(ctx context.Context, name string, r io.Reader) error
}

//
// NoopNoCompactionMark
//

// NoopNoCompactionMark is a no-op implementation of [NoCompactionMarkFilter].
type NoopNoCompactionMark struct{}

// NoCompactMarkedBlocks implementation of [NoCompactionMarkFilter], do nothing, returns nil.
func (NoopNoCompactionMark) NoCompactMarkedBlocks() map[ulid.ULID]*metadata.NoCompactMark {
	return nil
}

//
// TsdbBasedPlanner
//

var _ Planner = (*TsdbBasedPlanner)(nil)

// TsdbBasedPlanner is a Thanos planner with the same functionality as Prometheus' TSDB
// plus special handling of excluded blocks. It's the same functionality just without accessing filesystem,
// and special handling of excluded blocks.
type TsdbBasedPlanner struct {
	logger                      log.Logger
	ranges                      []int64
	noCompBlocksFunc            func() map[ulid.ULID]*metadata.NoCompactMark
	enableOverlappingCompaction bool
}

// NewPlanner initializes a new [tsdbBasedPlanner].
func NewPlanner(
	logger log.Logger,
	ranges []int64,
	noCompBlocks NoCompactionMarkFilter,
	enableOverlappingCompaction bool,
) (*TsdbBasedPlanner, error) {
	if len(ranges) == 0 {
		return nil, fmt.Errorf("at least one range must be provided")
	}

	if logger == nil {
		logger = log.NewNopLogger()
	}

	return &TsdbBasedPlanner{
		logger:                      logger,
		ranges:                      ranges,
		noCompBlocksFunc:            noCompBlocks.NoCompactMarkedBlocks,
		enableOverlappingCompaction: enableOverlappingCompaction,
	}, nil
}

// Plan is the main function that plans the compaction of the blocks.
func (p *TsdbBasedPlanner) Plan(_ context.Context, metasByMinTime []*metadata.Meta) ([]*metadata.Meta, bool, error) {
	metas, overlappingBlocks := p.plan(p.noCompBlocksFunc(), metasByMinTime)
	return metas, overlappingBlocks, nil
}

// plan is the main function that plans the compaction of the blocks.
func (p *TsdbBasedPlanner) plan(
	noCompactMarked map[ulid.ULID]*metadata.NoCompactMark,
	metasByMinTime []*metadata.Meta,
) ([]*metadata.Meta, bool) {
	notExcludedMetasByMinTime := make([]*metadata.Meta, 0, len(metasByMinTime))
	for _, meta := range metasByMinTime {
		if _, excluded := noCompactMarked[meta.ULID]; excluded {
			continue
		}
		notExcludedMetasByMinTime = append(notExcludedMetasByMinTime, meta)
	}

	res := p.selectOverlappingMetas(notExcludedMetasByMinTime)
	if len(res) > 0 {
		return res, true
	}
	// No overlapping blocks, do compaction the usual way.

	// We do not include a recently produced block with max(minTime), so the block which was just uploaded to bucket.
	// This gives users a window of a full block size maintenance if needed.
	if _, excluded := noCompactMarked[metasByMinTime[len(metasByMinTime)-1].ULID]; !excluded {
		notExcludedMetasByMinTime = notExcludedMetasByMinTime[:len(notExcludedMetasByMinTime)-1]
	}
	metasByMinTime = metasByMinTime[:len(metasByMinTime)-1]
	res = append(res, selectMetas(p.ranges, noCompactMarked, metasByMinTime)...)
	if len(res) > 0 {
		return res, false
	}

	// Compact any blocks with big enough time range that have >5% tombstones.
	for i := len(notExcludedMetasByMinTime) - 1; i >= 0; i-- {
		meta := notExcludedMetasByMinTime[i]
		if meta.MaxTime-meta.MinTime < p.ranges[len(p.ranges)/2] {
			break
		}
		if float64(meta.Stats.NumTombstones)/float64(meta.Stats.NumSeries+1) > 0.05 {
			return []*metadata.Meta{notExcludedMetasByMinTime[i]}, false
		}
	}

	return nil, false
}

// selectOverlappingMetas returns all dirs with overlapping time ranges.
// It expects sorted input by mint and returns the overlapping dirs in the same order as received.
func (p *TsdbBasedPlanner) selectOverlappingMetas(metasByMinTime []*metadata.Meta) []*metadata.Meta {
	if !p.enableOverlappingCompaction {
		return nil
	}

	if len(metasByMinTime) < 2 {
		return nil
	}

	var overlappingMetas []*metadata.Meta
	globalMaxt := metasByMinTime[0].MaxTime
	for i, m := range metasByMinTime[1:] {
		if m.MinTime < globalMaxt {
			if len(overlappingMetas) == 0 {
				// When it is the first overlap, need to add the last one as well.
				overlappingMetas = append(overlappingMetas, metasByMinTime[i])
			}
			overlappingMetas = append(overlappingMetas, m)
		} else if len(overlappingMetas) > 0 {
			break
		}

		if m.MaxTime > globalMaxt {
			globalMaxt = m.MaxTime
		}
	}

	return overlappingMetas
}

// selectMetas returns the dir metas that should be compacted into a single new block.
// If only a single block range is configured, the result is always nil.
func selectMetas(
	ranges []int64,
	noCompactMarked map[ulid.ULID]*metadata.NoCompactMark,
	metasByMinTime []*metadata.Meta,
) []*metadata.Meta {
	if len(ranges) < 2 || len(metasByMinTime) < 1 {
		return nil
	}

	highTime := metasByMinTime[len(metasByMinTime)-1].MinTime

	for _, iv := range ranges[1:] {
		parts := splitByRange(metasByMinTime, iv)
		if len(parts) == 0 {
			continue
		}
	Outer:
		for _, p := range parts {
			// Do not select the range if it has a block whose compaction failed.
			for _, m := range p {
				if m.Compaction.Failed {
					continue Outer
				}
			}

			if len(p) < 2 {
				continue
			}

			mint := p[0].MinTime
			maxt := p[len(p)-1].MaxTime

			// Pick the range of blocks if it spans the full range (potentially with gaps) or is before the most recent block.
			// This ensures we don't compact blocks prematurely when another one of the same size still would fits in the range
			// after upload.
			if maxt-mint != iv && maxt > highTime {
				continue
			}

			// Check if any of resulted blocks are excluded. Exclude them in a way that does not introduce gaps to the system
			// as well as preserve the ranges that would be used if they were not excluded.
			// This is meant as short-term workaround to create ability for marking some blocks to not be touched for compaction.
			lastExcluded := 0
			for i, id := range p {
				if _, excluded := noCompactMarked[id.ULID]; !excluded {
					continue
				}

				if len(p[lastExcluded:i]) > 1 {
					return p[lastExcluded:i]
				}

				lastExcluded = i + 1
			}

			if len(p[lastExcluded:]) > 1 {
				return p[lastExcluded:]
			}
		}
	}

	return nil
}

// splitByRange splits the directories by the time range. The range sequence starts at 0.
//
// For example, if we have blocks [0-10, 10-20, 50-60, 90-100] and the split range tr is 30
// it returns [0-10, 10-20], [50-60], [90-100].
func splitByRange(metasByMinTime []*metadata.Meta, tr int64) [][]*metadata.Meta {
	var splitDirs [][]*metadata.Meta

	for i := 0; i < len(metasByMinTime); {
		var (
			group []*metadata.Meta
			t0    int64
			m     = metasByMinTime[i]
		)

		// Compute start of aligned time range of size tr closest to the current block's start.
		if m.MinTime >= 0 {
			t0 = tr * (m.MinTime / tr)
		} else {
			t0 = tr * ((m.MinTime - tr + 1) / tr)
		}

		// Skip blocks that don't fall into the range. This can happen via misalignment or
		// by being the multiple of the intended range.
		if m.MaxTime > t0+tr {
			i++
			continue
		}

		// Add all metas to the current group that are within [t0, t0+tr].
		for ; i < len(metasByMinTime); i++ {
			// Either the block falls into the next range or doesn't fit at all (checked above).
			if metasByMinTime[i].MaxTime > t0+tr {
				break
			}
			group = append(group, metasByMinTime[i])
		}

		if len(group) > 0 {
			splitDirs = append(splitDirs, group)
		}
	}

	return splitDirs
}

//
// largeTotalIndexSizeFilter
//

var _ Planner = (*largeTotalIndexSizeFilter)(nil)

// largeTotalIndexSizeFilter is a planner that plans the compaction of the blocks without large index file size.
type largeTotalIndexSizeFilter struct {
	*TsdbBasedPlanner

	bw                     BlockWorker
	totalMaxIndexSizeBytes int64
	markedForNoCompact     prometheus.Counter
}

// WithLargeTotalIndexSizeFilter wraps Planner with largeTotalIndexSizeFilter that checks the given plans
// and estimates total index size. When found, it marks block for no compaction by placing no-compact-mark.json
// and updating cache. NOTE: The estimation is very rough as it assumes extreme cases of indexes sharing no bytes,
// thus summing all source index sizes. Adjust limit accordingly reducing to some % of actual limit you want to give.
func WithLargeTotalIndexSizeFilter(
	with *TsdbBasedPlanner,
	bw BlockWorker,
	totalMaxIndexSizeBytes int64,
	markedForNoCompact prometheus.Counter,
) *largeTotalIndexSizeFilter { //revive:disable-line:unexported-return // used as [Planner]
	return &largeTotalIndexSizeFilter{
		TsdbBasedPlanner:       with,
		bw:                     bw,
		totalMaxIndexSizeBytes: totalMaxIndexSizeBytes,
		markedForNoCompact:     markedForNoCompact,
	}
}

// Plan is the main function that plans the compaction of the blocks without large index file size.
func (t *largeTotalIndexSizeFilter) Plan(
	ctx context.Context,
	metasByMinTime []*metadata.Meta,
) ([]*metadata.Meta, bool, error) {
	return t.plan(ctx, nil, metasByMinTime)
}

// plan is the main function that plans the compaction of the blocks without large index file size.
func (t *largeTotalIndexSizeFilter) plan(
	ctx context.Context,
	extraNoCompactMarked map[ulid.ULID]*metadata.NoCompactMark,
	metasByMinTime []*metadata.Meta,
) ([]*metadata.Meta, bool, error) {
	noCompactMarked := t.noCompBlocksFunc()
	copiedNoCompactMarked := make(map[ulid.ULID]*metadata.NoCompactMark, len(noCompactMarked)+len(extraNoCompactMarked))
	maps.Copy(copiedNoCompactMarked, noCompactMarked)
	maps.Copy(copiedNoCompactMarked, extraNoCompactMarked)

PlanLoop:
	for {
		plan, overlappingBlocks := t.TsdbBasedPlanner.plan(copiedNoCompactMarked, metasByMinTime)

		var totalIndexBytes, maxIndexSize int64 = 0, math.MinInt64
		var biggestIndex int
		for i, p := range plan {
			indexSize := int64(-1)
			for _, f := range p.Thanos.Files {
				if f.RelPath == block.IndexFilename {
					indexSize = f.SizeBytes
				}
			}

			if indexSize <= 0 {
				// Get size from bkt instead.
				attr, err := t.bw.Attributes(ctx, filepath.Join(p.ULID.String(), block.IndexFilename))
				if err != nil {
					return nil, overlappingBlocks, fmt.Errorf(
						"get attr of %v: %w",
						filepath.Join(p.ULID.String(), block.IndexFilename),
						err,
					)
				}

				indexSize = attr.Size
			}

			if maxIndexSize < indexSize {
				maxIndexSize = indexSize
				biggestIndex = i
			}
			totalIndexBytes += indexSize

			// Leave 15% headroom for index compaction bloat.
			if totalIndexBytes >= int64(float64(t.totalMaxIndexSizeBytes)*0.85) {
				// Marking blocks for no compact to limit size.
				if err := block.MarkForNoCompact(
					ctx,
					t.logger,
					t.bw,
					plan[biggestIndex].ULID,
					metadata.IndexSizeExceedingNoCompactReason,
					fmt.Sprintf(
						"largeTotalIndexSizeFilter: Total compacted block's index size could exceed: %v.",
						t.totalMaxIndexSizeBytes,
					),
					t.markedForNoCompact,
				); err != nil {
					return nil, overlappingBlocks, fmt.Errorf(
						"mark %v for no compaction: %w",
						plan[biggestIndex].ULID.String(),
						err,
					)
				}

				// Make sure wrapped planner exclude this block.
				copiedNoCompactMarked[plan[biggestIndex].ULID] = &metadata.NoCompactMark{
					ID:      plan[biggestIndex].ULID,
					Version: metadata.NoCompactMarkVersion1,
				}
				continue PlanLoop
			}
		}

		// Planned blocks should not exceed limit.
		return plan, overlappingBlocks, nil
	}
}

//
// verticalCompactionDownsampleFilter
//

var _ Planner = (*verticalCompactionDownsampleFilter)(nil)

// verticalCompactionDownsampleFilter is a planner that plans the compaction of the blocks
// without vertical compaction downsampling.
type verticalCompactionDownsampleFilter struct {
	*largeTotalIndexSizeFilter

	bw                 BlockWorker
	markedForNoCompact prometheus.Counter
}

// WithVerticalCompactionDownsampleFilter wraps Planner with verticalCompactionDownsampleFilter that plans
// the compaction of the blocks without vertical compaction downsampling.
func WithVerticalCompactionDownsampleFilter(
	with *largeTotalIndexSizeFilter,
	bw BlockWorker,
	markedForNoCompact prometheus.Counter,
) *verticalCompactionDownsampleFilter { //revive:disable-line:unexported-return // used as [Planner]
	return &verticalCompactionDownsampleFilter{
		largeTotalIndexSizeFilter: with,
		bw:                        bw,
		markedForNoCompact:        markedForNoCompact,
	}
}

// Plan is the main function that plans the compaction of the blocks without vertical compaction downsampling.
func (v *verticalCompactionDownsampleFilter) Plan(
	ctx context.Context,
	metasByMinTime []*metadata.Meta,
) ([]*metadata.Meta, bool, error) {
	noCompactMarked := make(map[ulid.ULID]*metadata.NoCompactMark, 0)

PlanLoop:
	for {
		plan, overlappingBlocks, err := v.plan(ctx, noCompactMarked, metasByMinTime)
		if err != nil {
			return nil, overlappingBlocks, err
		}

		if overlappingBlocks {
			return plan, overlappingBlocks, nil
		}

		// If we have downsampled blocks, we need to mark them as no compact because it's impossible
		// to do that with vertical compaction. Technically, the resolution is part of the group key but
		// do not attach ourselves to that level of detail.
		marked := false
		for _, m := range plan {
			if m.Thanos.Downsample.Resolution == 0 {
				continue
			}

			if err := block.MarkForNoCompact(
				ctx,
				v.logger,
				v.bw,
				m.ULID,
				metadata.DownsampleVerticalCompactionNoCompactReason,
				"verticalCompactionDownsampleFilter: Downsampled block",
				v.markedForNoCompact,
			); err != nil {
				return nil, overlappingBlocks, fmt.Errorf("mark %v for no compaction: %w", m.ULID.String(), err)
			}

			noCompactMarked[m.ULID] = &metadata.NoCompactMark{ID: m.ULID, Version: metadata.NoCompactMarkVersion1}
			marked = true
		}

		if marked {
			continue PlanLoop
		}

		return plan, overlappingBlocks, nil
	}
}
