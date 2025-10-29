package head_test

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/storage/head/head"
)

type HeadSuite struct {
	suite.Suite

	id         string
	generation uint64
}

func TestHeadSuite(t *testing.T) {
	suite.Run(t, new(HeadSuite))
}

func (s *HeadSuite) SetupSuite() {
	s.id = "test-head-id"
	s.generation = uint64(42)
}

func (s *HeadSuite) TestClose() {
	sd := &ShardMock{
		ShardIDFunc: func() uint16 { return 0 },
		CloseFunc:   func() error { return nil },
	}

	closeCount := 0
	h := head.NewHead(
		s.id,
		false,
		true,
		[]*ShardMock{sd},
		newPerGoroutineShardMockfunc,
		func() { closeCount++ },
		s.generation,
		nil,
	)

	s.T().Log("first close head", h.String())
	err := h.Close()
	s.Require().NoError(err)

	s.Len(sd.CloseCalls(), 1)
	s.Equal(1, closeCount)

	s.T().Log("second close head", h.String())
	err = h.Close()
	s.Require().NoError(err)

	s.Len(sd.CloseCalls(), 1)
	s.Equal(1, closeCount)
}

func (s *HeadSuite) TestConcurrency() {
	sd := &ShardMock{
		ShardIDFunc: func() uint16 { return 0 },
		CloseFunc:   func() error { return nil },
	}

	h := head.NewHead(
		s.id,
		false,
		true,
		[]*ShardMock{sd, sd},
		newPerGoroutineShardMockfunc,
		nil,
		s.generation,
		nil,
	)
	defer h.Close()

	s.Equal(int64(4), h.Concurrency())
}

func (s *HeadSuite) TestEnqueue() {
	sd0 := &ShardMock{
		ShardIDFunc: func() uint16 { return 0 },
		CloseFunc:   func() error { return nil },
	}

	sd1 := &ShardMock{
		ShardIDFunc: func() uint16 { return 1 },
		CloseFunc:   func() error { return nil },
	}

	h := head.NewHead(
		s.id,
		false,
		true,
		[]*ShardMock{sd0, sd1},
		newPerGoroutineShardMockfunc,
		nil,
		s.generation,
		nil,
	)
	defer h.Close()

	shardsExecuted := uint32(0)
	t := h.CreateTask("test-task", func(shard *perGoroutineShardMock) error {
		atomic.AddUint32(&shardsExecuted, uint32(shard.ShardID()+1))
		return nil
	})

	h.Enqueue(t)

	err := t.Wait()
	s.Require().NoError(err)

	s.Equal(uint32(3), shardsExecuted)
}

func (s *HeadSuite) TestEnqueueOnShard() {
	sd0 := &ShardMock{
		ShardIDFunc: func() uint16 { return 0 },
		CloseFunc:   func() error { return nil },
	}

	sd1 := &ShardMock{
		ShardIDFunc: func() uint16 { return 1 },
		CloseFunc:   func() error { return nil },
	}

	h := head.NewHead(
		s.id,
		false,
		true,
		[]*ShardMock{sd0, sd1},
		newPerGoroutineShardMockfunc,
		nil,
		s.generation,
		nil,
	)
	defer h.Close()

	shardActual := uint16(1<<16 - 1)
	expectedShard := uint16(1)
	t := h.CreateTask("test-task", func(shard *perGoroutineShardMock) error {
		shardActual = shard.ShardID()
		return nil
	})

	h.EnqueueOnShard(t, expectedShard)

	err := t.Wait()
	s.Require().NoError(err)

	s.Equal(expectedShard, shardActual)

	expectedShard = uint16(0)
	t = h.CreateTask("test-task", func(shard *perGoroutineShardMock) error {
		shardActual = shard.ShardID()
		return nil
	})

	h.EnqueueOnShard(t, expectedShard)

	err = t.Wait()
	s.Require().NoError(err)

	s.Equal(expectedShard, shardActual)
}

func (s *HeadSuite) TestGeneration() {
	sd := &ShardMock{
		ShardIDFunc: func() uint16 { return 0 },
		CloseFunc:   func() error { return nil },
	}

	h := head.NewHead(
		s.id,
		false,
		true,
		[]*ShardMock{sd, sd},
		newPerGoroutineShardMockfunc,
		nil,
		s.generation,
		nil,
	)
	defer h.Close()

	s.Equal(s.generation, h.Generation())
}

func (s *HeadSuite) TestID() {
	sd := &ShardMock{
		ShardIDFunc: func() uint16 { return 0 },
		CloseFunc:   func() error { return nil },
	}

	h := head.NewHead(
		s.id,
		false,
		true,
		[]*ShardMock{sd, sd},
		newPerGoroutineShardMockfunc,
		nil,
		s.generation,
		nil,
	)
	defer h.Close()

	s.Equal(s.id, h.ID())
}

func (s *HeadSuite) TestIsReadOnly() {
	sd := &ShardMock{
		ShardIDFunc: func() uint16 { return 0 },
		CloseFunc:   func() error { return nil },
	}

	h := head.NewHead(
		s.id,
		false,
		true,
		[]*ShardMock{sd, sd},
		newPerGoroutineShardMockfunc,
		nil,
		s.generation,
		nil,
	)
	defer h.Close()

	s.False(h.IsReadOnly())

	h.SetReadOnly()
	s.True(h.IsReadOnly())
}

func (s *HeadSuite) TestNumberOfShards() {
	sd := &ShardMock{
		ShardIDFunc: func() uint16 { return 0 },
		CloseFunc:   func() error { return nil },
	}

	h := head.NewHead(
		s.id,
		false,
		true,
		[]*ShardMock{sd, sd, sd},
		newPerGoroutineShardMockfunc,
		nil,
		s.generation,
		nil,
	)
	defer h.Close()

	s.Equal(uint16(3), h.NumberOfShards())
}

func (s *HeadSuite) TestRangeQueueSize() {
	sd0 := &ShardMock{
		ShardIDFunc: func() uint16 { return 0 },
		CloseFunc:   func() error { return nil },
	}

	sd1 := &ShardMock{
		ShardIDFunc: func() uint16 { return 1 },
		CloseFunc:   func() error { return nil },
	}

	h := head.NewHead(
		s.id,
		false,
		true,
		[]*ShardMock{sd0, sd1},
		newPerGoroutineShardMockfunc,
		nil,
		s.generation,
		nil,
	)
	defer h.Close()

	execute := sync.WaitGroup{}
	execute.Add(4)

	done := make(chan struct{})
	t1 := h.CreateTask("test-task", func(_ *perGoroutineShardMock) error {
		execute.Done()
		<-done
		return nil
	})
	h.Enqueue(t1)

	t2 := h.CreateTask("test-task", func(_ *perGoroutineShardMock) error {
		execute.Done()
		<-done
		return nil
	})
	h.Enqueue(t2)

	execute.Wait()

	t3 := h.CreateTask("test-task", func(_ *perGoroutineShardMock) error {
		<-done
		return nil
	})
	h.Enqueue(t3)

	expectedShardID := 0
	for shardID, size := range h.RangeQueueSize() {
		s.Equal(expectedShardID, shardID)
		s.Equal(1, size)
		expectedShardID++
	}

	close(done)

	err := t1.Wait()
	s.Require().NoError(err)

	err = t2.Wait()
	s.Require().NoError(err)

	err = t2.Wait()
	s.Require().NoError(err)
}

func (s *HeadSuite) TestRangeShards() {
	sd0 := &ShardMock{
		ShardIDFunc: func() uint16 { return 0 },
		CloseFunc:   func() error { return nil },
	}

	sd1 := &ShardMock{
		ShardIDFunc: func() uint16 { return 1 },
		CloseFunc:   func() error { return nil },
	}

	h := head.NewHead(
		s.id,
		false,
		true,
		[]*ShardMock{sd0, sd1},
		newPerGoroutineShardMockfunc,
		nil,
		s.generation,
		nil,
	)
	defer h.Close()

	expectedShardID := uint16(0)
	for shard := range h.RangeShards() {
		s.Equal(expectedShardID, shard.ShardID())
		expectedShardID++
	}
}

//
// perGoroutineShardMock
//

// perGoroutineShardMock mock for [PerGoroutineShard].
type perGoroutineShardMock struct {
	*ShardMock
}

// newPerGoroutineShardMockfunc constructor for [PerGoroutineShard].
func newPerGoroutineShardMockfunc(sd *ShardMock, _ uint16) *perGoroutineShardMock {
	return &perGoroutineShardMock{ShardMock: sd}
}
