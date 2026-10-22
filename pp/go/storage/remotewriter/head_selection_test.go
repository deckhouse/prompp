package remotewriter

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/storage/catalog"
)

// HeadSelectionSuite checks that heads without WAL shards are skipped by remote write.
type HeadSelectionSuite struct {
	suite.Suite

	clock    *clockwork.FakeClock
	dataDir  string
	catalog  *catalog.Catalog
	previous *catalog.Record
	next     *catalog.Record
}

func TestHeadSelection(t *testing.T) {
	suite.Run(t, new(HeadSelectionSuite))
}

func (s *HeadSelectionSuite) SetupTest() {
	s.clock = clockwork.NewFakeClock()
	s.dataDir = s.T().TempDir()
	log, err := catalog.NewFileLogV2(filepath.Join(s.dataDir, "head.log"))
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.NoError(log.Close()) })

	s.catalog, err = catalog.New(s.clock, log, catalog.DefaultIDGenerator{}, catalog.DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)
	s.previous = s.createHead(1)
	s.createHead(0)
	s.next = s.createHead(1)
	s.createHead(0)
}

func (s *HeadSelectionSuite) TestNextHeadSkipsHeadsWithoutWAL() {
	got, err := nextHead(s.T().Context(), s.dataDir, s.catalog, s.previous.ID())

	s.Require().NoError(err)
	s.Equal(s.next.ID(), got.ID())
}

func (s *HeadSelectionSuite) TestScanForNextHeadSkipsHeadsWithoutWAL() {
	got, found, err := scanForNextHead(s.T().Context(), s.dataDir, s.catalog, "destination")

	s.Require().NoError(err)
	s.False(found)
	s.Equal(s.next.ID(), got.ID())
}

func (s *HeadSelectionSuite) createHead(shards uint16) *catalog.Record {
	s.clock.Advance(time.Second)
	record, err := s.catalog.Create(shards)
	s.Require().NoError(err)
	s.Require().NoError(os.Mkdir(filepath.Join(s.dataDir, record.ID()), 0o700))

	if shards == 0 {
		_, err = s.catalog.SetStatus(record.ID(), catalog.StatusPersisted)
		s.Require().NoError(err)
	}

	return record
}
