package storage

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/config"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/storage/block"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/head/container"
	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/head/keeper"
	"github.com/prometheus/prometheus/pp/go/storage/head/services"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
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

	// defaultStartMetricWriteInterval the default interval for start [MetricsUpdater] timer.
	defaultStartMetricWriteInterval = 5 * time.Second

	// DefaultPersistDuration the default interval for persisting [Head].
	DefaultPersistDuration = 2 * time.Minute

	// DefaultUnloadDataStorageInterval the default interval for unloading [DataStorage].
	DefaultUnloadDataStorageInterval = 5 * time.Minute

	// defaultStartPersistnerInterval the default interval for start [Persistener] timer.
	defaultStartPersistnerInterval = 15 * time.Second
)

var (
	// UnloadDataStorage flags for unloading [DataStorage].
	UnloadDataStorage = false

	// DefaultNumberOfShards default number of shards.
	DefaultNumberOfShards uint16 = 2
)

//
// Options
//

// Options manager launch options.
type Options struct {
	Seed                uint64
	BlockDuration       time.Duration
	CommitInterval      time.Duration
	MaxRetentionPeriod  time.Duration
	HeadRetentionPeriod time.Duration
	LongtermIntervalMs  int64
	KeeperCapacity      int
	DataDir             string
	MaxSegmentSize      uint32
	NumberOfShards      uint16
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

// Manager manages services for the work of the heads.
type Manager struct {
	g               run.Group
	closer          *util.Closer
	proxy           *Proxy
	cgogc           *cppbridge.CGOGC
	cfg             *Config
	rotatorMediator *mediator.Mediator
	mergerMediator  *mediator.Mediator
	isRunning       bool
}

// NewManager init new [Manager].
//
//revive:disable-next-line:function-length // this is contructor.
func NewManager(
	o *Options,
	clock clockwork.Clock,
	hcatalog *catalog.Catalog,
	reloadBlocksNotifier *TriggerNotifier,
	removedHeadNotifier *TriggerNotifier,
	readyNotifier ready.Notifier,
	r prometheus.Registerer,
) (*Manager, error) {
	if o == nil {
		return nil, errors.New("manager options is nil")
	}

	dataDir, err := filepath.Abs(o.DataDir)
	if err != nil {
		return nil, err
	}
	o.DataDir = dataDir

	dirStat, err := os.Stat(o.DataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to stat dir: %w", err)
	}

	if !dirStat.IsDir() {
		return nil, fmt.Errorf("%s is not directory", o.DataDir)
	}

	var unloadDataStorageInterval time.Duration
	if UnloadDataStorage {
		unloadDataStorageInterval = DefaultUnloadDataStorageInterval
	}

	builder := NewBuilder(hcatalog, o.DataDir, o.MaxSegmentSize, r, unloadDataStorageInterval)
	loader := NewLoader(o.DataDir, o.MaxSegmentSize, r, unloadDataStorageInterval)
	cfg := NewConfig(o.NumberOfShards)
	h, err := UploadOrBuildHead(clock, hcatalog, builder, loader, o.BlockDuration, cfg.NumberOfShards())
	if err != nil {
		return nil, err
	}

	if _, err = hcatalog.SetStatus(h.ID(), catalog.StatusActive); err != nil {
		return nil, errors.Join(fmt.Errorf("failed to set active status: %w", err), h.Close())
	}

	hKeeper := keeper.NewKeeper[Head](
		o.KeeperCapacity,
		removedHeadNotifier,
	)

	m := &Manager{
		g:      run.Group{},
		closer: util.NewCloser(),
		proxy:  NewProxy(container.NewWeighted(h, container.DefaultBackPressure), hKeeper, services.CFSViaRange),
		cgogc:  cppbridge.NewCGOGC(r),
		cfg:    cfg,
		rotatorMediator: mediator.NewMediator(
			mediator.NewRotateTimerWithSeed(clock, o.BlockDuration, o.Seed),
		),
		mergerMediator: mediator.NewMediator(
			mediator.NewConstantIntervalTimer(clock, DefaultMergeDuration, DefaultMergeDuration),
		),
	}

	m.initServices(o, hcatalog, builder, loader, reloadBlocksNotifier, readyNotifier, clock, r)
	logger.Infof("[Head Manager] created")

	return m, nil
}

// ApplyConfig update config.
func (m *Manager) ApplyConfig(cfg *config.Config) error {
	logger.Infof("reconfiguration start")
	defer logger.Infof("reconfiguration completed")

	if m.cfg.SetNumberOfShards(cfg.PPNumberOfShards()) || m.proxy.Get().NumberOfShards() != m.cfg.NumberOfShards() {
		m.rotatorMediator.Trigger()
	}

	return nil
}

// MergeOutOfOrderChunks send signal to merge chunks with out of order data chunks.
func (m *Manager) MergeOutOfOrderChunks() {
	m.mergerMediator.Trigger()
}

// Proxy returns proxy to the active [Head] and the keeper of old [Head]s.
func (m *Manager) Proxy() *Proxy {
	return m.proxy
}

// Run launches the [Manager]'s services.
func (m *Manager) Run() error {
	defer m.closer.Done()

	return m.g.Run()
}

// Shutdown safe shutdown [Manager]: stop services and close [Head]'s.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.close()

	return errors.Join(m.proxy.Close(), m.cgogc.Shutdown(ctx))
}

// initServices initializes services for startup.
//
//revive:disable-next-line:function-length // init contructor.
func (m *Manager) initServices(
	o *Options,
	hcatalog *catalog.Catalog,
	builder *Builder,
	loader *Loader,
	reloadBlocksTriggerNotifier *TriggerNotifier,
	readyNotifier ready.Notifier,
	clock clockwork.Clock,
	r prometheus.Registerer,
) {
	baseCtx := context.Background()

	// Termination handler.
	m.g.Add(
		func() error {
			readyNotifier.NotifyReady()
			m.isRunning = true
			<-m.closer.Signal()

			return nil
		},
		func(error) {
			m.close()
		},
	)

	// Persistener
	persistenerMediator := mediator.NewMediator(
		mediator.NewConstantIntervalTimer(clock, defaultStartPersistnerInterval, DefaultPersistDuration),
	)
	m.g.Add(
		func() error {
			services.NewPersistenerService(
				m.proxy,
				loader,
				hcatalog,
				block.NewWriter[*shard.Shard](
					o.DataDir,
					block.DefaultChunkSegmentSize,
					o.BlockDuration,
					r,
				),
				reloadBlocksTriggerNotifier,
				clock,
				persistenerMediator,
				o.MaxRetentionPeriod,
				o.HeadRetentionPeriod,
				o.LongtermIntervalMs,
				r,
			).Execute()

			return nil
		},
		func(error) {
			persistenerMediator.Close()
		},
	)

	// Rotator
	rotatorCtx, rotatorCancel := context.WithCancel(baseCtx)
	m.g.Add(
		func() error {
			return services.NewRotator(
				m.proxy,
				builder,
				m.rotatorMediator,
				m.cfg,
				&headInformer{catalog: hcatalog},
				head.CopyAddedSeries[*shard.Shard, *shard.PerGoroutineShard](shard.CopyAddedSeries),
				persistenerMediator.TriggerWithResetTimer,
				r,
			).Execute(rotatorCtx)
		},
		func(error) {
			m.rotatorMediator.Close()
			rotatorCancel()
		},
	)

	// Committer
	committerMediator := mediator.NewMediator(
		mediator.NewConstantIntervalTimer(clock, o.CommitInterval, o.CommitInterval),
	)
	committerCtx, committerCancel := context.WithCancel(baseCtx)
	m.g.Add(
		func() error {
			return services.NewCommitter(
				m.proxy,
				committerMediator,
				isNewHead(clock, hcatalog, o.CommitInterval),
			).Execute(committerCtx)
		},
		func(error) {
			committerMediator.Close()
			committerCancel()
		},
	)

	// Merger
	mergerCtx, mergerCancel := context.WithCancel(baseCtx)
	m.g.Add(
		func() error {
			return services.NewMerger(
				m.proxy,
				m.mergerMediator,
				isNewHead(clock, hcatalog, DefaultMergeDuration),
			).Execute(mergerCtx)
		},
		func(error) {
			m.mergerMediator.Close()
			mergerCancel()
		},
	)

	// MetricsUpdater
	metricsUpdaterMediator := mediator.NewMediator(
		mediator.NewConstantIntervalTimer(clock, defaultStartMetricWriteInterval, DefaultMetricWriteInterval),
	)
	metricsUpdaterCtx, metricsUpdaterCancel := context.WithCancel(baseCtx)
	m.g.Add(
		func() error {
			return services.NewMetricsUpdater(
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

func (m *Manager) close() {
	if !m.isRunning {
		m.closer.Done()
	}

	select {
	case <-m.closer.Signal():
	default:
		_ = m.closer.Close()
	}
}

//
// headInformer
//

// headInformer wrapper over [catalog.Catalog] for set statuses and get info.
type headInformer struct {
	catalog *catalog.Catalog
}

// CreatedAt returns the timestamp when the [Record]([Head]) was created.
func (hi *headInformer) CreatedAt(headID string) time.Duration {
	record, err := hi.catalog.Get(headID)
	if err != nil {
		return time.Duration(math.MaxInt64)
	}

	return time.Duration(record.CreatedAt()) * time.Millisecond
}

// SetActiveStatus sets the [catalog.StatusActive] status by headID.
func (hi *headInformer) SetActiveStatus(headID string) error {
	_, err := hi.catalog.SetStatus(headID, catalog.StatusActive)
	return err
}

// SetRotatedStatus sets the [catalog.StatusRotated] status by headID.
func (hi *headInformer) SetRotatedStatus(headID string) error {
	_, err := hi.catalog.SetStatus(headID, catalog.StatusRotated)
	return err
}

//
// isNewHead
//

// isNewHead builds a checker that checks if the head is new.
func isNewHead(clock clockwork.Clock, hcatalog *catalog.Catalog, interval time.Duration) func(headID string) bool {
	return func(headID string) bool {
		rec, err := hcatalog.Get(headID)
		if err != nil {
			return true
		}

		return clock.Now().Add(-interval).UnixMilli() < rec.CreatedAt()
	}
}

//
// TriggerNotifier
//

// TriggerNotifier to receive notifications about new events.
type TriggerNotifier struct {
	c chan struct{}
}

// NewTriggerNotifier init new [TriggerNotifier].
func NewTriggerNotifier() *TriggerNotifier {
	return &TriggerNotifier{c: make(chan struct{}, 1)}
}

// NewTriggerNotifier init new noop [TriggerNotifier].
func NewNoopTriggerNotifier() *TriggerNotifier {
	return &TriggerNotifier{}
}

// Chan returns channel with notifications.
func (tn *TriggerNotifier) Chan() <-chan struct{} {
	return tn.c
}

// Notify sends a notify that the writing is completed.
func (tn *TriggerNotifier) Notify() {
	select {
	case tn.c <- struct{}{}:
	default:
	}
}

//
// uploadOrBuildHead
//

// UploadOrBuildHead uploads or builds a new head.
//
//revive:disable-next-line:function-length // long but readable.
//revive:disable-next-line:cyclomatic // long but readable.
func UploadOrBuildHead(
	clock clockwork.Clock,
	hcatalog *catalog.Catalog,
	builder *Builder,
	loader *Loader,
	blockDuration time.Duration,
	numberOfShards uint16,
) (*Head, error) {
	headRecords := hcatalog.List(
		func(record *catalog.Record) bool {
			statusIsAppropriate := record.Status() == catalog.StatusNew ||
				record.Status() == catalog.StatusActive

			isInBlockTimeRange := clock.Now().Sub(
				time.UnixMilli(record.CreatedAt()),
			).Milliseconds() < blockDuration.Milliseconds()

			return record.DeletedAt() == 0 && statusIsAppropriate && isInBlockTimeRange
		},
		func(lhs, rhs *catalog.Record) bool {
			return lhs.CreatedAt() > rhs.CreatedAt()
		},
	)

	if numberOfShards == 0 {
		numberOfShards = DefaultNumberOfShards
	}

	var generation uint64
	if len(headRecords) == 0 {
		logger.Debugf("[Head Manager] no suitable heads were found, building new")
		return builder.Build(generation, numberOfShards)
	}

	h, err := loader.Load(headRecords[0], generation)
	if err != nil {
		if headRecords[0].Status() == catalog.StatusNew || headRecords[0].Status() == catalog.StatusActive {
			if _, setStatusErr := hcatalog.SetStatus(headRecords[0].ID(), catalog.StatusRotated); setStatusErr != nil {
				logger.Warnf("failed to set rotated status for head {%s}: %s", headRecords[0].ID(), setStatusErr)
			}
		}

		_ = h.Close()

		if errors.Is(err, cppbridge.ErrInvalidEncoderVersion) {
			logger.Warnf("[Head Manager] upload non continuable head {%s}, building new...", headRecords[0].ID())
			return builder.Build(generation+1, numberOfShards)
		}

		if !headRecords[0].Corrupted() {
			if _, setCorruptedErr := hcatalog.SetCorrupted(headRecords[0].ID()); setCorruptedErr != nil {
				logger.Errorf("failed to set corrupted state, head {%s}: %v", headRecords[0].ID(), setCorruptedErr)
			}
		}

		logger.Warnf("[Head Manager] upload corrupted head {%s}, building new...", headRecords[0].ID())
		return builder.Build(generation+1, numberOfShards)
	}

	if _, err = hcatalog.SetStatus(headRecords[0].ID(), catalog.StatusActive); err != nil {
		logger.Warnf("failed to set active status for head {%s}: %s", headRecords[0].ID(), err)

		return builder.Build(generation+1, numberOfShards)
	}

	return h, nil
}
