package tcompactor_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/suite"
	"github.com/thanos-io/objstore"
	"github.com/thanos-io/thanos/pkg/block/metadata"

	"github.com/prometheus/prometheus/pp-pkg/blocks/block"
	"github.com/prometheus/prometheus/pp-pkg/blocks/tcompactor"
	"github.com/prometheus/prometheus/tsdb"
)

type TsdbBasedPlannerSuite struct {
	suite.Suite

	ranges       []int64
	lPlanner     *leveledPlanner
	tPlanner     *tcompactor.TsdbBasedPlanner
	noCompBlocks *noCompactionMarkFilter
}

func TestTsdbBasedPlannerSuite(t *testing.T) {
	suite.Run(t, new(TsdbBasedPlannerSuite))
}

func (s *TsdbBasedPlannerSuite) SetupSuite() {
	s.ranges = []int64{20, 60, 180, 540, 1620}
}

func (s *TsdbBasedPlannerSuite) SetupTest() {
	lComp, err := tsdb.NewLeveledCompactor(s.T().Context(), nil, nil, s.ranges, nil, nil)
	s.Require().NoError(err)
	s.lPlanner = &leveledPlanner{dir: s.T().TempDir(), lComp: lComp}

	s.noCompBlocks = &noCompactionMarkFilter{}
	s.tPlanner, err = tcompactor.NewPlanner(log.NewNopLogger(), s.ranges, s.noCompBlocks, true)
	s.Require().NoError(err)
}

func (s *TsdbBasedPlannerSuite) TestPlanCompatibility() {
	for _, c := range []struct {
		name              string
		metas             []*metadata.Meta
		expected          []*metadata.Meta
		overlappingBlocks bool
	}{
		{
			name: "outside range",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
			},
		},
		{
			name: "two blocks not compacting",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
			},
		},
		{
			name: "three blocks not compacting",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 40, MaxTime: 60}},
			},
		},
		{
			name: "four blocks should be compacted",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 40, MaxTime: 60}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 60, MaxTime: 80}},
			},
			expected: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 40, MaxTime: 60}},
			},
		},
		{
			name: "2nd parent range",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(6, nil), MinTime: 0, MaxTime: 60}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(7, nil), MinTime: 60, MaxTime: 120}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(8, nil), MinTime: 120, MaxTime: 180}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(9, nil), MinTime: 180, MaxTime: 200}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(10, nil), MinTime: 200, MaxTime: 220}},
			},
			expected: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(6, nil), MinTime: 0, MaxTime: 60}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(7, nil), MinTime: 60, MaxTime: 120}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(8, nil), MinTime: 120, MaxTime: 180}},
			},
		},
		{
			name: "gap with size 20 not compacting",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 60, MaxTime: 80}},
			},
		},
		{
			name: "gap with size 20 between second and third block",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 60, MaxTime: 80}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(5, nil), MinTime: 80, MaxTime: 100}},
			},
			expected: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
			},
		},
		{
			name: "20 20 20 60 60 range blocks  5 is marked as fresh one",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 40, MaxTime: 60}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 60, MaxTime: 120}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(5, nil), MinTime: 120, MaxTime: 180}},
			},
			expected: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 40, MaxTime: 60}},
			},
		},
		{
			name: "blocks to fill the entire 2nd parent range but there is a gap",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(6, nil), MinTime: 0, MaxTime: 60}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(8, nil), MinTime: 120, MaxTime: 180}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(9, nil), MinTime: 180, MaxTime: 200}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(10, nil), MinTime: 200, MaxTime: 220}},
			},
			expected: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(6, nil), MinTime: 0, MaxTime: 60}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(8, nil), MinTime: 120, MaxTime: 180}},
			},
		},
		{
			name: "20 60 20 60 240 range blocks  compact 20 60 60",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 60, MaxTime: 120}},
				// Fresh one.
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(5, nil), MinTime: 960, MaxTime: 980}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(6, nil), MinTime: 120, MaxTime: 180}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(7, nil), MinTime: 720, MaxTime: 960}},
			},
			expected: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 60, MaxTime: 120}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(6, nil), MinTime: 120, MaxTime: 180}},
			},
		},
		{
			name: "not select large blocks that have many tombstones when there is no fresh block",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{
					Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 540,
					Stats: tsdb.BlockStats{
						NumSeries:     10,
						NumTombstones: 3,
					},
				}},
			},
		},
		{
			name: "select large blocks that have many tombstones when fresh appears",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{
					Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 540,
					Stats: tsdb.BlockStats{
						NumSeries:     10,
						NumTombstones: 3,
					},
				}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 540, MaxTime: 560}},
			},
			expected: []*metadata.Meta{{BlockMeta: tsdb.BlockMeta{
				Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 540,
				Stats: tsdb.BlockStats{
					NumSeries:     10,
					NumTombstones: 3,
				},
			}}},
		},
		{
			name: "for small blocks do not compact tombstones even when fresh appears",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{
					Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 60,
					Stats: tsdb.BlockStats{
						NumSeries:     10,
						NumTombstones: 3,
					},
				}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 60, MaxTime: 80}},
			},
		},
		{
			name: "regression test  we were stuck in a compact loop where we always recompacted" +
				"the same block when tombstones and series counts were zero",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{
					Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 540,
					Stats: tsdb.BlockStats{
						NumSeries:     0,
						NumTombstones: 0,
					},
				}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 540, MaxTime: 560}},
			},
		},
		{
			name: "regression test  we were wrongly assuming that new block is fresh from WAL when its ULID is" +
				" newest  need to actually look on max time instead with previous wrong approach 8 block was ignored" +
				" so we were wrongly compacting 5 and 7 and introducing block overlaps",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(5, nil), MinTime: 0, MaxTime: 360}},
				// Fresh one.
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(6, nil), MinTime: 540, MaxTime: 560}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(7, nil), MinTime: 360, MaxTime: 420}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(8, nil), MinTime: 420, MaxTime: 540}},
			},
			expected: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(7, nil), MinTime: 360, MaxTime: 420}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(8, nil), MinTime: 420, MaxTime: 540}},
			},
		},
		// |--------------|
		//               |----------------|
		//                                |--------------|
		{
			name: "overlapping blocks 1",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 19, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 40, MaxTime: 60}},
			},
			expected: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 19, MaxTime: 40}},
			},
			overlappingBlocks: true,
		},
		// |--------------|
		//                |--------------|
		//                        |--------------|
		{
			name: "overlapping blocks 2",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 30, MaxTime: 50}},
			},
			expected: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 30, MaxTime: 50}},
			},
			overlappingBlocks: true,
		},
		// |--------------|
		//         |---------------------|
		//                       |--------------|
		{
			name: "overlapping blocks 3",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 10, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 30, MaxTime: 50}},
			},
			expected: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 10, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 30, MaxTime: 50}},
			},
			overlappingBlocks: true,
		},
		// |--------------|
		//               |--------------------------------|
		//                |--------------|
		//                               |--------------|
		{
			name: "overlapping blocks 4",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(5, nil), MinTime: 0, MaxTime: 360}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(6, nil), MinTime: 340, MaxTime: 560}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(7, nil), MinTime: 360, MaxTime: 420}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(8, nil), MinTime: 420, MaxTime: 540}},
			},
			expected: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(5, nil), MinTime: 0, MaxTime: 360}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(6, nil), MinTime: 340, MaxTime: 560}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(7, nil), MinTime: 360, MaxTime: 420}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(8, nil), MinTime: 420, MaxTime: 540}},
			},
			overlappingBlocks: true,
		},
		// |--------------|
		//               |--------------|
		//                                            |--------------|
		//                                                          |--------------|
		{
			name: "overlapping blocks 5",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 10}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 9, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 30, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 39, MaxTime: 50}},
			},
			expected: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 10}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 9, MaxTime: 20}},
			},
			overlappingBlocks: true,
		},
	} {
		for _, e := range c.expected {
			// Add here to avoid boilerplate.
			e.Thanos.Labels = make(map[string]string)
		}
		for _, e := range c.metas {
			// Add here to avoid boilerplate.
			e.Thanos.Labels = make(map[string]string)
		}

		s.Run(c.name, func() {
			metasByMinTime := make([]*metadata.Meta, len(c.metas))
			for i := range metasByMinTime {
				metasByMinTime[i] = c.metas[i]
			}
			sort.Slice(metasByMinTime, func(i, j int) bool {
				return metasByMinTime[i].MinTime < metasByMinTime[j].MinTime
			})

			lPlan, err := s.lPlanner.Plan(metasByMinTime)
			s.Require().NoError(err)
			s.Equal(c.expected, lPlan)

			tPlan, overlappingBlocks, err := s.tPlanner.Plan(s.T().Context(), metasByMinTime)
			s.Require().NoError(err)
			s.Equal(c.overlappingBlocks, overlappingBlocks)
			s.Equal(c.expected, tPlan)
		})
	}
}

func (s *TsdbBasedPlannerSuite) TestRangeWithFailedCompactionWontGetSelected() {
	for _, c := range []struct {
		metas []*metadata.Meta
	}{
		{
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 40, MaxTime: 60}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 60, MaxTime: 80}},
			},
		},
		{
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 60, MaxTime: 80}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(5, nil), MinTime: 80, MaxTime: 100}},
			},
		},
		{
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 40, MaxTime: 60}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 60, MaxTime: 120}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(5, nil), MinTime: 120, MaxTime: 180}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(6, nil), MinTime: 180, MaxTime: 200}},
			},
		},
	} {
		s.Run("", func() {
			c.metas[1].Compaction.Failed = true
			// For compatibility.
			lPlan, err := s.lPlanner.Plan(c.metas)
			s.Require().NoError(err)
			s.Equal([]*metadata.Meta(nil), lPlan)

			tPlan, _, err := s.tPlanner.Plan(s.T().Context(), c.metas)
			s.Require().NoError(err)
			s.Equal([]*metadata.Meta(nil), tPlan)
		})
	}
}

func (s *TsdbBasedPlannerSuite) TestTSDBBasedPlannerPlanWithNoCompactMarks() {
	for _, c := range []struct {
		name              string
		metas             []*metadata.Meta
		noCompactMarks    map[ulid.ULID]*metadata.NoCompactMark
		expected          []*metadata.Meta
		overlappingBlocks bool
	}{
		{
			name: "outside range and excluded",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
			},
			noCompactMarks: map[ulid.ULID]*metadata.NoCompactMark{
				ulid.MustNew(1, nil): {},
			},
		},
		{
			name: "blocks to fill the entire parent but with first one excluded",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 40, MaxTime: 60}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 60, MaxTime: 80}},
			},
			noCompactMarks: map[ulid.ULID]*metadata.NoCompactMark{
				ulid.MustNew(1, nil): {},
			},
			expected: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 40, MaxTime: 60}},
			},
		},
		{
			name: "blocks to fill the entire parent but with second one excluded",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 40, MaxTime: 60}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 60, MaxTime: 80}},
			},
			noCompactMarks: map[ulid.ULID]*metadata.NoCompactMark{
				ulid.MustNew(2, nil): {},
			},
		},
		{
			name: "blocks to fill the entire parent but with last one excluded",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 40, MaxTime: 60}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 60, MaxTime: 80}},
			},
			noCompactMarks: map[ulid.ULID]*metadata.NoCompactMark{
				ulid.MustNew(4, nil): {},
			},
			expected: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 40, MaxTime: 60}},
			},
		},
		{
			name: "blocks to fill the entire parent but with last one fist excluded",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 40, MaxTime: 60}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 60, MaxTime: 80}},
			},
			noCompactMarks: map[ulid.ULID]*metadata.NoCompactMark{
				ulid.MustNew(1, nil): {},
				ulid.MustNew(4, nil): {},
			},
			expected: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 40, MaxTime: 60}},
			},
		},
		{
			name: "blocks to fill the entire parent but with all of them excluded",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 40, MaxTime: 60}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 60, MaxTime: 80}},
			},
			noCompactMarks: map[ulid.ULID]*metadata.NoCompactMark{
				ulid.MustNew(1, nil): {},
				ulid.MustNew(2, nil): {},
				ulid.MustNew(3, nil): {},
				ulid.MustNew(4, nil): {},
			},
		},
		{
			name: "block for the next parent range appeared and we have a gap with size 20 between" +
				"second and third block second block is excluded",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 60, MaxTime: 80}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(5, nil), MinTime: 80, MaxTime: 100}},
			},
			noCompactMarks: map[ulid.ULID]*metadata.NoCompactMark{
				ulid.MustNew(2, nil): {},
			},
		},
		{
			name: "20 60 20 60 240 range blocks  compact 20 60 60 but sixth 6th is excluded",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 60, MaxTime: 120}},
				// Fresh one.
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(5, nil), MinTime: 960, MaxTime: 980}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(6, nil), MinTime: 120, MaxTime: 180}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(7, nil), MinTime: 720, MaxTime: 960}},
			},
			noCompactMarks: map[ulid.ULID]*metadata.NoCompactMark{
				ulid.MustNew(6, nil): {},
			},
			expected: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 60, MaxTime: 120}},
			},
		},
		{
			name: "20 60 20 60 240 range blocks  compact 20 60 60 but 4th is excluded",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 60, MaxTime: 120}},
				// Fresh one.
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(5, nil), MinTime: 960, MaxTime: 980}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(6, nil), MinTime: 120, MaxTime: 180}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(7, nil), MinTime: 720, MaxTime: 960}},
			},
			noCompactMarks: map[ulid.ULID]*metadata.NoCompactMark{
				ulid.MustNew(4, nil): {},
			},
		},
		{
			name: "do not select large blocks that have many tombstones when fresh appears but are excluded",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{
					Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 540,
					Stats: tsdb.BlockStats{
						NumSeries:     10,
						NumTombstones: 3,
					},
				}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 540, MaxTime: 560}},
			},
			noCompactMarks: map[ulid.ULID]*metadata.NoCompactMark{
				ulid.MustNew(1, nil): {},
			},
		},
		// |--------------|
		//               |----------------|
		//                                |--------------|
		{
			name: "overlapping blocks 1 but one is excluded",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 19, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 40, MaxTime: 60}},
			},
			noCompactMarks: map[ulid.ULID]*metadata.NoCompactMark{
				ulid.MustNew(1, nil): {},
			},
		},
	} {
		s.Run(c.name, func() {
			metasByMinTime := make([]*metadata.Meta, len(c.metas))
			for i := range metasByMinTime {
				metasByMinTime[i] = c.metas[i]
			}

			sort.Slice(metasByMinTime, func(i, j int) bool {
				return metasByMinTime[i].MinTime < metasByMinTime[j].MinTime
			})

			s.noCompBlocks.noCompBlocks = c.noCompactMarks
			plan, overlappingBlocks, err := s.tPlanner.Plan(s.T().Context(), metasByMinTime)
			s.Require().NoError(err)
			s.Equal(c.overlappingBlocks, overlappingBlocks)
			s.Equal(c.expected, plan)
		})
	}
}

func (s *TsdbBasedPlannerSuite) TestPlanCompatibilityDisabledOverlappingCompactionDisabled() {
	lComp, err := tsdb.NewLeveledCompactorWithOptions(
		s.T().Context(),
		nil,
		nil,
		s.ranges,
		nil,
		tsdb.LeveledCompactorOptions{EnableOverlappingCompaction: false},
	)
	s.Require().NoError(err)
	lPlanner := &leveledPlanner{dir: s.T().TempDir(), lComp: lComp}

	tPlanner, err := tcompactor.NewPlanner(log.NewNopLogger(), s.ranges, s.noCompBlocks, false)
	s.Require().NoError(err)

	for _, c := range []struct {
		name     string
		metas    []*metadata.Meta
		expected []*metadata.Meta
	}{
		// |--------------|
		//               |----------------|
		//                                |--------------|
		{
			name: "overlapping blocks 1",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 19, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 40, MaxTime: 60}},
			},
		},
		// |--------------|
		//                |--------------|
		//                        |--------------|
		{
			name: "overlapping blocks 2",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 30, MaxTime: 50}},
			},
		},
		// |--------------|
		//         |---------------------|
		//                       |--------------|
		{
			name: "overlapping blocks 3",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 10, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 30, MaxTime: 50}},
			},
		},
		// |--------------|
		//               |--------------------------------|
		//                |--------------|
		//                               |--------------|
		{
			name: "overlapping blocks 4",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(5, nil), MinTime: 0, MaxTime: 360}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(6, nil), MinTime: 340, MaxTime: 560}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(7, nil), MinTime: 360, MaxTime: 420}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(8, nil), MinTime: 420, MaxTime: 540}},
			},
		},
		// |--------------|
		//               |--------------|
		//                                            |--------------|
		//                                                          |--------------|
		{
			name: "overlapping blocks 5",
			metas: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 10}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 9, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 30, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 39, MaxTime: 50}},
			},
		},
	} {
		for _, e := range c.expected {
			// Add here to avoid boilerplate.
			e.Thanos.Labels = make(map[string]string)
		}
		for _, e := range c.metas {
			// Add here to avoid boilerplate.
			e.Thanos.Labels = make(map[string]string)
		}

		s.Run(c.name, func() {
			metasByMinTime := make([]*metadata.Meta, len(c.metas))
			for i := range metasByMinTime {
				metasByMinTime[i] = c.metas[i]
			}
			sort.Slice(metasByMinTime, func(i, j int) bool {
				return metasByMinTime[i].MinTime < metasByMinTime[j].MinTime
			})

			lPlan, err := lPlanner.Plan(metasByMinTime)
			s.Require().NoError(err)
			s.Equal(c.expected, lPlan)

			tPlan, _, err := tPlanner.Plan(s.T().Context(), metasByMinTime)
			s.Require().NoError(err)
			s.Equal(c.expected, tPlan)
		})
	}
}

//revive:disable-next-line:cognitive-complexity // this is test
func (s *TsdbBasedPlannerSuite) TestLargeTotalIndexSizeFilterPlan() {
	bkt := objstore.NewInMemBucket()
	marked := promauto.With(nil).NewCounter(prometheus.CounterOpts{})

	planner := tcompactor.WithLargeTotalIndexSizeFilter(s.tPlanner, bkt, 100, marked)
	var lastMarkValue float64

	for _, c := range []struct {
		name  string
		metas []*metadata.Meta

		expected      []*metadata.Meta
		expectedMarks float64
	}{
		{
			name: "Outside range and excluded",
			metas: []*metadata.Meta{
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 100}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20},
				},
			},
			expectedMarks: 0,
		},
		{
			name: "Blocks to fill the entire parent but with first one too large",
			metas: []*metadata.Meta{
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 41}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20},
				},
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 30}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40},
				},
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 30}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 40, MaxTime: 60},
				},
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 30}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 60, MaxTime: 80},
				},
			},
			expectedMarks: 1,
			expected: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 40, MaxTime: 60}},
			},
		},
		{
			name: "Blocks to fill the entire parent but with second one too large",
			metas: []*metadata.Meta{
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 30}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20},
				},
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 41}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40},
				},
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 30}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 40, MaxTime: 60},
				},
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 20}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 60, MaxTime: 80},
				},
			},
			expectedMarks: 1,
		},
		{
			name: "Blocks to fill the entire parent but with last size exceeded",
			metas: []*metadata.Meta{
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 10}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20},
				},
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 10}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40},
				},
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 10}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 40, MaxTime: 60},
				},
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 90}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 60, MaxTime: 80},
				},
			},
			expected: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 40, MaxTime: 60}},
			},
		},
		{
			name: "Blocks to fill the entire parent but with pre-last one and first too large",
			metas: []*metadata.Meta{
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 90}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20},
				},
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 30}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40},
				},
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 30}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 40, MaxTime: 50},
				},
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 90}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 50, MaxTime: 60},
				},
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 90}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(5, nil), MinTime: 60, MaxTime: 80},
				},
			},
			expected: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 40, MaxTime: 50}},
			},
			expectedMarks: 2,
		},
		{
			name: "Block for the next parent range appeared and we have a gap with size 20 between second and" +
				" third block but second block is excluded",
			metas: []*metadata.Meta{
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 30}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20},
				},
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 90}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40},
				},
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 30}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 60, MaxTime: 80},
				},
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 30}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(5, nil), MinTime: 80, MaxTime: 100},
				},
			},
			expectedMarks: 1,
		},
		{
			name: "We have 20 60 20 60 240 range blocks  compact 20 60 60 but sixth 6th is excluded",
			metas: []*metadata.Meta{
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 30}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40},
				},
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 30}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 60, MaxTime: 120},
				},
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 30}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(5, nil), MinTime: 960, MaxTime: 980},
				}, // Fresh one.
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 90}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(6, nil), MinTime: 120, MaxTime: 180},
				},
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 30}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(7, nil), MinTime: 720, MaxTime: 960},
				},
			},
			expected: []*metadata.Meta{
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
				{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 60, MaxTime: 120}},
			},
			expectedMarks: 1,
		},
		// |--------------|
		//               |----------------|
		//                                |--------------|
		{
			name: "Overlapping blocks 1 but total is too large",
			metas: []*metadata.Meta{
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 90}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20},
				},
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 30}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 19, MaxTime: 40},
				},
				{
					Thanos:    metadata.Thanos{Files: []metadata.File{{RelPath: block.IndexFilename, SizeBytes: 30}}},
					BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 40, MaxTime: 60},
				},
			},
			expectedMarks: 1,
		},
	} {
		if !s.Run(c.name, func() {
			s.Run("from meta", func() {
				obj := bkt.Objects()
				for o := range obj {
					s.Require().NoError(bkt.Delete(s.T().Context(), o))
				}

				metasByMinTime := make([]*metadata.Meta, len(c.metas))
				for i := range metasByMinTime {
					orig := c.metas[i]
					m := &metadata.Meta{}
					*m = *orig
					metasByMinTime[i] = m
				}
				sort.Slice(metasByMinTime, func(i, j int) bool {
					return metasByMinTime[i].MinTime < metasByMinTime[j].MinTime
				})

				plan, _, err := planner.Plan(s.T().Context(), metasByMinTime)
				s.Require().NoError(err)

				for _, m := range plan {
					// For less boilerplate.
					m.Thanos = metadata.Thanos{}
				}

				s.Equal(c.expected, plan)
				s.Equal(c.expectedMarks, testutil.ToFloat64(marked)-lastMarkValue)
				lastMarkValue = testutil.ToFloat64(marked)
			})

			s.Run("from bkt", func() {
				obj := bkt.Objects()
				for o := range obj {
					s.Require().NoError(bkt.Delete(context.Background(), o))
				}

				metasByMinTime := make([]*metadata.Meta, len(c.metas))
				for i := range metasByMinTime {
					orig := c.metas[i]
					m := &metadata.Meta{}
					*m = *orig
					metasByMinTime[i] = m
				}
				sort.Slice(metasByMinTime, func(i, j int) bool {
					return metasByMinTime[i].MinTime < metasByMinTime[j].MinTime
				})

				for _, m := range metasByMinTime {
					s.Require().NoError(bkt.Upload(
						s.T().Context(),
						filepath.Join(m.ULID.String(), block.IndexFilename),
						bytes.NewReader(make([]byte, m.Thanos.Files[0].SizeBytes))),
					)
					m.Thanos = metadata.Thanos{}
				}

				plan, _, err := planner.Plan(context.Background(), metasByMinTime)
				s.Require().NoError(err)
				s.Equal(c.expected, plan)
				s.Equal(c.expectedMarks, testutil.ToFloat64(marked)-lastMarkValue)

				lastMarkValue = testutil.ToFloat64(marked)
			})
		}) {
			return
		}
	}
}

//
// leveledPlanner
//

// leveledPlanner is an adapter for the tsdb.Compactor interface.
type leveledPlanner struct {
	dir   string
	lComp *tsdb.LeveledCompactor
}

// Plan implements the tsdb.Compactor interface.
func (p *leveledPlanner) Plan(metasByMinTime []*metadata.Meta) ([]*metadata.Meta, error) {
	// TSDB planning works based on the meta.json files in the given dir. Mock it up.
	bdirs := make([]string, 0, len(metasByMinTime))
	for _, meta := range metasByMinTime {
		bdir := filepath.Join(p.dir, meta.ULID.String())
		if err := os.MkdirAll(bdir, 0o777); err != nil {
			return nil, fmt.Errorf("create planning block dir: %w", err)
		}

		if err := meta.WriteToDir(log.NewNopLogger(), bdir); err != nil {
			return nil, fmt.Errorf("write planning meta file: %w", err)
		}

		bdirs = append(bdirs, bdir)
	}

	plan, err := p.lComp.Plan(p.dir)
	if err != nil {
		return nil, err
	}

	var res []*metadata.Meta
	for _, pdir := range plan {
		meta, err := metadata.ReadFromDir(pdir)
		if err != nil {
			return nil, fmt.Errorf("read meta from %s: %w", pdir, err)
		}

		res = append(res, meta)
	}

	for _, bdir := range bdirs {
		if err := os.RemoveAll(bdir); err != nil {
			return nil, fmt.Errorf("remove planning block dir: %w", err)
		}
	}

	return res, nil
}

//
// noCompactionMarkFilter
//

// noCompactionMarkFilter is a filter that returns block ids that were marked for no compaction.
type noCompactionMarkFilter struct {
	noCompBlocks map[ulid.ULID]*metadata.NoCompactMark
}

// NoCompactMarkedBlocks implementation of [NoCompactionMarkFilter],
// returns the block ids that were marked for no compaction.
func (n *noCompactionMarkFilter) NoCompactMarkedBlocks() map[ulid.ULID]*metadata.NoCompactMark {
	return n.noCompBlocks
}
