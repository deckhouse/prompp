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
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

type ManagerSuite struct {
	suite.Suite

	dir       string
	compactor noopCompactor
	logger    log.Logger
}

func TestManagerSuite(t *testing.T) {
	suite.Run(t, new(ManagerSuite))
}

func (s *ManagerSuite) SetupTest() {
	s.dir = s.T().TempDir()
	s.createTestBlock(s.dir, 1000, "metric_a")
	s.createTestBlock(s.dir, 5000, "metric_b")

	s.logger = log.NewNopLogger()
}

func (s *ManagerSuite) TestManagerLoadsExistingBlocksOnStartup() {
	m, err := NewManager(s.dir, nil, s.compactor, nil, s.logger, nil)
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

	m, err := NewManager(s.dir, nil, s.compactor, blocksToDelete, s.logger, nil)
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

	m, err := NewManager(notDir, nil, s.compactor, nil, s.logger, nil)
	s.Require().Error(err)
	s.Require().Nil(m)
}

func (s *ManagerSuite) TestManagerExportsLoadedBlocksMetrics() {
	reg := prometheus.NewRegistry()
	m, err := NewManager(s.dir, nil, s.compactor, nil, s.logger, reg)
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

func (s *ManagerSuite) createTestBlock(dir string, startTS int, metric string) {
	s.T().Helper()

	series := []storage.Series{
		storage.NewListSeries(labels.FromStrings("__name__", metric), chunks.GenerateSamples(startTS, 2)),
	}
	_, err := tsdb.CreateBlock(series, dir, 0, log.NewNopLogger())
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
