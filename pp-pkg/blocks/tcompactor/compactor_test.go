package tcompactor_test

import (
	"testing"

	"github.com/go-kit/log"
	"github.com/prometheus/prometheus/pp-pkg/blocks/block"
	"github.com/prometheus/prometheus/pp-pkg/blocks/lcompactor"
	"github.com/prometheus/prometheus/pp-pkg/blocks/tcompactor"
	"github.com/prometheus/prometheus/pp-pkg/blocks/testutils"
	"github.com/stretchr/testify/suite"
)

type TCompactorSuite struct {
	suite.Suite
}

func TestTCompactorSuite(t *testing.T) {
	suite.Run(t, new(TCompactorSuite))
}

func (s *TCompactorSuite) TestHappyPath() {
	dir := s.T().TempDir()
	logger := log.NewNopLogger()

	countBlocks := 2
	blks := make([]*block.Block, 0, countBlocks)
	for i := range countBlocks {
		blkDir := testutils.CreateBlock(s.T(), dir, testutils.GenSeries(10, 3, int64(i), 10))
		blk, err := block.OpenBlock(logger, blkDir, nil)
		s.Require().NoError(err)
		blks = append(blks, blk)
	}
	defer func() {
		for _, blk := range blks {
			s.Require().NoError(blk.Close())
		}
	}()

	opts := tcompactor.Options{TsdbOptions: lcompactor.LeveledCompactorOptions{EnableOverlappingCompaction: true}}
	compactor, err := tcompactor.NewTCompactor(s.T().Context(), logger, dir, opts, nil)
	s.Require().NoError(err)

	ulids, err := compactor.Compact(blks)
	s.Require().NoError(err)
	s.Len(ulids, 1)

	overlaps, err := compactor.OverlappingBlocks(blks)
	s.Require().NoError(err)
	s.Len(overlaps, 1)
}

func (s *TCompactorSuite) TestNoCompact() {
	dir := s.T().TempDir()
	logger := log.NewNopLogger()

	countBlocks := 2
	blks := make([]*block.Block, 0, countBlocks)
	for i := range countBlocks {
		blkDir := testutils.CreateBlock(s.T(), dir, testutils.GenSeries(10, 3, int64((i+1)*10), int64((i+2)*10)))
		blk, err := block.OpenBlock(logger, blkDir, nil)
		s.Require().NoError(err)
		blks = append(blks, blk)
	}
	defer func() {
		for _, blk := range blks {
			s.Require().NoError(blk.Close())
		}
	}()

	opts := tcompactor.Options{TsdbOptions: lcompactor.LeveledCompactorOptions{EnableOverlappingCompaction: true}}
	compactor, err := tcompactor.NewTCompactor(s.T().Context(), logger, dir, opts, nil)
	s.Require().NoError(err)

	ulids, err := compactor.Compact(blks)
	s.Require().NoError(err)
	s.Empty(ulids, 0)

	overlaps, err := compactor.OverlappingBlocks(blks)
	s.Require().NoError(err)
	s.Empty(overlaps, 0)
}
