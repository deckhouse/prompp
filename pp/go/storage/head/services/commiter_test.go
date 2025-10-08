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
	activeHeadContainer *container.Weighted[storage.Head, *storage.Head]
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

func (s *CommitterSuite) createHead() *storage.Head {
	h, err := storage.NewBuilder(
		s.catalog,
		s.dataDir,
		maxSegmentSize,
		nil,
		unloadDataStorageInterval,
	).Build(0, shardsCount)
	s.Require().NoError(err)

	// swn := writer.NewSegmentWriteNotifier(shardsCount, func(uint32) {})
	// for shardID := range shardsCount {
	// 	s, err := b.createShardOnDisk(headDir, swn, shardID)
	// 	if err != nil {
	// 		return nil, err
	// 	}

	// 	shards[shardID] = s
	// }

	return h
}

// func (s *CommitterSuite) createShardOnMemory(
// 	headDir string,
// 	swn *writer.SegmentWriteNotifier,
// 	shardID uint16,
// ) (*shard.Shard, error) {
// 	headDir = filepath.Clean(headDir)
// 	shardFile, err := os.OpenFile( //nolint:gosec // need this permissions
// 		GetShardWalFilename(headDir, shardID),
// 		os.O_WRONLY|os.O_CREATE|os.O_APPEND,
// 		0o666, //revive:disable-line:add-constant // file permissions simple readable as octa-number
// 	)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to create shard wal file id %d: %w", shardID, err)
// 	}

// 	defer func() {
// 		if err == nil {
// 			return
// 		}

// 		_ = shardFile.Close()
// 	}()

// 	lss := shard.NewLSS()
// 	// logShards is 0 for single encoder
// 	shardWalEncoder := cppbridge.NewHeadWalEncoder(shardID, 0, lss.Target())

// 	_, err = writer.WriteHeader(shardFile, wal.FileFormatVersion, shardWalEncoder.Version())
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to write header: %w", err)
// 	}

// 	sw, err := writer.NewBuffered(shardID, shardFile, writer.WriteSegment[*cppbridge.HeadEncodedSegment], swn)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to create buffered writer shard id %d: %w", shardID, err)
// 	}

// 	var unloadedDataStorage *shard.UnloadedDataStorage
// 	var queriedSeriesStorage *shard.QueriedSeriesStorage
// 	if b.unloadDataStorageInterval != 0 {
// 		unloadedDataStorage = shard.NewUnloadedDataStorage(
// 			shard.NewFileStorage(GetUnloadedDataStorageFilename(headDir, shardID)),
// 		)

// 		queriedSeriesStorage = shard.NewQueriedSeriesStorage(
// 			shard.NewFileStorage(GetQueriedSeriesStorageFilename(headDir, shardID, 0)),
// 			shard.NewFileStorage(GetQueriedSeriesStorageFilename(headDir, shardID, 1)),
// 		)
// 	}

// 	return shard.NewShard(
// 		lss,
// 		shard.NewDataStorage(),
// 		unloadedDataStorage,
// 		queriedSeriesStorage,
// 		wal.NewWal(shardWalEncoder, sw, b.maxSegmentSize),
// 		shardID,
// 	), nil
// }

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
