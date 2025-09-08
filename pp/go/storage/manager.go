package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/jonboulle/clockwork"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/head/container"
	"github.com/prometheus/prometheus/pp/go/storage/head/services"
	"github.com/prometheus/prometheus/pp/go/storage/logger"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/pp/go/util"
)

const (
	// defaultCommitWaitInterval the minimum interval that the head must exist in order to perform operations on it.
	defaultCommitWaitInterval = 5 * time.Minute
)

type Manager struct {
	g               run.Group
	closer          *util.Closer
	rotatorConfig   *services.RotatorConfig
	rotatorMediator *NoopMediator
}

func NewManager(
	l log.Logger,
	clock clockwork.Clock,
	dataDir string,
	hcatalog *catalog.Catalog,
	blockDuration time.Duration,
	maxSegmentSize uint32,
	numberOfShards uint16,
	r prometheus.Registerer,
) (*Manager, error) {
	dirStat, err := os.Stat(dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to stat dir: %w", err)
	}

	if !dirStat.IsDir() {
		return nil, fmt.Errorf("%s is not directory", dataDir)
	}

	initLogHandler(l)

	builder := NewBuilder(
		hcatalog,
		dataDir,
		maxSegmentSize,
		r,
	)

	loader := NewLoader(
		dataDir,
		maxSegmentSize,
		r,
	)

	h, err := uploadOrBuildHead(
		clock,
		hcatalog,
		builder,
		loader,
		blockDuration,
		numberOfShards,
	)
	if err != nil {
		return nil, err
	}

	if _, err = hcatalog.SetStatus(h.ID(), catalog.StatusActive); err != nil {
		return nil, errors.Join(fmt.Errorf("failed to set active status: %w", err), h.Close())
	}

	// TODO Need close
	activeHead := container.NewWeighted(h)

	// TODO implements
	headKeeper := &NoopKeeper{}

	m := &Manager{
		g:      run.Group{},
		closer: util.NewCloser(),
	}

	baseCtx := context.Background()

	// Termination handler.
	m.g.Add(
		func() error {
			<-m.closer.Signal()

			return nil
		},
		func(error) {
			_ = m.closer.Close()
		},
	)

	// Rotator
	m.rotatorConfig = services.NewRotatorConfig(numberOfShards)
	m.rotatorMediator = &NoopMediator{c: make(chan struct{})}
	rotatorCtx, rotatorCancel := context.WithCancel(baseCtx)
	m.g.Add(
		func() error {
			return services.NewRotator(
				activeHead,
				builder,
				headKeeper,
				m.rotatorMediator,
				m.rotatorConfig,
				&headStatusSetter{catalog: hcatalog},
				r,
			).Execute(rotatorCtx)
		},
		func(error) {
			m.rotatorMediator.Close()
			rotatorCancel()
		},
	)

	isNewHead := func(headID string) bool {
		rec, err := hcatalog.Get(headID)
		if err != nil {
			return true
		}

		return clock.Now().Add(-defaultCommitWaitInterval).UnixMilli() < rec.CreatedAt()
	}

	// Committer
	committerMediator := &NoopMediator{c: make(chan struct{})}
	committerCtx, committerCancel := context.WithCancel(baseCtx)
	m.g.Add(
		func() error {
			return services.NewCommitter(activeHead, committerMediator, isNewHead).Execute(committerCtx)
		},
		func(error) {
			committerMediator.Close()
			committerCancel()
		},
	)

	// Merger
	mergerMediator := &NoopMediator{c: make(chan struct{})}
	mergerCtx, mergerCancel := context.WithCancel(baseCtx)
	m.g.Add(
		func() error {
			return services.NewMerger(activeHead, mergerMediator, isNewHead).Execute(mergerCtx)
		},
		func(error) {
			mergerMediator.Close()
			mergerCancel()
		},
	)

	// MetricsUpdater
	metricsUpdaterMediator := &NoopMediator{c: make(chan struct{})}
	metricsUpdaterCtx, metricsUpdaterCancel := context.WithCancel(baseCtx)
	m.g.Add(
		func() error {
			return services.NewMetricsUpdater(
				activeHead,
				headKeeper,
				metricsUpdaterMediator,
				querier.QueryHeadStatus,
				r,
			).Execute(metricsUpdaterCtx)
		},
		func(error) {
			metricsUpdaterMediator.Close()
			metricsUpdaterCancel()
		},
	)

	return m, nil
}

// TODO implementation.
func (m *Manager) Run() error {
	defer m.closer.Done()

	return m.g.Run()
}

// TODO implementation.
func (m *Manager) Shutdown(ctx context.Context) {
	_ = m.closer.Close()
}

// initLogHandler init log handler for pp.
func initLogHandler(l log.Logger) {
	l = log.With(l, "pp_caller", log.Caller(4))

	logger.Debugf = func(template string, args ...any) {
		_ = level.Debug(l).Log("msg", fmt.Sprintf(template, args...))
	}

	logger.Infof = func(template string, args ...any) {
		_ = level.Info(l).Log("msg", fmt.Sprintf(template, args...))
	}

	logger.Warnf = func(template string, args ...any) {
		_ = level.Warn(l).Log("msg", fmt.Sprintf(template, args...))
	}

	logger.Errorf = func(template string, args ...any) {
		_ = level.Error(l).Log("msg", fmt.Sprintf(template, args...))
	}
}

//
// NoopKeeper
//

// NoopKeeper implements Keeper.
type NoopKeeper struct{}

// Add implements Keeper.
func (*NoopKeeper) Add(*HeadOnDisk) {}

// RangeQueriableHeads implements Keeper.
func (k *NoopKeeper) RangeQueriableHeads(
	mint, maxt int64,
) func(func(*HeadOnDisk) bool) {
	return func(func(*HeadOnDisk) bool) {}
}

//
// NoopMediator
//

// NoopMediator implements Mediator.
type NoopMediator struct {
	c         chan struct{}
	closeOnce sync.Once
}

// C implements Mediator.
func (m *NoopMediator) C() <-chan struct{} {
	return m.c
}

// Close close channel and stop [Mediator].
func (m *NoopMediator) Close() {
	m.closeOnce.Do(func() {
		close(m.c)
	})
}

//
//
//

type headStatusSetter struct {
	catalog *catalog.Catalog
}

// SetActiveStatus sets the [catalog.StatusActive] status by headID.
func (ha *headStatusSetter) SetActiveStatus(headID string) error {
	_, err := ha.catalog.SetStatus(headID, catalog.StatusActive)
	return err
}

// SetRotatedStatus sets the [catalog.StatusRotated] status by headID.
func (ha *headStatusSetter) SetRotatedStatus(headID string) error {
	_, err := ha.catalog.SetStatus(headID, catalog.StatusRotated)
	return err
}
