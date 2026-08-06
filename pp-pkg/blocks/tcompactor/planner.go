package tcompactor

import (
	"context"
	"fmt"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/thanos-io/thanos/pkg/block/metadata"
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
	metas, overlappingBlocks := p.getPlan(p.noCompBlocksFunc(), metasByMinTime)
	return metas, overlappingBlocks, nil
}

// getPlan is the main function that plans the compaction of the blocks.
//
//revive:disable-next-line:cyclomatic // selecting metas for compaction is a complex task
func (p *TsdbBasedPlanner) getPlan(
	noCompactMarked map[ulid.ULID]*metadata.NoCompactMark,
	metasByMinTime []*metadata.Meta,
) ([]*metadata.Meta, bool) {
	if len(metasByMinTime) < 2 { //revive:disable-line:add-constant // check if metasByMinTime is valid
		return nil, false
	}
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
		//revive:disable-next-line:add-constant // half ranges length
		if meta.MaxTime-meta.MinTime < p.ranges[len(p.ranges)/2] {
			// If the block is entirely deleted, then we don't care about the block being big enough.
			if meta.Stats.NumTombstones > 0 && meta.Stats.NumTombstones >= meta.Stats.NumSeries {
				return []*metadata.Meta{notExcludedMetasByMinTime[i]}, false
			}
			break
		}

		//revive:disable-next-line:add-constant // calculate tombstones percentage
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

	//revive:disable-next-line:add-constant // check if metasByMinTime is valid
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
//
//revive:disable-next-line:cognitive-complexity // selecting metas for compaction is a complex task
//revive:disable-next-line:function-length // selecting metas for compaction is a complex task
//revive:disable-next-line:cyclomatic // selecting metas for compaction is a complex task
func selectMetas(
	ranges []int64,
	noCompactMarked map[ulid.ULID]*metadata.NoCompactMark,
	metasByMinTime []*metadata.Meta,
) []*metadata.Meta {
	//revive:disable-next-line:add-constant // check if ranges and metasByMinTime are valid
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

			//revive:disable-next-line:add-constant // check if part has at least 2 blocks
			if len(p) < 2 {
				continue
			}

			mint := p[0].MinTime
			maxt := p[len(p)-1].MaxTime

			// Pick the range of blocks if it spans the full range (potentially with gaps) or
			// is before the most recent block. This ensures we don't compact blocks prematurely
			// when another one of the same size still would fits in the range after upload.
			if maxt-mint != iv && maxt > highTime {
				continue
			}

			// Check if any of resulted blocks are excluded.
			// Exclude them in a way that does not introduce gaps to the system as well as preserve the ranges
			// that would be used if they were not excluded. This is meant as short-term workaround to create
			// ability for marking some blocks to not be touched for compaction.
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
