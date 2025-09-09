package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/jonboulle/clockwork"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/head/container"
	"github.com/prometheus/prometheus/pp/go/storage/head/proxy"
	"github.com/prometheus/prometheus/pp/go/storage/head/services"
	"github.com/prometheus/prometheus/pp/go/storage/logger"
	"github.com/prometheus/prometheus/pp/go/storage/mediator"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/pp/go/storage/ready"
	"github.com/prometheus/prometheus/pp/go/util"
)

const (
	// DefaultRotateDuration default block duration.
	DefaultRotateDuration = 2 * time.Hour

	// DefaultMergeDuration the default interval for the merge out of order chunks.
	DefaultMergeDuration = 5 * time.Minute

	// DefaultMetricWriteInterval default metric scrape interval.
	DefaultMetricWriteInterval = 15 * time.Second
)

// DefaultNumberOfShards default number of shards.
var DefaultNumberOfShards uint16 = 2

//
// Options
//

type Options struct {
	Seed           uint64
	BlockDuration  time.Duration
	CommitInterval time.Duration
	MaxSegmentSize uint32
	NumberOfShards uint16
}

//
// Config
//

// Config config for [Manager].
type Config struct {
	numberOfShards uint32
}

// NewConfig init new [Config].
func NewConfig(numberOfShards uint16) *Config {
	if numberOfShards == 0 {
		numberOfShards = DefaultNumberOfShards
	}

	return &Config{
		numberOfShards: uint32(numberOfShards),
	}
}

// NumberOfShards returns current number of shards.
func (c *Config) NumberOfShards() uint16 {
	return uint16(atomic.LoadUint32(&c.numberOfShards)) // #nosec G115 // no overflow
}

// SetNumberOfShards set new number of shards.
func (c *Config) SetNumberOfShards(numberOfShards uint16) bool {
	if numberOfShards == 0 {
		numberOfShards = DefaultNumberOfShards
	}

	if c.NumberOfShards() == numberOfShards {
		return false
	}

	atomic.StoreUint32(&c.numberOfShards, uint32(numberOfShards))

	return true
}

//
// Manager
//

type Manager struct {
	g               run.Group
	closer          *util.Closer
	proxy           *proxy.Proxy[*HeadOnDisk]
	cgogc           *cppbridge.CGOGC
	cfg             *Config
	rotatorMediator *mediator.Mediator
}

// NewManager init new [Manager].
func NewManager(
	clock clockwork.Clock,
	dataDir string,
	hcatalog *catalog.Catalog,
	options Options,
	triggerNotifier *ReloadBlocksTriggerNotifier,
	readyNotifier ready.Notifier,
	r prometheus.Registerer,
) (*Manager, error) {
	dirStat, err := os.Stat(dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to stat dir: %w", err)
	}

	if !dirStat.IsDir() {
		return nil, fmt.Errorf("%s is not directory", dataDir)
	}

	builder := NewBuilder(hcatalog, dataDir, options.MaxSegmentSize, r)

	loader := NewLoader(dataDir, options.MaxSegmentSize, r)

	cfg := NewConfig(options.NumberOfShards)

	h, err := uploadOrBuildHead(clock, hcatalog, builder, loader, options.BlockDuration, cfg.NumberOfShards())
	if err != nil {
		return nil, err
	}

	if _, err = hcatalog.SetStatus(h.ID(), catalog.StatusActive); err != nil {
		return nil, errors.Join(fmt.Errorf("failed to set active status: %w", err), h.Close())
	}

	readyNotifier.NotifyReady()

	// TODO implements
	headKeeper := &NoopKeeper{}

	m := &Manager{
		g:      run.Group{},
		closer: util.NewCloser(),
		proxy:  proxy.NewProxy(container.NewWeighted(h), headKeeper, services.CFSViaRange),
		cgogc:  cppbridge.NewCGOGC(r),
		cfg:    cfg,
		rotatorMediator: mediator.NewMediator(
			mediator.NewRotateTimerWithSeed(clock, options.BlockDuration, options.Seed),
		),
	}

	m.initServices(hcatalog, builder, clock, options.CommitInterval, r)

	logger.Infof("[Manager] created")

	return m, nil
}

// ApplyConfig update config.
func (m *Manager) ApplyConfig(numberOfShards uint16) error {
	logger.Infof("reconfiguration start")
	defer logger.Infof("reconfiguration completed")

	h := m.proxy.Get()
	if h.NumberOfShards() == numberOfShards {
		return nil
	}

	if m.cfg.SetNumberOfShards(numberOfShards) {
		m.rotatorMediator.Trigger()
	}

	return nil
}

// Run launches the [Manager]'s services.
func (m *Manager) Run() error {
	defer m.closer.Done()

	return m.g.Run()
}

// Shutdown safe shutdown [Manager]: stop services and close [Head]'s.
func (m *Manager) Shutdown(ctx context.Context) error {
	_ = m.closer.Close()

	return errors.Join(m.proxy.Close(), m.cgogc.Shutdown(ctx))
}

// initServices initializes services for startup.
func (m *Manager) initServices(
	hcatalog *catalog.Catalog,
	builder *Builder,
	clock clockwork.Clock,
	commitInterval time.Duration,
	r prometheus.Registerer,
) {
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
	rotatorCtx, rotatorCancel := context.WithCancel(baseCtx)
	m.g.Add(
		func() error {
			return services.NewRotator(
				m.proxy,
				m.proxy,
				builder,
				m.rotatorMediator,
				m.cfg,
				&statusSetter{catalog: hcatalog},
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

		return clock.Now().Add(-DefaultMergeDuration).UnixMilli() < rec.CreatedAt()
	}

	// Committer
	committerMediator := mediator.NewMediator(mediator.NewConstantIntervalTimer(clock, commitInterval))
	committerCtx, committerCancel := context.WithCancel(baseCtx)
	m.g.Add(
		func() error {
			return services.NewCommitter(m.proxy, committerMediator, isNewHead).Execute(committerCtx)
		},
		func(error) {
			committerMediator.Close()
			committerCancel()
		},
	)

	// Merger
	mergerMediator := mediator.NewMediator(mediator.NewConstantIntervalTimer(clock, DefaultMergeDuration))
	mergerCtx, mergerCancel := context.WithCancel(baseCtx)
	m.g.Add(
		func() error {
			return services.NewMerger(m.proxy, mergerMediator, isNewHead).Execute(mergerCtx)
		},
		func(error) {
			mergerMediator.Close()
			mergerCancel()
		},
	)

	// MetricsUpdater
	metricsUpdaterMediator := mediator.NewMediator(mediator.NewConstantIntervalTimer(clock, DefaultMetricWriteInterval))
	metricsUpdaterCtx, metricsUpdaterCancel := context.WithCancel(baseCtx)
	m.g.Add(
		func() error {
			return services.NewMetricsUpdater(
				m.proxy,
				m.proxy,
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
}

// InitLogHandler init log handler for pp.
func InitLogHandler(l log.Logger) {
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
// headStatusSetter
//

// statusSetter wrapper over [catalog.Catalog] for set statuses.
type statusSetter struct {
	catalog *catalog.Catalog
}

// SetActiveStatus sets the [catalog.StatusActive] status by headID.
func (ha *statusSetter) SetActiveStatus(headID string) error {
	_, err := ha.catalog.SetStatus(headID, catalog.StatusActive)
	return err
}

// SetRotatedStatus sets the [catalog.StatusRotated] status by headID.
func (ha *statusSetter) SetRotatedStatus(headID string) error {
	_, err := ha.catalog.SetStatus(headID, catalog.StatusRotated)
	return err
}

//
// ReloadBlocksTriggerNotifier
//

type ReloadBlocksTriggerNotifier struct {
	c chan struct{}
}

func NewReloadBlocksTriggerNotifier() *ReloadBlocksTriggerNotifier {
	return &ReloadBlocksTriggerNotifier{c: make(chan struct{}, 1)}
}

func (tn *ReloadBlocksTriggerNotifier) Chan() <-chan struct{} {
	return tn.c
}

func (tn *ReloadBlocksTriggerNotifier) NotifyWritten() {
	select {
	case tn.c <- struct{}{}:
	default:
	}
}

//
// NoopKeeper
//

// NoopKeeper implements Keeper.
type NoopKeeper struct{}

// Add implements Keeper.
func (*NoopKeeper) Add(*HeadOnDisk) {}

// Close implements Keeper.
func (*NoopKeeper) Close() error { return nil }

// RangeQueriableHeads implements Keeper.
func (k *NoopKeeper) RangeQueriableHeads(
	mint, maxt int64,
) func(func(*HeadOnDisk) bool) {
	return func(func(*HeadOnDisk) bool) {}
}
