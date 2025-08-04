package manager

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/logger"
	"github.com/prometheus/prometheus/pp/go/util"
)

type Timer interface {
	Chan() <-chan time.Time
	Reset()
	Stop()
}

type ActiveHeadContainer interface {
	Get(ctx context.Context) (storage.Head, error)
	Replace(ctx context.Context, newHead storage.Head) error
	With(ctx context.Context, fn func(h storage.Head) error) error
}

// type ActiveHeadContainer[T any] interface {
// 	Get() *T
// 	Replace(ctx context.Context, newHead *T) error
// 	With(ctx context.Context, fn func(h *T) error) error
// }

// var _ ActiveHeadContainer[testHead] = (*container.Weighted[testHead, *testHead])(nil)

// HeadBuilder builder for the [storage.Head].
type HeadBuilder interface {
	// inputRelabelerConfigs []*config.InputRelabelerConfig,
	BuildWithConfig(numberOfShards uint16) (storage.Head, error)
}

type Manager struct {
	//
	activeHead  ActiveHeadContainer
	headBuilder HeadBuilder
	rotateTimer Timer
	commitTimer Timer
	mergeTimer  Timer
	// TODO closer vs shutdowner
	closer     *util.Closer
	shutdowner *util.GracefulShutdowner

	rotateCounter prometheus.Counter

	numberOfShards uint16
}

// ApplyConfig update config.
func (m *Manager) ApplyConfig(
	ctx context.Context,
	inputRelabelerConfigs []*config.InputRelabelerConfig,
	numberOfShards uint16,
) error {
	logger.Infof("reconfiguration start")
	defer logger.Infof("reconfiguration completed")

	// TODO HeadConfigStorage

	return nil
}

// MergeOutOfOrderChunks merge chunks with out of order data chunks.
func (m *Manager) MergeOutOfOrderChunks(ctx context.Context) error {
	return m.activeHead.With(ctx, func(h storage.Head) error {
		h.MergeOutOfOrderChunks()

		return nil
	})
}

// Run starts processing of the [Manager].
// TODO implementation.
func (m *Manager) Run(ctx context.Context) error {
	go m.loop(ctx)
	return nil
}

// Shutdown safe shutdown [Manager].
func (m *Manager) Shutdown(ctx context.Context) error {
	return nil
}

// commitToWal commit the accumulated data into the wal.
func (m *Manager) commitToWal(ctx context.Context) error {
	return m.activeHead.With(ctx, func(h storage.Head) error {
		return h.CommitToWal()
	})
}

// TODO implementation.
func (m *Manager) loop(ctx context.Context) {
	defer m.closer.Done()

	for {
		select {
		case <-m.closer.Signal():
			return

		case <-m.commitTimer.Chan():
			if err := m.commitToWal(ctx); err != nil {
				logger.Errorf("wal commit failed: %v", err)
			}
			m.commitTimer.Reset()

		case <-m.mergeTimer.Chan():
			if err := m.MergeOutOfOrderChunks(ctx); err != nil {
				logger.Errorf("merge out of order chunks failed: %v", err)
			}
			m.mergeTimer.Reset()

		case <-m.rotateTimer.Chan():
			logger.Debugf("start rotation")

			if err := m.rotate(ctx); err != nil {
				logger.Errorf("rotation failed: %v", err)
			}
			m.rotateCounter.Inc()

			m.rotateTimer.Reset()
			m.commitTimer.Reset()
			m.mergeTimer.Reset()
		}
	}
}

func (m *Manager) rotate(ctx context.Context) error {
	newHead, err := m.headBuilder.BuildWithConfig(m.numberOfShards)
	if err != nil {
		return fmt.Errorf("failed to build a new head: %w", err)
	}

	oldHead, err := m.activeHead.Get(ctx)
	if err != nil {
		return fmt.Errorf("getting active head failed: %w", err)
	}

	newHead.CopySeriesFrom(oldHead)

	// storgae.Add(oldHead)

	// TODO if replace error?
	return m.activeHead.Replace(ctx, newHead)
}
