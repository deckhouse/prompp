package services_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/head/container"
	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/head/keeper"
	"github.com/prometheus/prometheus/pp/go/storage/head/services"
	"github.com/prometheus/prometheus/pp/go/storage/head/services/mock"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
	"github.com/prometheus/prometheus/pp/go/storage/head/task"
)

type RotatorSuite struct {
	suite.Suite

	baseCtx context.Context
}

func TestRotatorSuite(t *testing.T) {
	suite.Run(t, new(RotatorSuite))
}

func (s *RotatorSuite) SetupSuite() {
	s.baseCtx = context.Background()
}

func (s *RotatorSuite) createHead(
	headID string,
	segmentWriters []*mock.SegmentWriterMock,
	generation uint64,
	numberOfShards uint16,
) *storage.Head {
	shards := make([]*shard.Shard, numberOfShards)
	for shardID, segmentWriter := range segmentWriters {
		shards[shardID] = s.createShardOnMemory(segmentWriter, maxSegmentSize, uint16(shardID))
	}

	return head.NewHead(
		headID,
		false,
		true,
		shards,
		shard.NewPerGoroutineShard[*storage.Wal],
		nil,
		generation,
		nil,
	)
}

func (*RotatorSuite) createShardOnMemory(
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
		wal.NewWal(shardWalEncoder, segmentWriter, maxSegmentSize, shardID, nil),
		shardID,
	)
}

func (*RotatorSuite) nameIDGenerator(n uint64) string {
	return fmt.Sprintf("test-head-id-%d", n)
}

func (*RotatorSuite) addLabelSetToHead(h *storage.Head) {
	for sd := range h.RangeShards() {
		sd.LSSWithRLock(func(target, _ *cppbridge.LabelSetStorage) error {
			target.FindOrEmplace(model.LabelSetFromMap(map[string]string{"_name__": fmt.Sprintf("test0%d", sd.ShardID())}))
			return nil
		})
	}
}

func (*RotatorSuite) getNumSeriesFromHead(h *storage.Head) uint32 {
	numSeries := uint32(0)
	for sd := range h.RangeShards() {
		status := cppbridge.NewHeadStatus()
		sd.LSSWithRLock(func(target, _ *cppbridge.LabelSetStorage) error {
			status.FromLSS(target, 0)
			return nil
		})
		numSeries += status.NumSeries
	}

	return numSeries
}

func (s *RotatorSuite) TestRotate() {
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
	removedHeadNotifier := &mock.RemovedHeadNotifierMock{NotifyFunc: func() {}}
	rotatedCounter := uint64(0)
	rotatedTrigger := func() { rotatedCounter++ }

	proxyHead := storage.NewProxy(
		container.NewWeighted(
			s.createHead(s.nameIDGenerator(rotatedCounter), segmentWriters, rotatedCounter, shardsCount),
			container.DefaultBackPressure,
		),
		keeper.NewKeeper[storage.Head](2, removedHeadNotifier),
		func(*storage.Head) error { return nil },
	)

	headBuilder := &mock.HeadBuilderMock[
		*task.Generic[*shard.PerGoroutineShard],
		*shard.Shard,
		*shard.PerGoroutineShard,
		*storage.Head,
	]{BuildFunc: func(generation uint64, numberOfShards uint16) (*storage.Head, error) {
		return s.createHead(s.nameIDGenerator(generation), segmentWriters, generation, numberOfShards), nil
	}}
	cfg := &mock.RotatorConfigMock{NumberOfShardsFunc: func() uint16 { return shardsCount }}
	headInformer := &mock.HeadInformerMock{
		SetActiveStatusFunc:  func(string) error { return nil },
		SetRotatedStatusFunc: func(string) error { return nil },
		CreatedAtFunc:        func(string) time.Duration { return time.Duration(time.Now().UnixMilli()) },
	}

	rotator := services.NewRotator(
		proxyHead,
		headBuilder,
		mediator,
		cfg,
		headInformer,
		head.CopyAddedSeries[*shard.Shard, *shard.PerGoroutineShard](shard.CopyAddedSeries),
		rotatedTrigger,
		nil,
	)

	aHead := proxyHead.Get()
	s.addLabelSetToHead(aHead)

	done := make(chan struct{})
	services.CopySeriesOnRotate = false

	s.T().Run("execute", func(t *testing.T) {
		t.Parallel()

		err := rotator.Execute(s.baseCtx)
		close(done)
		s.NoError(err)
	})

	s.T().Run("tick", func(t *testing.T) {
		t.Parallel()

		<-start
		trigger <- struct{}{}
		close(trigger)
		<-done

		s.Require().NoError(proxyHead.Close())

		s.Equal(uint64(1), rotatedCounter)
		aHead := proxyHead.Get()
		s.Equal(uint64(1), aHead.Generation())
		s.Equal(s.nameIDGenerator(rotatedCounter), aHead.ID())
		s.Equal(headInformer.SetActiveStatusCalls()[0].HeadID, aHead.ID())
		actualNumSeries := s.getNumSeriesFromHead(aHead)
		s.Equal(uint32(0), actualNumSeries)

		for _, segmentWriter := range segmentWriters {
			if !s.Len(segmentWriter.WriteCalls(), 1) {
				return
			}
			if !s.Len(segmentWriter.FlushCalls(), 1) {
				return
			}
			if !s.Len(segmentWriter.SyncCalls(), 1) {
				return
			}

			for _, call := range segmentWriter.WriteCalls() {
				s.Equal(uint32(0), call.Segment.Samples())
			}
		}

		rHeads := proxyHead.Heads()
		s.Len(rHeads, 1)
		s.Equal(s.nameIDGenerator(rotatedCounter-1), rHeads[0].ID())
		s.Equal(headInformer.SetRotatedStatusCalls()[0].HeadID, rHeads[0].ID())
		s.True(rHeads[0].IsReadOnly())
	})
}

func (s *RotatorSuite) TestCopySeriesOnRotate() {
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
	removedHeadNotifier := &mock.RemovedHeadNotifierMock{NotifyFunc: func() {}}
	rotatedCounter := uint64(0)
	rotatedTrigger := func() { rotatedCounter++ }

	proxyHead := storage.NewProxy(
		container.NewWeighted(
			s.createHead(s.nameIDGenerator(rotatedCounter), segmentWriters, rotatedCounter, shardsCount),
			container.DefaultBackPressure,
		),
		keeper.NewKeeper[storage.Head](2, removedHeadNotifier),
		func(*storage.Head) error { return nil },
	)

	headBuilder := &mock.HeadBuilderMock[
		*task.Generic[*shard.PerGoroutineShard],
		*shard.Shard,
		*shard.PerGoroutineShard,
		*storage.Head,
	]{BuildFunc: func(generation uint64, numberOfShards uint16) (*storage.Head, error) {
		return s.createHead(s.nameIDGenerator(generation), segmentWriters, generation, numberOfShards), nil
	}}
	cfg := &mock.RotatorConfigMock{NumberOfShardsFunc: func() uint16 { return shardsCount }}
	headInformer := &mock.HeadInformerMock{
		SetActiveStatusFunc:  func(string) error { return nil },
		SetRotatedStatusFunc: func(string) error { return nil },
		CreatedAtFunc:        func(string) time.Duration { return time.Duration(time.Now().UnixMilli()) },
	}

	rotator := services.NewRotator(
		proxyHead,
		headBuilder,
		mediator,
		cfg,
		headInformer,
		head.CopyAddedSeries[*shard.Shard, *shard.PerGoroutineShard](shard.CopyAddedSeries),
		rotatedTrigger,
		nil,
	)

	aHead := proxyHead.Get()
	s.addLabelSetToHead(aHead)
	expectedNumSeries := s.getNumSeriesFromHead(aHead)

	done := make(chan struct{})
	services.CopySeriesOnRotate = true

	s.T().Run("execute", func(t *testing.T) {
		t.Parallel()

		err := rotator.Execute(s.baseCtx)
		close(done)
		s.NoError(err)
	})

	s.T().Run("tick", func(t *testing.T) {
		t.Parallel()

		<-start
		trigger <- struct{}{}
		close(trigger)
		<-done

		s.Require().NoError(proxyHead.Close())

		s.Equal(uint64(1), rotatedCounter)
		aHead := proxyHead.Get()
		s.Equal(uint64(1), aHead.Generation())
		s.Equal(s.nameIDGenerator(rotatedCounter), aHead.ID())
		s.Equal(headInformer.SetActiveStatusCalls()[0].HeadID, aHead.ID())
		actualNumSeries := s.getNumSeriesFromHead(aHead)
		s.Equal(expectedNumSeries, actualNumSeries)

		for _, segmentWriter := range segmentWriters {
			if !s.Len(segmentWriter.WriteCalls(), 1) {
				return
			}
			if !s.Len(segmentWriter.FlushCalls(), 1) {
				return
			}
			if !s.Len(segmentWriter.SyncCalls(), 1) {
				return
			}

			for _, call := range segmentWriter.WriteCalls() {
				s.Equal(uint32(0), call.Segment.Samples())
			}
		}

		rHeads := proxyHead.Heads()
		s.Len(rHeads, 1)
		s.Equal(s.nameIDGenerator(rotatedCounter-1), rHeads[0].ID())
		s.Equal(headInformer.SetRotatedStatusCalls()[0].HeadID, rHeads[0].ID())
		s.True(rHeads[0].IsReadOnly())
	})
}
