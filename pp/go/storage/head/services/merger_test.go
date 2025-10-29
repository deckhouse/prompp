package services_test

import (
	"context"
	"testing"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/head/container"
	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/head/services"
	"github.com/prometheus/prometheus/pp/go/storage/head/services/mock"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
	"github.com/stretchr/testify/suite"
)

type MergerSuite struct {
	suite.Suite

	baseCtx context.Context
}

func TestMergerSuite(t *testing.T) {
	suite.Run(t, new(MergerSuite))
}

func (s *MergerSuite) SetupSuite() {
	s.baseCtx = context.Background()
}

func (s *MergerSuite) createHead(
	unloadedFS []*mock.AppendFileMock,
	queriedSeriesFS [][2]*mock.StorageFileMock,
) *storage.Head {
	shards := make([]*shard.Shard, shardsCount)
	for shardID := range unloadedFS {
		shards[shardID] = s.createShardOnMemory(
			unloadedFS[shardID],
			queriedSeriesFS[shardID],
			maxSegmentSize,
			uint16(shardID),
		)
	}

	return head.NewHead(
		"test-head-id",
		false,
		true,
		shards,
		shard.NewPerGoroutineShard[*storage.Wal],
		nil,
		0,
		nil,
	)
}

func (*MergerSuite) createShardOnMemory(
	unloadedFS *mock.AppendFileMock,
	queriedSeriesFS [2]*mock.StorageFileMock,
	maxSegmentSize uint32,
	shardID uint16,
) *shard.Shard {
	lss := shard.NewLSS()
	shardWalEncoder := cppbridge.NewHeadWalEncoder(shardID, 0, lss.Target())

	segmentWriter := &mock.SegmentWriterMock{
		WriteFunc:       func(*cppbridge.HeadEncodedSegment) error { return nil },
		FlushFunc:       func() error { return nil },
		SyncFunc:        func() error { return nil },
		CloseFunc:       func() error { return nil },
		CurrentSizeFunc: func() int64 { return 0 },
	}

	unloadedDataStorage := shard.NewUnloadedDataStorage(unloadedFS)
	queriedSeriesStorage := shard.NewQueriedSeriesStorage(
		queriedSeriesFS[0],
		queriedSeriesFS[1],
	)

	return shard.NewShard(
		lss,
		shard.NewDataStorage(),
		unloadedDataStorage,
		queriedSeriesStorage,
		wal.NewWal(shardWalEncoder, segmentWriter, maxSegmentSize, shardID, nil),
		shardID,
	)
}

func (s *MergerSuite) TestHappyPath() {
	trigger := make(chan struct{}, 1)
	start := make(chan struct{})
	mediator := &mock.MediatorMock{
		CFunc: func() <-chan struct{} {
			close(start)
			return trigger
		},
	}

	unloadedFS := make([]*mock.AppendFileMock, shardsCount)
	queriedSeriesFS := make([][2]*mock.StorageFileMock, shardsCount)
	for shardID := range shardsCount {
		unloadedFS[shardID] = &mock.AppendFileMock{
			CloseFunc: func() error { return nil },
			OpenFunc:  func() error { return nil },
			SyncFunc:  func() error { return nil },
			WriteFunc: func([]byte) (int, error) { return 0, nil },
		}

		queriedSeriesFS[shardID] = [2]*mock.StorageFileMock{
			{
				CloseFunc:    func() error { return nil },
				OpenFunc:     func(int) error { return nil },
				SeekFunc:     func(int64, int) (int64, error) { return 0, nil },
				SyncFunc:     func() error { return nil },
				TruncateFunc: func(int64) error { return nil },
				WriteFunc:    func([]byte) (int, error) { return 0, nil },
			},
			{
				CloseFunc:    func() error { return nil },
				OpenFunc:     func(int) error { return nil },
				SeekFunc:     func(int64, int) (int64, error) { return 0, nil },
				SyncFunc:     func() error { return nil },
				TruncateFunc: func(int64) error { return nil },
				WriteFunc:    func([]byte) (int, error) { return 0, nil },
			},
		}
	}

	activeHeadContainer := container.NewWeighted(
		s.createHead(unloadedFS, queriedSeriesFS),
		container.DefaultBackPressure,
	)
	isNewHead := func(string) bool { return false }

	merger := services.NewMerger(activeHeadContainer, mediator, isNewHead)
	done := make(chan struct{})

	s.T().Run("execute", func(t *testing.T) {
		t.Parallel()

		err := merger.Execute(s.baseCtx)
		close(done)
		s.NoError(err)
	})

	s.T().Run("tick", func(t *testing.T) {
		t.Parallel()

		<-start
		trigger <- struct{}{}
		close(trigger)
		<-done

		s.Require().NoError(activeHeadContainer.Close())

		for shardID := range unloadedFS {
			s.Len(unloadedFS[shardID].OpenCalls(), 1)
			s.Len(unloadedFS[shardID].SyncCalls(), 1)
			s.Len(unloadedFS[shardID].WriteCalls(), 2)

			s.Len(queriedSeriesFS[shardID][0].OpenCalls(), 1)
			s.Len(queriedSeriesFS[shardID][0].SeekCalls(), 1)
			s.Len(queriedSeriesFS[shardID][0].SyncCalls(), 1)
			s.Len(queriedSeriesFS[shardID][0].TruncateCalls(), 1)
			s.Len(queriedSeriesFS[shardID][0].WriteCalls(), 2)

			s.Empty(queriedSeriesFS[shardID][1].OpenCalls())
			s.Empty(queriedSeriesFS[shardID][1].SeekCalls())
			s.Empty(queriedSeriesFS[shardID][1].SyncCalls())
			s.Empty(queriedSeriesFS[shardID][1].TruncateCalls())
			s.Empty(queriedSeriesFS[shardID][1].WriteCalls())
		}
	})
}

func (s *MergerSuite) TestSkipNewHead() {
	trigger := make(chan struct{}, 1)
	start := make(chan struct{})
	mediator := &mock.MediatorMock{
		CFunc: func() <-chan struct{} {
			close(start)
			return trigger
		},
	}

	unloadedFS := make([]*mock.AppendFileMock, shardsCount)
	queriedSeriesFS := make([][2]*mock.StorageFileMock, shardsCount)
	for shardID := range shardsCount {
		unloadedFS[shardID] = &mock.AppendFileMock{
			CloseFunc: func() error { return nil },
			OpenFunc:  func() error { return nil },
			SyncFunc:  func() error { return nil },
			WriteFunc: func([]byte) (int, error) { return 0, nil },
		}

		queriedSeriesFS[shardID] = [2]*mock.StorageFileMock{
			{
				CloseFunc:    func() error { return nil },
				OpenFunc:     func(int) error { return nil },
				SeekFunc:     func(int64, int) (int64, error) { return 0, nil },
				SyncFunc:     func() error { return nil },
				TruncateFunc: func(int64) error { return nil },
				WriteFunc:    func([]byte) (int, error) { return 0, nil },
			},
			{
				CloseFunc:    func() error { return nil },
				OpenFunc:     func(int) error { return nil },
				SeekFunc:     func(int64, int) (int64, error) { return 0, nil },
				SyncFunc:     func() error { return nil },
				TruncateFunc: func(int64) error { return nil },
				WriteFunc:    func([]byte) (int, error) { return 0, nil },
			},
		}
	}

	activeHeadContainer := container.NewWeighted(
		s.createHead(unloadedFS, queriedSeriesFS),
		container.DefaultBackPressure,
	)
	isNewHead := func(string) bool { return true }

	merger := services.NewMerger(activeHeadContainer, mediator, isNewHead)
	done := make(chan struct{})

	s.T().Run("execute", func(t *testing.T) {
		t.Parallel()

		err := merger.Execute(s.baseCtx)
		close(done)
		s.NoError(err)
	})

	s.T().Run("tick", func(t *testing.T) {
		t.Parallel()

		<-start
		trigger <- struct{}{}
		close(trigger)
		<-done

		s.Require().NoError(activeHeadContainer.Close())

		for shardID := range unloadedFS {
			s.Empty(unloadedFS[shardID].OpenCalls())
			s.Empty(unloadedFS[shardID].SyncCalls())
			s.Empty(unloadedFS[shardID].WriteCalls())

			s.Empty(queriedSeriesFS[shardID][0].OpenCalls())
			s.Empty(queriedSeriesFS[shardID][0].SeekCalls())
			s.Empty(queriedSeriesFS[shardID][0].SyncCalls())
			s.Empty(queriedSeriesFS[shardID][0].TruncateCalls())
			s.Empty(queriedSeriesFS[shardID][0].WriteCalls())

			s.Empty(queriedSeriesFS[shardID][1].OpenCalls())
			s.Empty(queriedSeriesFS[shardID][1].SeekCalls())
			s.Empty(queriedSeriesFS[shardID][1].SyncCalls())
			s.Empty(queriedSeriesFS[shardID][1].TruncateCalls())
			s.Empty(queriedSeriesFS[shardID][1].WriteCalls())
		}
	})
}
