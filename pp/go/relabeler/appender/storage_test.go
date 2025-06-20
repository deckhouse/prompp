package appender

import (
	"context"
	"fmt"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/block"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/relabeler/querier"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type blockWriterMock struct {
}

func (blockWriterMock) Write(block block.Block) error {
	panic("must not be called")
}

type headMock struct {
	id          string
	generation  uint64
	flushFunc   func() error
	RotateFunc  func() error
	maxTime     int64
	writeToFunc func(writer relabeler.BlockWriter) error
}

func (h *headMock) ID() string {
	return h.id
}

func (h *headMock) Generation() uint64 {
	return h.generation
}

func (h *headMock) Append(ctx context.Context, incomingData *relabeler.IncomingData, state *cppbridge.State, relabelerID string, commitToWal bool) ([][]*cppbridge.InnerSeries, cppbridge.RelabelerStats, error) {
	panic("must not be called")
}

func (h *headMock) CommitToWal() error {
	panic("must not be called")
}

func (h *headMock) MergeOutOfOrderChunks() {
	panic("must not be called")
}

func (h *headMock) NumberOfShards() uint16 {
	panic("must not be called")
}

func (h *headMock) Stop() {
	panic("must not be called")
}

func (h *headMock) Flush() error {
	return h.flushFunc()
}

func (h *headMock) Reconfigure(inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16) error {
	panic("must not be called")
}

func (h *headMock) WriteMetrics() {
	panic("must not be called")
}

func (h *headMock) Status(limit int) relabeler.HeadStatus {
	return relabeler.HeadStatus{
		HeadStats: relabeler.HeadStats{
			MaxTime: h.maxTime,
		},
	}
}

func (h *headMock) Rotate() error {
	return h.RotateFunc()
}

func (h *headMock) Close() error {
	return nil
}

func (h *headMock) Discard() error {
	return nil
}

func (h *headMock) String() string {
	return fmt.Sprintf("head {id: %s, generation: %d", h.id, h.generation)
}

func (h *headMock) CopySeriesFrom(other relabeler.Head) {
	panic("must not be called")
}

func (h *headMock) Enqueue(t *relabeler.GenericTask) {
	panic("must not be called")
}

func (h *headMock) CreateTask(taskName string, fn relabeler.ShardFn, isLss, isExclusive bool) *relabeler.GenericTask {
	panic("must not be called")
}

func (h *headMock) WriteTo(blockWriter relabeler.BlockWriter) error {
	return h.writeToFunc(blockWriter)
}

func TestQueryableStorage(t *testing.T) {
	clock := clockwork.NewFakeClock()
	s := NewQueryableStorageWithWriteNotifier(
		blockWriterMock{},
		prometheus.DefaultRegisterer,
		&querier.Metrics{},
		noOpWriteNotifier{},
		clock,
		time.Minute,
		time.Minute,
		time.Hour,
		time.Minute*2,
		1,
	)

	s.Run()
	defer s.Close()

	flushCalled := make(chan struct{})
	rotateCalled := make(chan struct{})
	writeToCalled := make(chan struct{})
	h := &headMock{
		id:         "test_head_id",
		generation: 0,
		flushFunc: func() error {
			flushCalled <- struct{}{}
			return nil
		},
		RotateFunc: func() error {
			rotateCalled <- struct{}{}
			return nil
		},
		maxTime: 0,
		writeToFunc: func(writer relabeler.BlockWriter) error {
			writeToCalled <- struct{}{}
			return nil
		},
	}

	s.Add(h)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	require.NoError(t, clock.BlockUntilContext(ctx, 1))
	clock.Advance(time.Minute)

	select {
	case <-flushCalled:
		t.FailNow()
	default:
	}

	select {
	case <-rotateCalled:
		t.FailNow()
	default:
	}

	select {
	case <-writeToCalled:
		t.FailNow()
	default:
	}

	clock.Advance(time.Minute)
	<-time.After(time.Minute)

	select {
	case <-flushCalled:
	case <-time.After(time.Second):
		t.Fail()
	}

	select {
	case <-rotateCalled:
	case <-time.After(time.Second):
		t.Fail()
	}

	select {
	case <-writeToCalled:
	case <-time.After(time.Second):
		t.Fail()
	}
}
