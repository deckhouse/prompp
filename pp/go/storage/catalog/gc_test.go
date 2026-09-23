package catalog_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/storage/catalog"
)

const gcTestRetentionPeriod = 10 * time.Hour

type GCSuite struct {
	suite.Suite

	clock   *clockwork.FakeClock
	catalog *catalog.Catalog
	gc      *catalog.GC
}

func TestGCSuite(t *testing.T) {
	suite.Run(t, new(GCSuite))
}

func (s *GCSuite) SetupTest() {
	s.clock = clockwork.NewFakeClockAt(time.Unix(1_000_000, 0))
	dataDir := s.T().TempDir()

	l, err := catalog.NewFileLogV3(filepath.Join(dataDir, "head.log"))
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = l.Close() })

	s.catalog, err = catalog.New(s.clock, l, catalog.DefaultIDGenerator{}, catalog.DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)

	s.gc = catalog.NewGC(dataDir, s.catalog, s.clock, nil, nil, gcTestRetentionPeriod)
}

func (s *GCSuite) createRotatedRecord(maxtOffset time.Duration) *catalog.Record {
	r, err := s.catalog.Create(1)
	s.Require().NoError(err)

	createdAt := s.clock.Now()
	_, err = s.catalog.SetStatusWithTimeBounds(
		r.ID(),
		catalog.StatusRotated,
		createdAt.UnixMilli(),
		createdAt.Add(maxtOffset).UnixMilli(),
	)
	s.Require().NoError(err)

	return r
}

func (s *GCSuite) recordExists(id string) bool {
	_, err := s.catalog.Get(id)
	return err == nil
}

func (s *GCSuite) TestKeepsHeadWithMaxtInRetentionPeriod() {
	// Arrange
	r := s.createRotatedRecord(2 * time.Hour)
	s.clock.Advance(gcTestRetentionPeriod + time.Hour)

	// Act
	s.gc.Iterate()

	// Assert
	s.True(s.recordExists(r.ID()))
}

func (s *GCSuite) TestRemovesHeadWithMaxtOutOfRetentionPeriod() {
	// Arrange
	r := s.createRotatedRecord(2 * time.Hour)
	s.clock.Advance(gcTestRetentionPeriod + 2*time.Hour)

	// Act
	s.gc.Iterate()

	// Assert
	s.False(s.recordExists(r.ID()))
}

func (s *GCSuite) TestRemovesHeadWithUnknownTimeBoundsByCreatedAt() {
	// Arrange
	r, err := s.catalog.Create(1)
	s.Require().NoError(err)
	_, err = s.catalog.SetStatus(r.ID(), catalog.StatusRotated)
	s.Require().NoError(err)
	s.clock.Advance(gcTestRetentionPeriod)

	// Act
	s.gc.Iterate()

	// Assert
	s.False(s.recordExists(r.ID()))
}

func (s *GCSuite) TestKeepsNotOutdatedCorruptedHead() {
	// Arrange
	r := s.createRotatedRecord(2 * time.Hour)
	_, err := s.catalog.SetCorrupted(r.ID())
	s.Require().NoError(err)
	_, err = s.catalog.SetStatus(r.ID(), catalog.StatusPersisted)
	s.Require().NoError(err)
	s.clock.Advance(gcTestRetentionPeriod + time.Hour)

	// Act
	s.gc.Iterate()

	// Assert
	s.True(s.recordExists(r.ID()))
}
