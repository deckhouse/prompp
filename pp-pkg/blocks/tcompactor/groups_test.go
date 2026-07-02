package tcompactor_test

import (
	"context"
	"testing"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/stretchr/testify/suite"
	"github.com/thanos-io/thanos/pkg/block/metadata"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp-pkg/blocks/block"
	"github.com/prometheus/prometheus/pp-pkg/blocks/lcompactor"
	"github.com/prometheus/prometheus/pp-pkg/blocks/tcompactor"
	"github.com/prometheus/prometheus/pp-pkg/blocks/tcompactor/mock"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

type GrouperSuite struct {
	suite.Suite
}

func TestGrouperSuite(t *testing.T) {
	suite.Run(t, new(GrouperSuite))
}

func (s *GrouperSuite) TestOneGroup() {
	blks := []*block.Block{{}, {}}
	ls := map[string]string{"foo": "bar"}
	blks[0].Metadata().Thanos.Labels = ls
	blks[1].Metadata().Thanos.Labels = ls

	groups, err := tcompactor.NewDefaultGrouper(log.NewNopLogger(), nil, false).Groups(blks)
	s.Require().NoError(err)
	s.Len(groups, 1)
}

func (s *GrouperSuite) TestTwoGroupsByLabels() {
	blks := []*block.Block{{}, {}}
	ls1 := map[string]string{"foo": "bar"}
	ls2 := map[string]string{"foo": "baz"}
	blks[0].Metadata().Thanos.Labels = ls1
	blks[1].Metadata().Thanos.Labels = ls2

	groups, err := tcompactor.NewDefaultGrouper(log.NewNopLogger(), nil, false).Groups(blks)
	s.Require().NoError(err)
	s.Len(groups, 2)
}

func (s *GrouperSuite) TestTwoGroupsByResolution() {
	blks := []*block.Block{{}, {}}
	ls := map[string]string{"foo": "bar"}
	blks[0].Metadata().Thanos.Labels = ls
	blks[1].Metadata().Thanos.Labels = ls
	blks[0].Metadata().Thanos.Downsample.Resolution = 10
	blks[1].Metadata().Thanos.Downsample.Resolution = 20

	groups, err := tcompactor.NewDefaultGrouper(log.NewNopLogger(), nil, false).Groups(blks)
	s.Require().NoError(err)
	s.Len(groups, 2)
}

//
// GroupCompactSuite
//

type GroupCompactSuite struct {
	suite.Suite
}

func TestGroupCompactSuite(t *testing.T) {
	suite.Run(t, new(GroupCompactSuite))
}

func (s *GroupCompactSuite) TestHappyPath() {
	noopCounter := promauto.With(nil).NewCounter(prometheus.CounterOpts{Name: "noop", Help: "noop"})
	ls := map[string]string{"foo": "bar"}

	blks := []*block.Block{{}, {}}
	blks[0].Metadata().Thanos.Labels = ls
	blks[1].Metadata().Thanos.Labels = ls

	group := tcompactor.NewGroup(
		log.NewNopLogger(),
		s.T().Name(),
		labels.FromMap(ls),
		0,
		noopCounter, noopCounter, noopCounter, noopCounter, noopCounter,
		false,
	)

	for i := range blks {
		s.Require().NoError(group.AppendMeta(blks[i].Metadata()))
	}

	planner := &mock.PlannerMock{
		PlanFunc: func(_ context.Context, metasByMinTime []*metadata.Meta) ([]*metadata.Meta, bool, error) {
			s.Len(metasByMinTime, len(blks))
			return metasByMinTime, false, nil
		},
	}

	compactor := &mock.CompactorMock{
		CompactWithBlockPopulatorWithWriteMetaFileFunc: func(
			_ string,
			dirs []string,
			open []*block.Block,
			populator lcompactor.BlockPopulator,
			_ func(logger log.Logger, dir string, meta *tsdb.BlockMeta) (int64, error),
		) ([]ulid.ULID, error) {
			s.Len(dirs, len(blks))
			s.Len(open, len(blks))
			brs := make([]tsdb.BlockReader, len(open))
			for i := range open {
				brs[i] = open[i]
			}

			s.Require().NoError(populator.PopulateBlock(s.T().Context(), nil, nil, nil, nil, brs, nil, nil, nil, nil))
			return []ulid.ULID{ulid.MustNew(1, nil)}, nil
		},
	}

	blockPopulator := &mock.BlockPopulatorMock{
		PopulateBlockFunc: func(
			_ context.Context,
			_ *lcompactor.CompactorMetrics,
			_ log.Logger,
			_ chunkenc.Pool,
			_ storage.VerticalChunkSeriesMergeFunc,
			blocks []tsdb.BlockReader,
			_ *tsdb.BlockMeta,
			_ tsdb.IndexWriter,
			_ tsdb.ChunkWriter,
			_ tsdb.IndexReaderPostingsFunc,
		) error {
			s.Len(blocks, len(blks))
			return nil
		},
	}

	ulids, err := group.Compact(s.T().Context(), "test-dir", planner, compactor, blockPopulator, blks)
	s.Require().NoError(err)
	s.Len(ulids, 1)
	s.Len(planner.PlanCalls(), 1)
	s.Len(compactor.CompactWithBlockPopulatorWithWriteMetaFileCalls(), 1)
	s.Len(blockPopulator.PopulateBlockCalls(), 1)
}

func (s *GroupCompactSuite) TestNoPlan() {
	noopCounter := promauto.With(nil).NewCounter(prometheus.CounterOpts{Name: "noop", Help: "noop"})
	ls := map[string]string{"foo": "bar"}

	blks := []*block.Block{{}, {}}
	blks[0].Metadata().Thanos.Labels = ls
	blks[1].Metadata().Thanos.Labels = ls

	group := tcompactor.NewGroup(
		log.NewNopLogger(),
		s.T().Name(),
		labels.FromMap(ls),
		0,
		noopCounter, noopCounter, noopCounter, noopCounter, noopCounter,
		false,
	)

	for i := range blks {
		s.Require().NoError(group.AppendMeta(blks[i].Metadata()))
	}

	planner := &mock.PlannerMock{
		PlanFunc: func(_ context.Context, metasByMinTime []*metadata.Meta) ([]*metadata.Meta, bool, error) {
			s.Len(metasByMinTime, len(blks))
			return nil, false, nil
		},
	}

	compactor := &mock.CompactorMock{
		CompactWithBlockPopulatorWithWriteMetaFileFunc: func(
			_ string,
			dirs []string,
			open []*block.Block,
			_ lcompactor.BlockPopulator,
			_ func(logger log.Logger, dir string, meta *tsdb.BlockMeta) (int64, error),
		) ([]ulid.ULID, error) {
			s.Len(dirs, len(blks))
			s.Len(open, len(blks))
			return nil, nil
		},
	}

	ulids, err := group.Compact(s.T().Context(), "test-dir", planner, compactor, nil, blks)
	s.Require().NoError(err)
	s.Empty(ulids)
	s.Len(planner.PlanCalls(), 1)
	s.Empty(compactor.CompactWithBlockPopulatorWithWriteMetaFileCalls())
}

func (s *GroupCompactSuite) TestNoCompact() {
	noopCounter := promauto.With(nil).NewCounter(prometheus.CounterOpts{Name: "noop", Help: "noop"})
	ls := map[string]string{"foo": "bar"}

	blks := []*block.Block{{}, {}}
	blks[0].Metadata().Thanos.Labels = ls
	blks[1].Metadata().Thanos.Labels = ls

	group := tcompactor.NewGroup(
		log.NewNopLogger(),
		s.T().Name(),
		labels.FromMap(ls),
		0,
		noopCounter, noopCounter, noopCounter, noopCounter, noopCounter,
		false,
	)

	for i := range blks {
		s.Require().NoError(group.AppendMeta(blks[i].Metadata()))
	}

	planner := &mock.PlannerMock{
		PlanFunc: func(_ context.Context, metasByMinTime []*metadata.Meta) ([]*metadata.Meta, bool, error) {
			s.Len(metasByMinTime, len(blks))
			return metasByMinTime, false, nil
		},
	}

	compactor := &mock.CompactorMock{
		CompactWithBlockPopulatorWithWriteMetaFileFunc: func(
			_ string,
			dirs []string,
			open []*block.Block,
			_ lcompactor.BlockPopulator,
			_ func(logger log.Logger, dir string, meta *tsdb.BlockMeta) (int64, error),
		) ([]ulid.ULID, error) {
			s.Len(dirs, len(blks))
			s.Len(open, len(blks))
			return nil, nil
		},
	}

	ulids, err := group.Compact(s.T().Context(), "test-dir", planner, compactor, nil, blks)
	s.Require().NoError(err)
	s.Empty(ulids)
	s.Len(planner.PlanCalls(), 1)
	s.Len(compactor.CompactWithBlockPopulatorWithWriteMetaFileCalls(), 1)
}

//
// GroupOverlappingBlocksSuite
//

type GroupOverlappingBlocksSuite struct {
	suite.Suite

	group *tcompactor.Group
	ls    map[string]string
}

func TestGroupOverlappingBlocksSuite(t *testing.T) {
	suite.Run(t, new(GroupOverlappingBlocksSuite))
}

func (s *GroupOverlappingBlocksSuite) SetupTest() {
	noopCounter := promauto.With(nil).NewCounter(prometheus.CounterOpts{Name: "noop", Help: "noop"})
	s.ls = map[string]string{"foo": "bar"}

	s.group = tcompactor.NewGroup(
		log.NewNopLogger(),
		s.T().Name(),
		labels.FromMap(s.ls),
		0,
		noopCounter, noopCounter, noopCounter, noopCounter, noopCounter,
		false,
	)
	for i := 10; i >= 0; i-- {
		s.Require().NoError(s.group.AppendMeta(&metadata.Meta{
			Thanos:    metadata.Thanos{Labels: s.ls},
			BlockMeta: tsdb.BlockMeta{MinTime: int64(i * 10), MaxTime: int64((i + 1) * 10)},
		}))
	}
}

func (s *GroupOverlappingBlocksSuite) TestOverlappingBlocksEmpty() {
	overlaps := block.Overlaps{}
	s.group.OverlappingBlocks(overlaps)
	s.Empty(overlaps)
}

func (s *GroupOverlappingBlocksSuite) TestOverlappingBlocks_10_20() {
	expBM := tsdb.BlockMeta{MinTime: 15, MaxTime: 17}
	expOverlaps := block.Overlaps{
		{Min: 15, Max: 17, Key: s.group.String()}: {{MinTime: 10, MaxTime: 20}, expBM},
	}

	s.Require().NoError(s.group.AppendMeta(&metadata.Meta{
		Thanos:    metadata.Thanos{Labels: s.ls},
		BlockMeta: expBM,
	}))

	overlaps := block.Overlaps{}
	s.group.OverlappingBlocks(overlaps)
	s.Equal(expOverlaps, overlaps)
}

func (s *GroupOverlappingBlocksSuite) TestOverlappingBlocks_20_30_and_30_40() {
	expBM := tsdb.BlockMeta{MinTime: 21, MaxTime: 31}
	expOverlaps := block.Overlaps{
		{Min: 21, Max: 30, Key: s.group.String()}: {{MinTime: 20, MaxTime: 30}, expBM},
		{Min: 30, Max: 31, Key: s.group.String()}: {expBM, {MinTime: 30, MaxTime: 40}},
	}

	s.Require().NoError(s.group.AppendMeta(&metadata.Meta{
		Thanos:    metadata.Thanos{Labels: s.ls},
		BlockMeta: expBM,
	}))

	overlaps := block.Overlaps{}
	s.group.OverlappingBlocks(overlaps)
	s.Equal(expOverlaps, overlaps)
}

func (s *GroupOverlappingBlocksSuite) TestOverlappingBlocks_30_40() {
	expBM1 := tsdb.BlockMeta{MinTime: 33, MaxTime: 39}
	expBM2 := tsdb.BlockMeta{MinTime: 34, MaxTime: 36}
	expOverlaps := block.Overlaps{
		{Min: 34, Max: 36, Key: s.group.String()}: {{MinTime: 30, MaxTime: 40}, expBM1, expBM2},
	}

	s.Require().NoError(s.group.AppendMeta(&metadata.Meta{
		Thanos:    metadata.Thanos{Labels: s.ls},
		BlockMeta: expBM1,
	}))

	s.Require().NoError(s.group.AppendMeta(&metadata.Meta{
		Thanos:    metadata.Thanos{Labels: s.ls},
		BlockMeta: expBM2,
	}))

	overlaps := block.Overlaps{}
	s.group.OverlappingBlocks(overlaps)
	s.Equal(expOverlaps, overlaps)
}

func (s *GroupOverlappingBlocksSuite) TestOverlappingBlocks_50_60() {
	expBM := tsdb.BlockMeta{MinTime: 50, MaxTime: 60}
	expOverlaps := block.Overlaps{
		{Min: 50, Max: 60, Key: s.group.String()}: {{MinTime: 50, MaxTime: 60}, expBM},
	}

	s.Require().NoError(s.group.AppendMeta(&metadata.Meta{
		Thanos:    metadata.Thanos{Labels: s.ls},
		BlockMeta: expBM,
	}))

	overlaps := block.Overlaps{}
	s.group.OverlappingBlocks(overlaps)
	s.Equal(expOverlaps, overlaps)
}

func (s *GroupOverlappingBlocksSuite) TestOverlappingBlocks_60_70_and_70_80_and_80_90() {
	expBM := tsdb.BlockMeta{MinTime: 61, MaxTime: 85}
	expOverlaps := block.Overlaps{
		{Min: 61, Max: 70, Key: s.group.String()}: {{MinTime: 60, MaxTime: 70}, expBM},
		{Min: 70, Max: 80, Key: s.group.String()}: {expBM, {MinTime: 70, MaxTime: 80}},
		{Min: 80, Max: 85, Key: s.group.String()}: {expBM, {MinTime: 80, MaxTime: 90}},
	}

	s.Require().NoError(s.group.AppendMeta(&metadata.Meta{
		Thanos:    metadata.Thanos{Labels: s.ls},
		BlockMeta: expBM,
	}))

	overlaps := block.Overlaps{}
	s.group.OverlappingBlocks(overlaps)
	s.Equal(expOverlaps, overlaps)
}

func (s *GroupOverlappingBlocksSuite) TestOverlappingBlocks_90_100_and_100_110__90_100() {
	expBM1 := tsdb.BlockMeta{MinTime: 92, MaxTime: 105}
	expBM2 := tsdb.BlockMeta{MinTime: 94, MaxTime: 99}
	expOverlaps := block.Overlaps{
		{Min: 94, Max: 99, Key: s.group.String()}:   {{MinTime: 90, MaxTime: 100}, expBM1, expBM2},
		{Min: 100, Max: 105, Key: s.group.String()}: {expBM1, {MinTime: 100, MaxTime: 110}},
	}

	s.Require().NoError(s.group.AppendMeta(&metadata.Meta{
		Thanos:    metadata.Thanos{Labels: s.ls},
		BlockMeta: expBM1,
	}))

	s.Require().NoError(s.group.AppendMeta(&metadata.Meta{
		Thanos:    metadata.Thanos{Labels: s.ls},
		BlockMeta: expBM2,
	}))

	overlaps := block.Overlaps{}
	s.group.OverlappingBlocks(overlaps)
	s.Equal(expOverlaps, overlaps)
}

func (s *GroupOverlappingBlocksSuite) TestOverlappingBlocks_all_together() {
	expBMs := []tsdb.BlockMeta{
		{MinTime: 15, MaxTime: 17},
		{MinTime: 21, MaxTime: 31},
		{MinTime: 33, MaxTime: 39},
		{MinTime: 34, MaxTime: 36},
		{MinTime: 50, MaxTime: 60},
		{MinTime: 61, MaxTime: 85},
		{MinTime: 92, MaxTime: 105},
		{MinTime: 94, MaxTime: 99},
	}
	expOverlaps := block.Overlaps{
		{Min: 15, Max: 17, Key: s.group.String()}:   {{MinTime: 10, MaxTime: 20}, expBMs[0]},
		{Min: 21, Max: 30, Key: s.group.String()}:   {{MinTime: 20, MaxTime: 30}, expBMs[1]},
		{Min: 30, Max: 31, Key: s.group.String()}:   {expBMs[1], {MinTime: 30, MaxTime: 40}},
		{Min: 34, Max: 36, Key: s.group.String()}:   {{MinTime: 30, MaxTime: 40}, expBMs[2], expBMs[3]},
		{Min: 50, Max: 60, Key: s.group.String()}:   {{MinTime: 50, MaxTime: 60}, expBMs[4]},
		{Min: 61, Max: 70, Key: s.group.String()}:   {{MinTime: 60, MaxTime: 70}, expBMs[5]},
		{Min: 70, Max: 80, Key: s.group.String()}:   {expBMs[5], {MinTime: 70, MaxTime: 80}},
		{Min: 80, Max: 85, Key: s.group.String()}:   {expBMs[5], {MinTime: 80, MaxTime: 90}},
		{Min: 94, Max: 99, Key: s.group.String()}:   {{MinTime: 90, MaxTime: 100}, expBMs[6], expBMs[7]},
		{Min: 100, Max: 105, Key: s.group.String()}: {expBMs[6], {MinTime: 100, MaxTime: 110}},
	}

	for i := range expBMs {
		s.Require().NoError(s.group.AppendMeta(&metadata.Meta{
			Thanos:    metadata.Thanos{Labels: s.ls},
			BlockMeta: expBMs[i],
		}))
	}

	overlaps := block.Overlaps{}
	s.group.OverlappingBlocks(overlaps)
	s.Equal(expOverlaps, overlaps)
}

func (s *GroupOverlappingBlocksSuite) TestOverlappingBlocks_additional_case() {
	noopCounter := promauto.With(nil).NewCounter(prometheus.CounterOpts{Name: "noop", Help: "noop"})
	s.group = tcompactor.NewGroup(
		log.NewNopLogger(),
		s.T().Name(),
		labels.FromMap(s.ls),
		0,
		noopCounter, noopCounter, noopCounter, noopCounter, noopCounter,
		false,
	)

	expBMs := []tsdb.BlockMeta{
		{MinTime: 1, MaxTime: 5},
		{MinTime: 2, MaxTime: 3},
		{MinTime: 2, MaxTime: 3},
		{MinTime: 2, MaxTime: 3},
		{MinTime: 2, MaxTime: 3},
		{MinTime: 2, MaxTime: 6},
		{MinTime: 3, MaxTime: 5},
		{MinTime: 5, MaxTime: 7},
		{MinTime: 7, MaxTime: 10},
		{MinTime: 8, MaxTime: 9},
	}

	expOverlaps := block.Overlaps{
		{Min: 2, Max: 3, Key: s.group.String()}: {expBMs[0], expBMs[1], expBMs[2], expBMs[3], expBMs[4], expBMs[5]},
		{Min: 3, Max: 5, Key: s.group.String()}: {expBMs[0], expBMs[5], expBMs[6]},
		{Min: 5, Max: 6, Key: s.group.String()}: {expBMs[5], expBMs[7]},
		{Min: 8, Max: 9, Key: s.group.String()}: {expBMs[8], expBMs[9]},
	}

	for i := range expBMs {
		s.Require().NoError(s.group.AppendMeta(&metadata.Meta{
			Thanos:    metadata.Thanos{Labels: s.ls},
			BlockMeta: expBMs[i],
		}))
	}

	overlaps := block.Overlaps{}
	s.group.OverlappingBlocks(overlaps)
	s.Equal(expOverlaps, overlaps)
}
