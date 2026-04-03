package services_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/head/container"
	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/head/services"
	"github.com/prometheus/prometheus/pp/go/storage/head/services/mock"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
)

type CommitterSuite struct {
	suite.Suite

	baseCtx context.Context
}

func TestCommitterSuite(t *testing.T) {
	suite.Run(t, new(CommitterSuite))
}

func (s *CommitterSuite) SetupSuite() {
	s.baseCtx = context.Background()
}

func (s *CommitterSuite) createHead(segmentWriters []*mock.SegmentWriterMock) *storage.Head {
	shards := make([]*shard.Shard, shardsCount)
	for shardID, segmentWriter := range segmentWriters {
		shards[shardID] = s.createShardOnMemory(segmentWriter, maxSegmentSize, uint16(shardID))
	}

	return head.NewHead(
		"test-head-id",
		shards,
		shard.NewPerGoroutineShard[*storage.Wal],
		nil,
		0,
		nil,
	)
}

func (*CommitterSuite) createShardOnMemory(
	segmentWriter *mock.SegmentWriterMock,
	maxSegmentSize uint32,
	shardID uint16,
) *shard.Shard {
	lss := shard.NewLSS()
	// logShards is 0 for single encoder
	shardWalEncoder := cppbridge.NewHeadWalEncoder(shardID, 0, lss.Target())

	return shard.NewShard(
		lss,
		shard.NewDataStorage(),
		nil,
		nil,
		wal.NewWal(shardWalEncoder, segmentWriter, lss, maxSegmentSize, shardID, nil),
		shardID,
	)
}

func (s *CommitterSuite) TestHappyPath() {
	trigger := make(chan struct{}, 1)
	start := make(chan struct{})
	mediator := &mock.MediatorMock{
		CFunc: func() <-chan struct{} {
			close(start)
			return trigger
		},
	}

	segmentWriters := make([]*mock.SegmentWriterMock, shardsCount)
	for shardID := range shardsCount {
		segmentWriters[shardID] = &mock.SegmentWriterMock{
			WriteFunc:       func(*cppbridge.HeadEncodedSegment) error { return nil },
			FlushFunc:       func() error { return nil },
			SyncFunc:        func() error { return nil },
			CloseFunc:       func() error { return nil },
			CurrentSizeFunc: func() int64 { return 0 },
		}
	}
	activeHeadContainer := container.NewWeighted(s.createHead(segmentWriters), container.DefaultBackPressure)
	isNewHead := func(string) bool { return false }

	committer := services.NewCommitter(activeHeadContainer, mediator, isNewHead)
	done := make(chan struct{})

	s.T().Run("execute", func(t *testing.T) {
		t.Parallel()

		err := committer.Execute(s.baseCtx)
		close(done)
		s.NoError(err)
	})

	s.T().Run("tick", func(t *testing.T) {
		t.Parallel()

		<-start
		trigger <- struct{}{}
		trigger <- struct{}{}
		close(trigger)
		<-done

		s.Require().NoError(activeHeadContainer.Close())

		for _, segmentWriter := range segmentWriters {
			if !s.Len(segmentWriter.WriteCalls(), 2) {
				return
			}
			if !s.Len(segmentWriter.FlushCalls(), 2) {
				return
			}
			if !s.Len(segmentWriter.SyncCalls(), 2) {
				return
			}

			for _, call := range segmentWriter.WriteCalls() {
				s.Equal(uint32(0), call.Segment.Samples())
			}
		}
	})
}

func (s *CommitterSuite) TestSkipNewHead() {
	trigger := make(chan struct{}, 1)
	start := make(chan struct{})
	mediator := &mock.MediatorMock{
		CFunc: func() <-chan struct{} {
			close(start)
			return trigger
		},
	}

	segmentWriters := make([]*mock.SegmentWriterMock, shardsCount)
	for shardID := range shardsCount {
		segmentWriters[shardID] = &mock.SegmentWriterMock{
			WriteFunc:       func(*cppbridge.HeadEncodedSegment) error { return nil },
			FlushFunc:       func() error { return nil },
			SyncFunc:        func() error { return nil },
			CloseFunc:       func() error { return nil },
			CurrentSizeFunc: func() int64 { return 0 },
		}
	}
	activeHeadContainer := container.NewWeighted(s.createHead(segmentWriters), container.DefaultBackPressure)

	isNewHead := func(string) bool { return true }
	committer := services.NewCommitter(activeHeadContainer, mediator, isNewHead)
	done := make(chan struct{})

	s.T().Run("execute", func(t *testing.T) {
		t.Parallel()

		err := committer.Execute(s.baseCtx)
		close(done)
		s.Require().NoError(err)
	})

	s.T().Run("tick", func(t *testing.T) {
		t.Parallel()

		<-start
		trigger <- struct{}{}
		trigger <- struct{}{}
		close(trigger)
		<-done

		s.Require().NoError(activeHeadContainer.Close())

		for _, segmentWriter := range segmentWriters {
			s.Empty(segmentWriter.WriteCalls())
			s.Empty(segmentWriter.FlushCalls())
			s.Empty(segmentWriter.SyncCalls())
		}
	})
}
