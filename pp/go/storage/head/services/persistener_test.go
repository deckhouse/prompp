package services_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/block"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/head/container"
	"github.com/prometheus/prometheus/pp/go/storage/head/keeper"
	"github.com/prometheus/prometheus/pp/go/storage/head/services"
	"github.com/prometheus/prometheus/pp/go/storage/head/services/mock"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/task"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
	"github.com/stretchr/testify/suite"
)

const (
	shardsCount               = 2
	maxSegmentSize            = 1024
	unloadDataStorageInterval = time.Duration(1)

	tsdbRetentionPeriod = time.Millisecond * 100

	retentionPeriod = time.Minute * 5
)

type GenericPersistenceSuite struct {
	suite.Suite
	dataDir         string
	longtermDataDir string
	clock           *clockwork.FakeClock
	catalog         *catalog.Catalog
	proxy           *storage.Proxy
	blockWriter     *mock.HeadBlockWriterMock[*shard.Shard]
	writeNotifier   *mock.WriteNotifierMock
}

func (s *GenericPersistenceSuite) SetupTest() {
	s.dataDir = s.createDataDirectory()
	s.clock = clockwork.NewFakeClockAt(time.UnixMilli(0))
	s.createCatalog()

	h := s.mustCreateHead()
	activeHeadContainer := container.NewWeighted(h, container.DefaultBackPressure)
	removedHeadNotifier := &mock.WriteNotifierMock{NotifyFunc: func() {}}
	hKeeper := keeper.NewKeeper[storage.Head](1, removedHeadNotifier)
	s.proxy = storage.NewProxy(activeHeadContainer, hKeeper, func(*storage.Head) error { return nil })
	s.blockWriter = &mock.HeadBlockWriterMock[*shard.Shard]{}
	s.writeNotifier = &mock.WriteNotifierMock{NotifyFunc: func() {}}
}

func (s *GenericPersistenceSuite) createDataDirectory() string {
	dataDir := filepath.Join(s.T().TempDir(), "data")
	s.Require().NoError(os.MkdirAll(dataDir, os.ModeDir))
	return dataDir
}

func (s *GenericPersistenceSuite) createHead() (*storage.Head, error) {
	return storage.NewBuilder(
		s.catalog,
		s.dataDir,
		maxSegmentSize,
		prometheus.DefaultRegisterer,
		unloadDataStorageInterval,
	).Build(0, shardsCount)
}

func (s *GenericPersistenceSuite) mustCreateHead() *storage.Head {
	h, err := s.createHead()
	s.Require().NoError(err)
	return h
}

func (s *GenericPersistenceSuite) createCatalog() {
	l, err := catalog.NewFileLogV2(filepath.Join(s.dataDir, "catalog.log"))
	s.Require().NoError(err)

	s.catalog, err = catalog.New(
		s.clock,
		l,
		&catalog.DefaultIDGenerator{},
		catalog.DefaultMaxLogFileSize,
		nil,
	)
	s.Require().NoError(err)
}

type PersistenerSuite struct {
	GenericPersistenceSuite
	persistener *services.Persistener[
		*task.Generic[*shard.PerGoroutineShard],
		*shard.Shard,
		*shard.PerGoroutineShard,
		*mock.HeadBlockWriterMock[*shard.Shard],
		*storage.Head,
	]
}

func (s *PersistenerSuite) SetupTest() {
	s.GenericPersistenceSuite.SetupTest()

	s.persistener = services.NewPersistener[
		*task.Generic[*shard.PerGoroutineShard],
		*shard.Shard,
		*shard.PerGoroutineShard,
		*mock.HeadBlockWriterMock[*shard.Shard],
		*storage.Head,
	](s.catalog, s.blockWriter, s.writeNotifier, s.clock, tsdbRetentionPeriod, retentionPeriod, nil)
}

func TestPersistenerSuite(t *testing.T) {
	suite.Run(t, new(PersistenerSuite))
}

func (s *PersistenerSuite) TestNoHeads() {
	// Arrange

	// Act
	outdated := s.persistener.Persist(nil)

	// Assert
	s.Equal([]*storage.Head(nil), outdated)
	s.Empty(s.blockWriter.WriteCalls())
}

func (s *PersistenerSuite) TestNoPersistWritableHead() {
	// Arrange
	heads := []*storage.Head{s.mustCreateHead()}

	// Act
	outdated := s.persistener.Persist(heads)

	// Assert
	s.Equal([]*storage.Head(nil), outdated)
	s.Empty(s.blockWriter.WriteCalls())
}

func (s *PersistenerSuite) TestNoPersistPersistedHead() {
	// Arrange
	head := s.mustCreateHead()
	storagetest.MustAppendTimeSeries(&s.Suite, head, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "value1"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 0},
			},
		},
	})
	head.SetReadOnly()
	_, err := s.catalog.SetStatus(head.ID(), catalog.StatusPersisted)
	s.Require().NoError(err)

	s.clock.Advance(retentionPeriod - 1)

	// Act
	outdated := s.persistener.Persist([]*storage.Head{head})

	// Assert
	s.Equal([]*storage.Head(nil), outdated)
	s.Empty(s.blockWriter.WriteCalls())
}

func (s *PersistenerSuite) TestOutdatedPersistedHead() {
	// Arrange
	head := s.mustCreateHead()
	storagetest.MustAppendTimeSeries(&s.Suite, head, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "value1"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 0},
			},
		},
	})
	head.SetReadOnly()
	_, err := s.catalog.SetStatus(head.ID(), catalog.StatusPersisted)
	s.Require().NoError(err)

	s.clock.Advance(retentionPeriod)

	// Act
	outdated := s.persistener.Persist([]*storage.Head{head})

	// Assert
	s.Equal([]*storage.Head{head}, outdated)
	s.Empty(s.blockWriter.WriteCalls())
}

func (s *PersistenerSuite) TestOutdatedHead() {
	// Arrange
	s.clock.Advance(tsdbRetentionPeriod)

	head := s.mustCreateHead()
	storagetest.MustAppendTimeSeries(&s.Suite, head, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "value1"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 0},
			},
		},
	})
	head.SetReadOnly()

	// Act
	outdated := s.persistener.Persist([]*storage.Head{head})

	// Assert
	s.Equal([]*storage.Head{head}, outdated)
	s.Empty(s.blockWriter.WriteCalls())
}

func (s *PersistenerSuite) TestPersistHeadSuccess() {
	// Arrange
	s.clock.Advance(tsdbRetentionPeriod)
	blockWriter := block.NewWriter[*shard.Shard](
		s.dataDir,
		s.longtermDataDir,
		block.DefaultChunkSegmentSize,
		cppbridge.NoDownsampling,
		2*time.Hour,
		prometheus.DefaultRegisterer,
	)
	s.blockWriter.WriteFunc = func(shard *shard.Shard) ([]block.WrittenBlock, error) {
		return blockWriter.Write(shard)
	}

	head := s.mustCreateHead()
	storagetest.MustAppendTimeSeries(&s.Suite, head, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "value1"),
			Samples: []cppbridge.Sample{
				{Timestamp: 1, Value: 0},
			},
		},
	})
	head.SetReadOnly()

	// Act
	outdated := s.persistener.Persist([]*storage.Head{head})
	record, err := s.catalog.Get(head.ID())

	// Assert
	s.Equal([]*storage.Head(nil), outdated)
	s.Len(s.blockWriter.WriteCalls(), 2)
	s.Len(s.writeNotifier.NotifyCalls(), 1)
	s.Require().NoError(err)
	s.Equal(catalog.StatusPersisted, record.Status())
}

func (s *PersistenerSuite) TestPersistHeadErrorOnBlockWriterForSecondShard() {
	// Arrange
	s.clock.Advance(tsdbRetentionPeriod)
	blockWriter := block.NewWriter[*shard.Shard](
		s.dataDir,
		s.longtermDataDir,
		block.DefaultChunkSegmentSize,
		cppbridge.NoDownsampling,
		2*time.Hour,
		prometheus.DefaultRegisterer,
	)
	s.blockWriter.WriteFunc = func(shard *shard.Shard) ([]block.WrittenBlock, error) {
		if len(s.blockWriter.WriteCalls()) == 2 {
			return nil, errors.New("some error")
		}

		return blockWriter.Write(shard)
	}

	head := s.mustCreateHead()
	storagetest.MustAppendTimeSeries(&s.Suite, head, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "value1"),
			Samples: []cppbridge.Sample{
				{Timestamp: 1, Value: 0},
			},
		},
	})
	head.SetReadOnly()

	// Act
	outdated := s.persistener.Persist([]*storage.Head{head})
	record, err := s.catalog.Get(head.ID())

	// Assert
	s.Equal([]*storage.Head(nil), outdated)
	s.Len(s.blockWriter.WriteCalls(), 2)
	s.Empty(s.writeNotifier.NotifyCalls())
	s.Require().NoError(err)
	s.Equal(catalog.StatusNew, record.Status())
}

type PersistenerServiceSuite struct {
	GenericPersistenceSuite
	loader  *storage.Loader
	service *services.PersistenerService[
		*task.Generic[*shard.PerGoroutineShard],
		*shard.Shard,
		*shard.PerGoroutineShard,
		*mock.HeadBlockWriterMock[*shard.Shard],
		*storage.Head,
		*storage.Proxy,
		*storage.Loader,
	]
}

func (s *PersistenerServiceSuite) SetupTest() {
	s.GenericPersistenceSuite.SetupTest()

	s.loader = storage.NewLoader(s.dataDir, maxSegmentSize, prometheus.DefaultRegisterer, unloadDataStorageInterval)
	s.service = services.NewPersistenerService[
		*task.Generic[*shard.PerGoroutineShard],
		*shard.Shard,
		*shard.PerGoroutineShard,
		*mock.HeadBlockWriterMock[*shard.Shard],
		*storage.Head,
		*storage.Proxy,
		*storage.Loader,
	](
		s.proxy,
		s.loader,
		s.catalog,
		s.blockWriter,
		s.writeNotifier,
		s.clock,
		nil,
		tsdbRetentionPeriod,
		retentionPeriod,
		nil,
	)
}

func TestPersistenerServiceSuite(t *testing.T) {
	suite.Run(t, new(PersistenerServiceSuite))
}

func (s *PersistenerServiceSuite) TestRemoveOutdatedHeadFromKeeper() {
	// Arrange
	s.clock.Advance(tsdbRetentionPeriod)
	head := s.mustCreateHead()
	storagetest.MustAppendTimeSeries(&s.Suite, head, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "value1"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 0},
			},
		},
	})
	head.SetReadOnly()
	record, _ := s.catalog.SetStatus(head.ID(), catalog.StatusRotated)
	_ = s.proxy.Add(head, time.Duration(s.clock.Now().Nanosecond()))

	// Act
	s.service.ProcessHeads()

	// Assert
	s.Empty(s.proxy.Heads())
	s.Equal(catalog.StatusPersisted, record.Status())
}

func (s *PersistenerServiceSuite) TestLoadHeadsInKeeper() {
	// Arrange
	head := s.mustCreateHead()
	storagetest.MustAppendTimeSeries(&s.Suite, head, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "value1"),
			Samples: []cppbridge.Sample{
				{Timestamp: 1, Value: 0},
			},
		},
	})
	record, _ := s.catalog.SetStatus(head.ID(), catalog.StatusRotated)
	s.Require().NoError(head.Close())

	// Act
	s.service.ProcessHeads()

	// Assert
	s.Require().Len(s.proxy.Heads(), 1)
	s.Equal(head.ID(), s.proxy.Heads()[0].ID())
	s.Equal(int64(0), record.CreatedAt())
}

func (s *PersistenerServiceSuite) TestHeadAlreadyExistsInKeeper() {
	// Arrange
	head := s.mustCreateHead()
	storagetest.MustAppendTimeSeries(&s.Suite, head, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "value1"),
			Samples: []cppbridge.Sample{
				{Timestamp: 1, Value: 0},
			},
		},
	})
	_, _ = s.catalog.SetStatus(head.ID(), catalog.StatusRotated)
	_ = s.proxy.Add(head, 0)

	// Act
	s.service.ProcessHeads()

	// Assert
	s.Require().Len(s.proxy.Heads(), 1)
	s.Equal(head.ID(), s.proxy.Heads()[0].ID())
}
