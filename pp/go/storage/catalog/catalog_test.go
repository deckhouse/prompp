package catalog_test

import (
	"os"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/storage/catalog"
)

type CatalogSuite struct {
	suite.Suite
}

func TestCatalogSuite(t *testing.T) {
	suite.Run(t, new(CatalogSuite))
}

func (s *CatalogSuite) TestHappyPath() {
	tmpFile, err := os.CreateTemp("", "log_file")
	s.Require().NoError(err)

	logFileName := tmpFile.Name()
	s.Require().NoError(tmpFile.Close())

	l, err := catalog.NewFileLogV2(logFileName)
	s.Require().NoError(err)

	clock := clockwork.NewFakeClockAt(time.Now())

	idGenerator := &testIDGenerator{}

	c, err := catalog.New(
		clock,
		l,
		idGenerator,
		catalog.DefaultMaxLogFileSize,
		nil,
	)
	s.Require().NoError(err)

	now := clock.Now().UnixMilli()

	var nos1 uint16 = 2
	var nos2 uint16 = 4

	r1, err := c.Create(nos1)
	s.Require().NoError(err)

	s.Require().Equal(idGenerator.last(), r1.ID())
	s.Require().Equal(idGenerator.last(), r1.Dir())
	s.Require().Equal(nos1, r1.NumberOfShards())
	s.Require().Equal(now, r1.CreatedAt())
	s.Require().Equal(now, r1.UpdatedAt())
	s.Require().Equal(int64(0), r1.DeletedAt())
	s.Require().Equal(catalog.StatusNew, r1.Status())

	clock.Advance(time.Second)
	now = clock.Now().UnixMilli()

	r2, err := c.Create(nos2)
	s.Require().NoError(err)

	s.Require().Equal(idGenerator.last(), r2.ID())
	s.Require().Equal(idGenerator.last(), r2.Dir())
	s.Require().Equal(nos2, r2.NumberOfShards())
	s.Require().Equal(now, r2.CreatedAt())
	s.Require().Equal(now, r2.UpdatedAt())
	s.Require().Equal(int64(0), r2.DeletedAt())
	s.Require().Equal(catalog.StatusNew, r2.Status())

	_, err = c.SetStatus(r1.ID(), catalog.StatusPersisted)
	s.Require().NoError(err)

	c = nil
	s.Require().NoError(l.Close())

	l, err = catalog.NewFileLogV2(logFileName)
	s.Require().NoError(err)
	c, err = catalog.New(
		clock,
		l,
		catalog.DefaultIDGenerator{},
		catalog.DefaultMaxLogFileSize,
		nil,
	)
	s.Require().NoError(err)

	records, err := c.List(nil, nil)
	s.Require().NoError(err)
	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt() < records[j].CreatedAt()
	})
}

func (s *CatalogSuite) TestCatalogSyncFail() {
	tmpFile, err := os.CreateTemp("", "log_file")
	s.Require().NoError(err)

	logFileName := tmpFile.Name()
	s.Require().NoError(tmpFile.Close())

	l, err := catalog.NewFileLogV2(logFileName)
	s.Require().NoError(err)

	clock := clockwork.NewFakeClockAt(time.Now())

	idGenerator := &testIDGenerator{}

	c, err := catalog.New(
		clock,
		l,
		idGenerator,
		catalog.DefaultMaxLogFileSize,
		prometheus.DefaultRegisterer,
	)
	s.Require().NoError(err)

	var nos1 uint16 = 2
	var nos2 uint16 = 4

	r1, err := c.Create(nos1)
	s.Require().NoError(err)

	r2, err := c.Create(nos2)
	s.Require().NoError(err)

	fileInfo, err := os.Stat(logFileName)
	s.Require().NoError(err)
	s.Require().NoError(os.Truncate(logFileName, fileInfo.Size()-1))

	l, err = catalog.NewFileLogV2(logFileName)
	s.Require().NoError(err)

	c, err = catalog.New(
		clock,
		l,
		idGenerator,
		catalog.DefaultMaxLogFileSize,
		nil,
	)
	s.Require().NoError(err)

	restoredR1, err := c.Get(r1.ID())
	s.Require().NoError(err)

	_, err = c.Get(r2.ID())
	s.Require().Error(err)

	s.Require().Equal(r1.ID(), restoredR1.ID())
	s.Require().Equal(r1.Dir(), restoredR1.Dir())
	s.Require().Equal(r1.NumberOfShards(), restoredR1.NumberOfShards())
	s.Require().Equal(r1.CreatedAt(), restoredR1.CreatedAt())
	s.Require().Equal(r1.UpdatedAt(), restoredR1.UpdatedAt())
	s.Require().Equal(r1.DeletedAt(), restoredR1.DeletedAt())
	s.Require().Equal(r1.Status(), restoredR1.Status())
}

// testIDGenerator generator UUID for test.
type testIDGenerator struct {
	lastUUID uuid.UUID
}

// Generate UUID. Implementation [catalog.IDGenerator].
func (g *testIDGenerator) Generate() uuid.UUID {
	g.lastUUID = uuid.New()
	return g.lastUUID
}

// last returns last UUID as string.
func (g *testIDGenerator) last() string {
	return g.lastUUID.String()
}
