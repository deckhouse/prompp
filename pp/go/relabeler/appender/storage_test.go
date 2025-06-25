package appender

import (
	"context"
	"errors"
	"fmt"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/block"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
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
	closeFunc   func() error
	discardFunc func() error
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
	return h.closeFunc()
}

func (h *headMock) Discard() error {
	return h.discardFunc()
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

func TestQueryableStorage_Success(t *testing.T) {
	logger.Debugf = func(s string, i ...interface{}) {
		t.Logf(s, i...)
	}

	clock := clockwork.NewFakeClockAt(time.Time{})

	processingInterval := DefaultProcessingInterval
	retentionDuration := time.Hour
	afterConversionRetentionDuration := processingInterval * 2

	s := NewQueryableStorageWithWriteNotifier(
		blockWriterMock{},
		prometheus.DefaultRegisterer,
		&querier.Metrics{},
		noOpWriteNotifier{},
		clock,
		DefaultInitialDelay,
		processingInterval,
		retentionDuration,
		afterConversionRetentionDuration,
		1,
	)

	s.Run()
	defer s.Close()

	flushCalled := make(chan struct{})
	rotateCalled := make(chan struct{})
	writeToCalled := make(chan struct{})
	closeCalled := make(chan struct{})
	discardCalled := make(chan struct{})

	h := &headMock{
		id:         "test_head_id",
		generation: 0,
		flushFunc: func() error {
			logger.Debugf("flush called")
			close(flushCalled)
			return nil
		},
		RotateFunc: func() error {
			logger.Debugf("rotate called")
			close(rotateCalled)
			return nil
		},
		maxTime: clock.Now().UnixMilli(),
		writeToFunc: func(writer relabeler.BlockWriter) error {
			logger.Debugf("writeto called")
			close(writeToCalled)
			return nil
		},
		closeFunc: func() error {
			logger.Debugf("close called")
			close(closeCalled)
			return nil
		},
		discardFunc: func() error {
			logger.Debugf("discard called")
			close(discardCalled)
			return nil
		},
	}

	requireChannelIsNotClosedf(t, flushCalled, "flush must be not called by this time")
	requireChannelIsNotClosedf(t, rotateCalled, "rotate must be not called by this time")
	requireChannelIsNotClosedf(t, writeToCalled, "writeto must be not called by this time")
	requireChannelIsNotClosedf(t, closeCalled, "close must be not called by this time")
	requireChannelIsNotClosedf(t, discardCalled, "discard must be not called by this time")

	wh := s.NewWritableHead(h)
	processed, converted := s.ProcessHead(wh, false)
	require.False(t, processed)
	require.True(t, converted)

	requireChannelIsClosedf(t, flushCalled, "flush should be called at this time")
	requireChannelIsClosedf(t, rotateCalled, "rotate should be called at this time")
	requireChannelIsClosedf(t, writeToCalled, "writeto should be called at this time")
	requireChannelIsNotClosedf(t, closeCalled, "close must be not called by this time")
	requireChannelIsNotClosedf(t, discardCalled, "discard must be not called by this time")

	clock.Advance(afterConversionRetentionDuration)

	processed, converted = s.ProcessHead(wh, false)
	require.False(t, processed)
	require.False(t, converted)

	requireChannelIsClosedf(t, flushCalled, "flush should be called at this time")
	requireChannelIsClosedf(t, rotateCalled, "rotate should be called at this time")
	requireChannelIsClosedf(t, writeToCalled, "writeto should be called at this time")
	requireChannelIsNotClosedf(t, closeCalled, "close must be not called by this time")
	requireChannelIsNotClosedf(t, discardCalled, "discard must be not called by this time")

	clock.Advance(time.Duration(1))

	processed, converted = s.ProcessHead(wh, false)
	require.True(t, processed)
	require.False(t, converted)

	requireChannelIsClosedf(t, flushCalled, "flush should be called at this time")
	requireChannelIsClosedf(t, rotateCalled, "rotate should be called at this time")
	requireChannelIsClosedf(t, writeToCalled, "writeto should be called at this time")
	requireChannelIsClosedf(t, closeCalled, "close should be called at this time")
	requireChannelIsClosedf(t, discardCalled, "discard should be called at this time")
}

func TestQueryableStorage_Outdated(t *testing.T) {
	logger.Debugf = func(s string, i ...interface{}) {
		t.Logf(s, i...)
	}

	clock := clockwork.NewFakeClockAt(time.Time{})

	processingInterval := DefaultProcessingInterval
	retentionDuration := processingInterval * 5
	afterConversionRetentionDuration := processingInterval * 2

	s := NewQueryableStorageWithWriteNotifier(
		blockWriterMock{},
		prometheus.DefaultRegisterer,
		&querier.Metrics{},
		noOpWriteNotifier{},
		clock,
		DefaultInitialDelay,
		processingInterval,
		retentionDuration,
		afterConversionRetentionDuration,
		1,
	)

	s.Run()
	defer s.Close()

	flushCalled := make(chan struct{})
	rotateCalled := make(chan struct{})
	writeToCalled := make(chan struct{})
	closeCalled := make(chan struct{})
	discardCalled := make(chan struct{})

	h := &headMock{
		id:         "test_head_id",
		generation: 0,
		flushFunc: func() error {
			logger.Debugf("flush called")
			return errors.New("flush error")
		},
		RotateFunc: func() error {
			logger.Debugf("rotate called")
			close(rotateCalled)
			return nil
		},
		maxTime: clock.Now().UnixMilli(),
		writeToFunc: func(writer relabeler.BlockWriter) error {
			logger.Debugf("writeto called")
			close(writeToCalled)
			return nil
		},
		closeFunc: func() error {
			logger.Debugf("close called")
			close(closeCalled)
			return nil
		},
		discardFunc: func() error {
			logger.Debugf("discard called")
			close(discardCalled)
			return nil
		},
	}

	requireChannelIsNotClosedf(t, flushCalled, "flush must be not called by this time")
	requireChannelIsNotClosedf(t, rotateCalled, "rotate must be not called by this time")
	requireChannelIsNotClosedf(t, writeToCalled, "writeto must be not called by this time")
	requireChannelIsNotClosedf(t, closeCalled, "close must be not called by this time")
	requireChannelIsNotClosedf(t, discardCalled, "discard must be not called by this time")

	wh := s.NewWritableHead(h)
	processed, converted := s.ProcessHead(wh, false)
	require.False(t, processed)
	require.False(t, converted)

	for i := time.Duration(0); i < retentionDuration+processingInterval; i += processingInterval {
		clock.Advance(processingInterval)
		processed, converted = s.ProcessHead(wh, false)
		if i < retentionDuration {
			require.False(t, processed)
			require.False(t, converted)
			continue
		}
		break
	}

	require.True(t, processed)
	require.False(t, converted)

	requireChannelIsNotClosedf(t, flushCalled, "flush must be not called by this time")
	requireChannelIsClosedf(t, rotateCalled, "rotate should be called at this time")
	requireChannelIsClosedf(t, closeCalled, "close should be called at this time")
	requireChannelIsClosedf(t, discardCalled, "discard should be called at this time")
}

func TestQueryableStorage_Displaced(t *testing.T) {
	logger.Debugf = func(s string, i ...interface{}) {
		t.Logf(s, i...)
	}

	clock := clockwork.NewFakeClockAt(time.Time{})

	processingInterval := DefaultProcessingInterval
	retentionDuration := processingInterval * 5
	afterConversionRetentionDuration := processingInterval * 2

	s := NewQueryableStorageWithWriteNotifier(
		blockWriterMock{},
		prometheus.DefaultRegisterer,
		&querier.Metrics{},
		noOpWriteNotifier{},
		clock,
		DefaultInitialDelay,
		processingInterval,
		retentionDuration,
		afterConversionRetentionDuration,
		1,
	)

	s.Run()
	defer s.Close()

	flushCalled := make(chan struct{})
	rotateCalled := make(chan struct{})
	writeToCalled := make(chan struct{})
	closeCalled := make(chan struct{})
	discardCalled := make(chan struct{})

	h := &headMock{
		id:         "test_head_id",
		generation: 0,
		flushFunc: func() error {
			logger.Debugf("flush called")
			close(flushCalled)
			return nil
		},
		RotateFunc: func() error {
			logger.Debugf("rotate called")
			close(rotateCalled)
			return nil
		},
		maxTime: clock.Now().UnixMilli(),
		writeToFunc: func(writer relabeler.BlockWriter) error {
			logger.Debugf("writeto called")
			close(writeToCalled)
			return nil
		},
		closeFunc: func() error {
			logger.Debugf("close called")
			close(closeCalled)
			return nil
		},
		discardFunc: func() error {
			logger.Debugf("discard called")
			close(discardCalled)
			return nil
		},
	}

	requireChannelIsNotClosedf(t, flushCalled, "flush must be not called by this time")
	requireChannelIsNotClosedf(t, rotateCalled, "rotate must be not called by this time")
	requireChannelIsNotClosedf(t, writeToCalled, "writeto must be not called by this time")
	requireChannelIsNotClosedf(t, closeCalled, "close must be not called by this time")
	requireChannelIsNotClosedf(t, discardCalled, "discard must be not called by this time")

	wh := s.NewWritableHead(h)
	processed, converted := s.ProcessHead(wh, true)
	require.True(t, processed)
	require.False(t, converted)

	requireChannelIsClosedf(t, flushCalled, "flush should be called at this time")
	requireChannelIsClosedf(t, rotateCalled, "rotate should be called at this time")
	requireChannelIsClosedf(t, closeCalled, "close should be called at this time")
	requireChannelIsClosedf(t, discardCalled, "discard should be called at this time")
}

func requireChannelIsClosedf(t *testing.T, c chan struct{}, format string, args ...interface{}) {
	select {
	case _, ok := <-c:
		if !ok {
			return
		}
	default:
	}

	t.Fatalf(format, args...)
}

func requireChannelIsNotClosedf(t *testing.T, c chan struct{}, format string, args ...interface{}) {
	select {
	case _, ok := <-c:
		if ok {
			return
		}
	default:
		return
	}

	t.Fatalf(format, args...)
}
