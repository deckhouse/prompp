package services_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/head/container"
	"github.com/prometheus/prometheus/pp/go/storage/head/services"
	"github.com/prometheus/prometheus/pp/go/storage/head/services/mock"
)

type CommitterSuite struct {
	suite.Suite

	baseCtx             context.Context
	dataDir             string
	log                 *catalog.FileLog
	catalog             *catalog.Catalog
	activeHeadContainer *container.Weighted[storage.HeadOnDisk, *storage.HeadOnDisk]
}

func TestCommitterSuite(t *testing.T) {
	suite.Run(t, new(CommitterSuite))
}

func (s *CommitterSuite) SetupSuite() {
	s.baseCtx = context.Background()
}

func (s *CommitterSuite) SetupTest() {
	s.createDataDirectory()
	s.createCatalog()
	s.activeHeadContainer = container.NewWeighted(s.createHead())
}

func (s *CommitterSuite) TearDownTest() {
	s.dataDir = ""

	if s.log != nil {
		s.NoError(s.log.Close())
		s.log = nil
	}

	if s.catalog != nil {
		s.catalog = nil
	}

	if s.activeHeadContainer != nil {
		s.NoError(s.activeHeadContainer.Close())
		s.activeHeadContainer = nil
	}
}

func (s *CommitterSuite) createDataDirectory() {
	dataDir := filepath.Join(s.T().TempDir(), "data")
	s.Require().NoError(os.MkdirAll(dataDir, os.ModeDir))
	s.dataDir = dataDir
}

func (s *CommitterSuite) createCatalog() {
	l, err := catalog.NewFileLogV2(filepath.Join(s.dataDir, "head.log"))
	s.Require().NoError(err)
	clock := clockwork.NewRealClock()

	s.catalog, err = catalog.New(
		clock,
		l,
		&catalog.DefaultIDGenerator{},
		catalog.DefaultMaxLogFileSize,
		nil,
	)
	s.Require().NoError(err)
}

func (s *CommitterSuite) createHead() *storage.HeadOnDisk {
	h, err := storage.NewBuilder(
		s.catalog,
		s.dataDir,
		maxSegmentSize,
		nil,
		unloadDataStorageInterval,
	).Build(0, shardsCount)
	s.Require().NoError(err)

	return h
}

func (s *CommitterSuite) TestCommitter() {
	trigger := make(chan struct{}, 1)
	start := make(chan struct{})
	mediator := &mock.MediatorMock{
		CFunc: func() <-chan struct{} {
			close(start)
			return trigger
		},
	}
	isNewHead := func(string) bool {
		return false
	}
	committer := services.NewCommitter(s.activeHeadContainer, mediator, isNewHead)

	done := make(chan struct{})
	go func() {
		s.NoError(committer.Execute(s.baseCtx))
		close(done)
	}()

	<-start
	trigger <- struct{}{}
	close(trigger)
	<-done
}
