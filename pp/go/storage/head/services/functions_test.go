package services_test

import (
	"errors"
	"testing"

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
		shard.NewDataStorage(),
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
