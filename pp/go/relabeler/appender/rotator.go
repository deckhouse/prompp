package appender

import (
	"context"
	"time"

	"github.com/jonboulle/clockwork"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/prometheus/pp/go/util"
)

// DefaultRotateDuration - default block duration.
const (
	DefaultRotateDuration = 2 * time.Hour
	DefaultCommitTimeout  = time.Second * 5
	DefaultMergeDuration  = 5 * time.Minute
)

// Rotatable is something that can be rotated.
type RotateCommitable interface {
	Rotate(ctx context.Context) error
	CommitToWal(ctx context.Context) error
	MergeOutOfOrderChunks(ctx context.Context)
}

type Timer interface {
	Chan() <-chan time.Time
	Reset()
	Stop()
}

// RotateCommiter is a rotation trigger.
type RotateCommiter struct {
	rotateCommitable RotateCommitable
	rotateTimer      Timer
	commitTimer      Timer
	mergeTimer       Timer
	run              chan struct{}
	closer           *util.Closer
	rotateCounter    prometheus.Counter
	rotateTimestamp  prometheus.Gauge
}

// NewRotateCommiter - Rotator constructor.
func NewRotateCommiter(
	ctx context.Context,
	rotateCommitable RotateCommitable,
	rotateTimer Timer,
	commitTimer Timer,
	mergeTimer Timer,
	registerer prometheus.Registerer,
) *RotateCommiter {
	factory := util.NewUnconflictRegisterer(registerer)
	r := &RotateCommiter{
		rotateCommitable: rotateCommitable,
		rotateTimer:      rotateTimer,
		commitTimer:      commitTimer,
		mergeTimer:       mergeTimer,
		run:              make(chan struct{}),
		closer:           util.NewCloser(),
		rotateCounter: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "prompp_rotator_rotate_count",
				Help: "Total counter of rotate rotatable object.",
			},
		),
		rotateTimestamp: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "prompp_rotator_rotate_timestamp",
				Help: "Timestamp in seconds of rotate rotatable object.",
			},
		),
	}
	go r.loop(ctx)

	return r
}

// Run - runs rotation loop.
func (r *RotateCommiter) Run() {
	close(r.run)
}

func (r *RotateCommiter) loop(ctx context.Context) {
	defer r.closer.Done()

	select {
	case <-r.run:
		r.rotateTimer.Reset()
		r.commitTimer.Reset()
		r.mergeTimer.Reset()

	case <-r.closer.Signal():
		return
	}

	for {
		select {
		case <-r.closer.Signal():
			return
		case <-r.commitTimer.Chan():
			if err := r.rotateCommitable.CommitToWal(ctx); err != nil {
				logger.Errorf("wal commit failed: %v", err)
			}
			r.commitTimer.Reset()

		case <-r.mergeTimer.Chan():
			r.rotateCommitable.MergeOutOfOrderChunks(ctx)
			r.mergeTimer.Reset()

		case <-r.rotateTimer.Chan():
			logger.Debugf("start rotation")

			if err := r.rotateCommitable.Rotate(ctx); err != nil {
				logger.Errorf("rotation failed: %v", err)
			}
			r.rotateCounter.Inc()
			r.rotateTimestamp.Set(float64(time.Now().UnixMilli()))

			r.rotateTimer.Reset()
			r.commitTimer.Reset()
			r.mergeTimer.Reset()
		}
	}
}

// Close - io.Closer interface implementation.
func (r *RotateCommiter) Close() error {
	return r.closer.Close()
}

type ConstantIntervalTimer struct {
	timer    clockwork.Timer
	interval time.Duration
}

func NewConstantIntervalTimer(clock clockwork.Clock, interval time.Duration) *ConstantIntervalTimer {
	return &ConstantIntervalTimer{
		timer:    clock.NewTimer(interval),
		interval: interval,
	}
}

func (t *ConstantIntervalTimer) Chan() <-chan time.Time {
	return t.timer.Chan()
}

func (t *ConstantIntervalTimer) Reset() {
	t.timer.Reset(t.interval)
}

func (t *ConstantIntervalTimer) Stop() {
	t.timer.Stop()
}
