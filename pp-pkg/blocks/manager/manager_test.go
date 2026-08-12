package manager

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp-pkg/blocks/block"
	"github.com/prometheus/prometheus/pp-pkg/blocks/upsampler"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

type ManagerSuite struct {
	suite.Suite

	dir       string
	compactor noopCompactor
	logger    log.Logger
	chunkPool chunkenc.Pool
}

func TestManagerSuite(t *testing.T) {
	suite.Run(t, new(ManagerSuite))
}

func (s *ManagerSuite) SetupTest() {
	s.dir = s.T().TempDir()
	s.createTestBlock(s.dir, 1000, "metric_a")
	s.createTestBlock(s.dir, 5000, "metric_b")

	s.logger = log.NewNopLogger()
	s.chunkPool = chunkenc.NewPool()
}

func (s *ManagerSuite) TestManagerLoadsExistingBlocksOnStartup() {
	m, err := NewManager(s.dir, nil, s.compactor, nil, s.chunkPool, nil, s.logger, nil)
	s.Require().NoError(err)
	s.T().Cleanup(m.Close)

	blocks := m.Blocks()
	s.Require().Len(blocks, 2)
	s.Require().LessOrEqual(blocks[0].Meta().MinTime, blocks[1].Meta().MinTime)
}

func (s *ManagerSuite) TestManagerAppliesBlocksToDeleteOnInitialReload() {
	var marked ulid.ULID
	blocksToDelete := func(blocks []*block.Block) map[ulid.ULID]struct{} {
		if len(blocks) == 0 {
			return nil
		}
		if marked.Compare(ulid.ULID{}) == 0 {
			marked = blocks[0].Meta().ULID
		}
		return map[ulid.ULID]struct{}{marked: {}}
	}

	m, err := NewManager(s.dir, nil, s.compactor, blocksToDelete, s.chunkPool, nil, s.logger, nil)
	s.Require().NoError(err)
	s.T().Cleanup(m.Close)

	blocks := m.Blocks()
	s.Require().Len(blocks, 1)
	s.Require().NotEqual(marked, blocks[0].Meta().ULID)

	_, err = os.Stat(filepath.Join(s.dir, marked.String()))
	s.Require().True(os.IsNotExist(err), "expected deleted block dir to be removed")
}

func (s *ManagerSuite) TestManagerReturnsErrorOnInitialReloadFailure() {
	tmp := s.T().TempDir()
	notDir := filepath.Join(tmp, "not-a-directory")
	s.Require().NoError(os.WriteFile(notDir, []byte("x"), 0o600))

	m, err := NewManager(notDir, nil, s.compactor, nil, s.chunkPool, nil, s.logger, nil)
	s.Require().Error(err)
	s.Require().Nil(m)
}

func (s *ManagerSuite) TestManagerExportsLoadedBlocksMetrics() {
	reg := prometheus.NewRegistry()
	m, err := NewManager(s.dir, nil, s.compactor, nil, s.chunkPool, nil, s.logger, reg)
	s.Require().NoError(err)
	s.T().Cleanup(m.Close)

	s.Require().Equal(float64(2), testutil.ToFloat64(m.metrics.loadedBlocks))
	s.Require().Greater(testutil.ToFloat64(m.metrics.symbolTableSize), 0.0)

	durationCounts := map[int64]int{}
	for _, b := range m.Blocks() {
		duration := normalizeBlockDurationMinutes(b.Meta().MaxTime - b.Meta().MinTime)
		durationCounts[duration]++
	}
	for duration, count := range durationCounts {
		s.Require().Equal(
			float64(count),
			testutil.ToFloat64(m.metrics.loadedBlocksByDuration.WithLabelValues(strconv.FormatInt(duration, 10))),
		)
	}
}

func (s *ManagerSuite) TestManagerQuerierWrapsDownsamplingBlocks() {
	workDir := s.T().TempDir()
	// Create a downsampling block
	s.createTestDownsamplingBlock(s.dir, 1000, workDir, 60000)

	m, err := NewManager(s.dir, &Options{
		RetentionDuration: 100,
		DownsamplingMS:    60000, // 1 minute downsampling
	}, s.compactor, nil, s.chunkPool, nil, s.logger, nil)
	s.Require().NoError(err)
	s.T().Cleanup(m.Close)

	// Get blocks and verify they were loaded (now we have 3: 2 from SetupTest + 1 downsampling)
	blocks := m.Blocks()
	s.Require().Len(blocks, 3)

	// Verify at least one block is a downsampling block
	hasDownsamplingBlock := false
	for _, b := range blocks {
		if b.IsDownsamplingBlock() {
			hasDownsamplingBlock = true
			break
		}
	}
	s.Require().True(hasDownsamplingBlock, "should have at least one downsampling block")

	// Create a querier for a range that triggers downsampling
	// needDownsampling returns true when (maxt - mint) > retentionMS
	mintMS, maxtMS := int64(0), int64(1000) // Wide range to trigger downsampling
	q, err := m.Querier(mintMS, maxtMS)
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = q.Close() })

	// Verify that querier was created and is functional
	s.NotNil(q)

	uq, ok := q.(*upsampler.Querier)
	s.Require().True(ok)
	s.NotNil(uq)

	// Try a simple select (without upsample because no SelectHints provided)
	ss := q.Select(s.T().Context(), false, nil)
	s.NotNil(ss)
}

func (s *ManagerSuite) TestSkipBlock() {
	workDir := s.T().TempDir()
	// Create a downsampling block
	s.createTestDownsamplingBlock(s.dir, 1000, workDir, 60000)

	m, err := NewManager(s.dir, &Options{
		RetentionDuration: 100,
		DownsamplingMS:    60000, // 1 minute downsampling
	}, s.compactor, nil, s.chunkPool, nil, s.logger, nil)
	s.Require().NoError(err)
	s.T().Cleanup(m.Close)

	// Get blocks and verify they were loaded (now we have 3: 2 from SetupTest + 1 downsampling)
	blocks := m.Blocks()
	s.Require().Len(blocks, 3)

	// Verify at least one block is a downsampling block
	mintMS, maxtMS := int64(0), int64(1000) // Wide range to trigger downsampling
	hasDownsamplingBlock := false
	for _, b := range blocks {
		if b.IsDownsamplingBlock() {
			s.Require().False(m.skipBlock(b, mintMS, maxtMS, true))
			s.Require().True(m.skipBlock(b, mintMS, maxtMS, false))
			hasDownsamplingBlock = true
			break
		}
		s.Require().True(m.skipBlock(b, mintMS, maxtMS, true))
		s.Require().False(m.skipBlock(b, mintMS, maxtMS, false))
	}
	s.Require().True(hasDownsamplingBlock, "should have at least one downsampling block")
}

func (s *ManagerSuite) createTestBlock(dir string, startTS int, metric string) {
	s.T().Helper()

	series := []storage.Series{
		storage.NewListSeries(labels.FromStrings("__name__", metric), chunks.GenerateSamples(startTS, 2)),
	}
	_, err := tsdb.CreateBlock(series, dir, 0, log.NewNopLogger())
	s.Require().NoError(err)
}

func (s *ManagerSuite) createTestDownsamplingBlock(dir string, startTS int, metric string, resolution int64) {
	s.T().Helper()

	// Create a normal block first
	series := []storage.Series{
		storage.NewListSeries(labels.FromStrings("__name__", metric), chunks.GenerateSamples(startTS, 2)),
	}
	blockIDStr, err := tsdb.CreateBlock(series, dir, 0, s.logger)
	s.Require().NoError(err)

	// Create Thanos metadata file for downsampling BEFORE loading the block
	// This way when OpenBlocks loads the block, it will read the resolution
	meta, _, err := block.ReadFromDir(blockIDStr)
	s.Require().NoError(err)
	meta.Thanos.Downsample.Resolution = resolution

	_, err = block.WriteThanosMetaFile(s.logger, blockIDStr, meta)
	s.Require().NoError(err)
}

//
// NoopCompactor
//

// noopCompactor is a compaction runner that does nothing.
type noopCompactor struct{}

// Compact implements the compactionRunner interface.
func (noopCompactor) Compact([]*block.Block) ([]ulid.ULID, error) {
	return nil, nil
}

// OverlappingBlocks implements the compactionRunner interface.
func (noopCompactor) OverlappingBlocks([]*block.Block) (block.Overlaps, error) {
	return nil, nil
}
