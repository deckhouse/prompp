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

	baseCtx             context.Context
	segmentWriter       *mock.SegmentWriterMock
	activeHeadContainer *container.Weighted[storage.Head, *storage.Head]
}

func TestMergerSuite(t *testing.T) {
	suite.Run(t, new(MergerSuite))
}

func (s *MergerSuite) SetupSuite() {
	s.baseCtx = context.Background()
}

func (s *MergerSuite) SetupTest() {
	s.activeHeadContainer = container.NewWeighted(s.createHead())
}

func (s *MergerSuite) createHead() *storage.Head {
	shards := make([]*shard.Shard, shardsCount)
	for shardID := range shardsCount {
		shards[shardID] = s.createShardOnMemory(maxSegmentSize, uint16(shardID))
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

func (s *MergerSuite) createShardOnMemory(maxSegmentSize uint32, shardID uint16) *shard.Shard {
	s.segmentWriter = &mock.SegmentWriterMock{
		WriteFunc:       func(*cppbridge.HeadEncodedSegment) error { return nil },
		FlushFunc:       func() error { return nil },
		SyncFunc:        func() error { return nil },
		CloseFunc:       func() error { return nil },
		CurrentSizeFunc: func() int64 { return 0 },
	}

	lss := shard.NewLSS()
	shardWalEncoder := cppbridge.NewHeadWalEncoder(shardID, 0, lss.Target())

	return shard.NewShard(
		lss,
		shard.NewDataStorage(),
		nil,
		nil,
		wal.NewWal(shardWalEncoder, s.segmentWriter, maxSegmentSize),
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
	isNewHead := func(string) bool { return false }
	merger := services.NewMerger(s.activeHeadContainer, mediator, isNewHead)
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

		s.Require().NoError(s.activeHeadContainer.Close())
	})
}

func (s *MergerSuite) TestSkipNewHead() {
	s.T().Log("TestSkipNewHead")
}
