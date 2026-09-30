package services_test

import (
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/head/services"
	"github.com/prometheus/prometheus/pp/go/storage/head/services/mock"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
)

type FunctionsSuite struct {
	suite.Suite
}

func TestFunctionsSuite(t *testing.T) {
	suite.Run(t, new(FunctionsSuite))
}

func (s *FunctionsSuite) newShard(
	segmentWriter *mock.SegmentWriterMock,
	shardID uint16,
) *shard.Shard {
	lss := shard.NewLSS()
	shardWalEncoder := cppbridge.NewHeadWalEncoder(shardID, 0, lss.Target())

	return shard.NewShard(
		lss,
		shard.NewDataStorage(false, false),
		nil,
		nil,
		wal.NewWal(shardWalEncoder, segmentWriter, lss, maxSegmentSize, shardID, nil),
		shardID,
	)
}

func (s *FunctionsSuite) newHead(segmentWriters []*mock.SegmentWriterMock) *storage.Head {
	shards := make([]*shard.Shard, len(segmentWriters))
	for shardID, sw := range segmentWriters {
		shards[shardID] = s.newShard(sw, uint16(shardID))
	}

	return head.NewHead(
		"close-wals-test-head",
		shards,
		shard.NewPerGoroutineShard[*storage.Wal],
		nil,
		0,
		nil,
	)
}

// cfsVariants are the commit-flush-sync functions, which must behave identically.
var cfsVariants = []struct {
	name string
	cfs  func(*storage.Head) error
}{
	{name: "CFSViaRange", cfs: services.CFSViaRange[*shard.Shard, *storage.Head]},
	{name: "LongCFSViaRange", cfs: services.LongCFSViaRange[*shard.Shard, *storage.Head]},
}

func (*FunctionsSuite) newCFSSegmentWriter(writeErr, flushErr error) *mock.SegmentWriterMock {
	return &mock.SegmentWriterMock{
		WriteFunc: func(*cppbridge.HeadEncodedSegment) error { return writeErr },
		FlushFunc: func() error { return flushErr },
		SyncFunc:  func() error { return nil },
	}
}

func (s *FunctionsSuite) TestCFSViaRangeCommitsFlushesAndSyncsEveryShard() {
	for _, variant := range cfsVariants {
		s.Run(variant.name, func() {
			segmentWriters := make([]*mock.SegmentWriterMock, shardsCount)
			for shardID := range shardsCount {
				segmentWriters[shardID] = s.newCFSSegmentWriter(nil, nil)
			}
			h := s.newHead(segmentWriters)

			s.Require().NoError(variant.cfs(h))

			for shardID, sw := range segmentWriters {
				s.Lenf(sw.WriteCalls(), 1, "shard %d", shardID)
				s.Lenf(sw.FlushCalls(), 1, "shard %d", shardID)
				s.Lenf(sw.SyncCalls(), 1, "shard %d", shardID)
			}
		})
	}
}

func (s *FunctionsSuite) TestCFSViaRangeFlushesAndSyncsAfterCommitError() {
	for _, variant := range cfsVariants {
		s.Run(variant.name, func() {
			commitErr := errors.New("shard 0 write failed")
			segmentWriters := []*mock.SegmentWriterMock{
				s.newCFSSegmentWriter(commitErr, nil),
				s.newCFSSegmentWriter(nil, nil),
			}
			h := s.newHead(segmentWriters)

			s.Require().ErrorIs(variant.cfs(h), commitErr)

			// A failed commit must not prevent flushing and syncing already written data.
			for shardID, sw := range segmentWriters {
				s.Lenf(sw.WriteCalls(), 1, "shard %d", shardID)
				s.Lenf(sw.FlushCalls(), 1, "shard %d", shardID)
				s.Lenf(sw.SyncCalls(), 1, "shard %d", shardID)
			}
		})
	}
}

func (s *FunctionsSuite) TestCFSViaRangeSkipsSyncOnFlushError() {
	for _, variant := range cfsVariants {
		s.Run(variant.name, func() {
			flushErr := errors.New("shard 0 flush failed")
			segmentWriters := []*mock.SegmentWriterMock{
				s.newCFSSegmentWriter(nil, flushErr),
				s.newCFSSegmentWriter(nil, nil),
			}
			h := s.newHead(segmentWriters)

			s.Require().ErrorIs(variant.cfs(h), flushErr)

			s.Empty(segmentWriters[0].SyncCalls(), "sync must be skipped for the shard with failed flush")
			s.Len(segmentWriters[1].SyncCalls(), 1, "other shards must still be synced")
		})
	}
}

func (s *FunctionsSuite) TestLongCFSViaRangeProcessesShardsConcurrently() {
	// concurrency is limited by GOMAXPROCS, so it must allow all shards at once
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(shardsCount))

	entered := make(chan struct{}, shardsCount)
	release := make(chan struct{})
	segmentWriters := make([]*mock.SegmentWriterMock, shardsCount)
	for shardID := range shardsCount {
		segmentWriters[shardID] = s.newCFSSegmentWriter(nil, nil)
		// every shard blocks in write until all shards have entered it, so a sequential run never completes
		segmentWriters[shardID].WriteFunc = func(*cppbridge.HeadEncodedSegment) error {
			entered <- struct{}{}
			<-release
			return nil
		}
	}
	h := s.newHead(segmentWriters)

	done := make(chan error, 1)
	go func() { done <- services.LongCFSViaRange(h) }()

	timeout := time.After(5 * time.Second)
	for range shardsCount {
		select {
		case <-entered:
		case <-timeout:
			close(release)
			s.FailNow("shards are not processed concurrently")
		}
	}
	close(release)

	s.Require().NoError(<-done)
}

func (s *FunctionsSuite) TestLongCFSViaRangeLimitsConcurrencyByGOMAXPROCS() {
	const limit = 1
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(limit))

	var (
		mtx         sync.Mutex
		inFlight    int
		maxInFlight int
	)
	segmentWriters := make([]*mock.SegmentWriterMock, shardsCount)
	for shardID := range shardsCount {
		segmentWriters[shardID] = s.newCFSSegmentWriter(nil, nil)
		segmentWriters[shardID].WriteFunc = func(*cppbridge.HeadEncodedSegment) error {
			mtx.Lock()
			inFlight++
			maxInFlight = max(maxInFlight, inFlight)
			mtx.Unlock()

			// give the other shards a chance to enter write if the limit is not respected
			time.Sleep(10 * time.Millisecond)

			mtx.Lock()
			inFlight--
			mtx.Unlock()
			return nil
		}
	}
	h := s.newHead(segmentWriters)

	s.Require().NoError(services.LongCFSViaRange(h))

	s.Equal(limit, maxInFlight)
}

func (s *FunctionsSuite) TestCloseWalsClosesEveryShardWal() {
	segmentWriters := make([]*mock.SegmentWriterMock, shardsCount)
	for shardID := range shardsCount {
		segmentWriters[shardID] = &mock.SegmentWriterMock{
			CloseFunc: func() error { return nil },
		}
	}
	h := s.newHead(segmentWriters)

	s.Require().NoError(services.CloseWals(h))

	for shardID, sw := range segmentWriters {
		s.Lenf(sw.CloseCalls(), 1, "shard %d", shardID)
	}
}

func (s *FunctionsSuite) TestCloseWalsIsIdempotent() {
	segmentWriters := make([]*mock.SegmentWriterMock, shardsCount)
	for shardID := range shardsCount {
		segmentWriters[shardID] = &mock.SegmentWriterMock{
			CloseFunc: func() error { return nil },
		}
	}
	h := s.newHead(segmentWriters)

	s.Require().NoError(services.CloseWals(h))
	s.Require().NoError(services.CloseWals(h))

	for shardID, sw := range segmentWriters {
		s.Lenf(sw.CloseCalls(), 1, "shard %d", shardID)
	}
}

func (s *FunctionsSuite) TestCloseWalsAggregatesErrorsFromAllShards() {
	firstErr := errors.New("shard 0 close failed")
	secondErr := errors.New("shard 1 close failed")
	segmentWriters := []*mock.SegmentWriterMock{
		{CloseFunc: func() error { return firstErr }},
		{CloseFunc: func() error { return secondErr }},
	}
	h := s.newHead(segmentWriters)

	err := services.CloseWals(h)
	s.Require().Error(err)
	s.Require().ErrorIs(err, firstErr)
	s.Require().ErrorIs(err, secondErr)

	// All shards must have been visited despite earlier errors.
	for shardID, sw := range segmentWriters {
		s.Lenf(sw.CloseCalls(), 1, "shard %d", shardID)
	}
}
