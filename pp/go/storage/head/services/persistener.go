package services

import (
	"slices"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/util"
)

// defaultCoolingInterval the interval after which the rotation should have
// taken place to eliminate errors in the selection from the catalog.
const defaultCoolingInterval = 60 * time.Second

//
// Persistener
//

// Persistener converts and saves spent [Head]s.
type Persistener[
	TTask Task,
	TShard, TGoShard Shard,
	THeadBlockWriter HeadBlockWriter[TShard],
	THead Head[TTask, TShard, TGoShard],
] struct {
	catalog       *catalog.Catalog
	blockWriter   THeadBlockWriter
	writeNotifier WriteNotifier

	clock               clockwork.Clock
	tsdbRetentionPeriod time.Duration
	retentionPeriod     time.Duration
	// stat
	events                  prometheus.Counter
	headPersistenceDuration prometheus.Histogram
}

// NewPersistener init new [Persistener].
func NewPersistener[
	TTask Task,
	TShard, TGoShard Shard,
	THeadBlockWriter HeadBlockWriter[TShard],
	THead Head[TTask, TShard, TGoShard],
](
	hcatalog *catalog.Catalog,
	blockWriter THeadBlockWriter,
	writeNotifier WriteNotifier,
	clock clockwork.Clock,
	tsdbRetentionPeriod time.Duration,
	retentionPeriod time.Duration,
	registerer prometheus.Registerer,
) *Persistener[TTask, TShard, TGoShard, THeadBlockWriter, THead] {
	factory := util.NewUnconflictRegisterer(registerer)
	return &Persistener[TTask, TShard, TGoShard, THeadBlockWriter, THead]{
		catalog:             hcatalog,
		blockWriter:         blockWriter,
		writeNotifier:       writeNotifier,
		clock:               clock,
		tsdbRetentionPeriod: tsdbRetentionPeriod,
		retentionPeriod:     retentionPeriod,
		events: factory.NewCounter(
			prometheus.CounterOpts{
				Name:        "prompp_head_event_count",
				Help:        "Number of head events",
				ConstLabels: prometheus.Labels{"type": "persisted"},
			},
		),
		headPersistenceDuration: factory.NewHistogram(
			prometheus.HistogramOpts{
				Name: "prompp_head_persistence_duration",
				Help: "Block write duration in milliseconds.",
				Buckets: []float64{
					500, 1000, 2500, 5000, 7500,
					10000, 25000, 50000, 75000, 100000,
				},
			},
		),
	}
}

// Persist spent [Head]s.
//
//revive:disable-next-line:function-length // long but readable.
//revive:disable-next-line:cognitive-complexity // long but readable.
//revive:disable-next-line:cyclomatic // long but readable.
func (p *Persistener[TTask, TShard, TGoShard, THeadBlockWriter, THead]) Persist(heads []THead) (outdatedHeads []THead) {
	shouldNotify := false
	for _, head := range heads {
		start := p.clock.Now()
		if !head.IsReadOnly() {
			continue
		}

		logger.Debugf("[Persistener]: head %s start persist", head.ID())
		if record, err := p.catalog.Get(head.ID()); err != nil {
			logger.Errorf("[Persistener]: failed get head %s from catalog: %v", head.ID(), err)
		} else if record.Status() == catalog.StatusPersisted {
			if p.persistedHeadIsOutdated(record.UpdatedAt()) {
				logger.Debugf("[Persistener]: persisted head %s is outdated", head.ID())
				outdatedHeads = append(outdatedHeads, head)
			}

			continue
		}

		if p.HeadIsOutdated(head) {
			logger.Debugf("[Persistener]: head %s is outdated", head.ID())
			if _, err := p.catalog.SetStatus(head.ID(), catalog.StatusPersisted); err != nil {
				logger.Errorf("[Persistener]: set head status in catalog %s: %v", head.ID(), err)
				continue
			}

			outdatedHeads = append(outdatedHeads, head)
			continue
		}

		if err := p.flushHead(head); err != nil {
			logger.Errorf("[Persistener]: failed flush head %s: %v", head.ID(), err)
			continue
		}

		if err := p.persistHead(head); err != nil {
			logger.Errorf("[Persistener]: failed persist head %s: %v", head.ID(), err)
			continue
		}

		if _, err := p.catalog.SetStatus(head.ID(), catalog.StatusPersisted); err != nil {
			logger.Errorf("[Persistener]: set head status in catalog %s: %v", head.ID(), err)
			continue
		}

		logger.Infof("[Persistener]: head %s persisted, duration: %v", head.ID(), p.clock.Since(start))
		p.events.Inc()
		p.headPersistenceDuration.Observe(float64(p.clock.Since(start).Milliseconds()))
		shouldNotify = true
	}

	if shouldNotify {
		p.writeNotifier.Notify()
	}

	return outdatedHeads
}

func (*Persistener[TTask, TShard, TGoShard, THeadBlockWriter, THead]) headTimeInterval(
	head THead,
) cppbridge.TimeInterval {
	timeInterval := cppbridge.NewInvalidTimeInterval()
	for shard := range head.RangeShards() {
		interval := shard.TimeInterval(false)
		timeInterval.MinT = min(interval.MinT, timeInterval.MinT)
		timeInterval.MaxT = max(interval.MaxT, timeInterval.MaxT)
	}
	return timeInterval
}

// HeadIsOutdated check [Head] is outdated.
func (p *Persistener[TTask, TShard, TGoShard, THeadBlockWriter, THead]) HeadIsOutdated(head THead) bool {
	return p.clock.Since(time.UnixMilli(p.headTimeInterval(head).MaxT)) >= p.tsdbRetentionPeriod
}

func (p *Persistener[TTask, TShard, TGoShard, THeadBlockWriter, THead]) persistedHeadIsOutdated(
	persistTimeMs int64,
) bool {
	return p.clock.Since(time.UnixMilli(persistTimeMs)) >= p.retentionPeriod
}

func (*Persistener[TTask, TShard, TGoShard, THeadBlockWriter, THead]) flushHead(head THead) error {
	for shard := range head.RangeShards() {
		if err := shard.WalFlush(); err != nil {
			return err
		}
	}

	return nil
}

func (p *Persistener[TTask, TShard, TGoShard, THeadBlockWriter, THead]) persistHead(head THead) error {
	for shard := range head.RangeShards() {
		if _, err := p.blockWriter.Write(shard); err != nil {
			return err
		}
	}

	return nil
}

//
// PersistenerService
//

// PersistenerService service for persist spent [Head]s.
type PersistenerService[
	TTask Task,
	TShard, TGoShard Shard,
	THeadBlockWriter HeadBlockWriter[TShard],
	THead Head[TTask, TShard, TGoShard],
	TProxyHead ProxyHead[TTask, TShard, TGoShard, THead],
	TLoader Loader[TTask, TShard, TGoShard, THead],
] struct {
	persistener         *Persistener[TTask, TShard, TGoShard, THeadBlockWriter, THead]
	proxy               TProxyHead
	loader              TLoader
	catalog             *catalog.Catalog
	mediator            Mediator
	clock               clockwork.Clock
	tsdbRetentionPeriod time.Duration
}

// NewPersistenerService init new [PersistenerService].
func NewPersistenerService[
	TTask Task,
	TShard, TGoShard Shard,
	THeadBlockWriter HeadBlockWriter[TShard],
	THead Head[TTask, TShard, TGoShard],
	TProxyHead ProxyHead[TTask, TShard, TGoShard, THead],
	TLoader Loader[TTask, TShard, TGoShard, THead],
](
	proxy TProxyHead,
	loader TLoader,
	hcatalog *catalog.Catalog,
	blockWriter THeadBlockWriter,
	writeNotifier WriteNotifier,
	clock clockwork.Clock,
	mediator Mediator,
	tsdbRetentionPeriod time.Duration,
	retentionPeriod time.Duration,
	registerer prometheus.Registerer,
) *PersistenerService[TTask, TShard, TGoShard, THeadBlockWriter, THead, TProxyHead, TLoader] {
	return &PersistenerService[TTask, TShard, TGoShard, THeadBlockWriter, THead, TProxyHead, TLoader]{
		persistener: NewPersistener[TTask, TShard, TGoShard, THeadBlockWriter, THead](
			hcatalog,
			blockWriter,
			writeNotifier,
			clock,
			tsdbRetentionPeriod,
			retentionPeriod,
			registerer,
		),
		proxy:               proxy,
		loader:              loader,
		catalog:             hcatalog,
		mediator:            mediator,
		clock:               clock,
		tsdbRetentionPeriod: tsdbRetentionPeriod,
	}
}

// Execute starts the [PersistenerService].
func (pg *PersistenerService[TTask, TShard, TGoShard, THeadBlockWriter, THead, TKeeper, TLoader]) Execute() {
	logger.Infof("The PersistenerService is running.")

	for range pg.mediator.C() {
		pg.ProcessHeads()
	}

	logger.Infof("The PersistenerService stopped.")
}

// ProcessHeads process persist [Head]s.
func (pg *PersistenerService[TTask, TShard, TGoShard, THeadBlockWriter, THead, TKeeper, TLoader]) ProcessHeads() {
	heads := pg.proxy.Heads()
	pg.persistHeads(heads)
	pg.loadRotatedHeadsInKeeper(heads)
}

func (pg *PersistenerService[TTask, TShard, TGoShard, THeadBlockWriter, THead, TKeeper, TLoader]) persistHeads(
	heads []THead,
) {
	pg.proxy.Remove(pg.persistener.Persist(heads))
}

// loadRotatedHeadsInKeeper loads rotated or unused [Head]s and adds them to the [Keeper].
//
//revive:disable-next-line:cyclomatic // but readable
func (pg *PersistenerService[
	TTask,
	TShard,
	TGoShard,
	THeadBlockWriter,
	THead,
	TKeeper,
	TLoader,
]) loadRotatedHeadsInKeeper(keeperHeads []THead) {
	if !pg.proxy.HasSlot() {
		return
	}

	headExists := func(id string) bool {
		return slices.ContainsFunc(keeperHeads, func(head THead) bool {
			return head.ID() == id
		})
	}

	records := pg.catalog.List(func(record *catalog.Record) bool {
		// in case the rotated status was not set due to an error
		statusIsAppropriate := record.Status() == catalog.StatusNew ||
			record.Status() == catalog.StatusRotated ||
			record.Status() == catalog.StatusActive

		isOutdated := pg.clock.Since(time.UnixMilli(record.CreatedAt())) >= pg.tsdbRetentionPeriod

		return statusIsAppropriate && !headExists(record.ID()) && record.DeletedAt() == 0 && !isOutdated
	}, catalog.LessByUpdateAt)

	aheadID := pg.proxy.Get().ID()
	for _, record := range records {
		// skip active head
		if aheadID == record.ID() {
			continue
		}

		// skip the newly created head
		if (record.Status() == catalog.StatusNew || record.Status() == catalog.StatusActive) &&
			pg.clock.Since(time.UnixMilli(record.CreatedAt())) < defaultCoolingInterval {
			continue
		}

		if !pg.proxy.HasSlot() {
			break
		}

		if !pg.loadAndAddHeadToKeeper(record) {
			break
		}
	}
}

// loadAndAddHeadToKeeper loads [Head] and adds them to the [Keeper].
func (pg *PersistenerService[
	TTask,
	TShard,
	TGoShard,
	THeadBlockWriter,
	THead,
	TKeeper,
	TLoader,
]) loadAndAddHeadToKeeper(record *catalog.Record) bool {
	head := pg.loader.Load(record, 0)
	head.SetReadOnly()
	if err := pg.proxy.Add(head, time.Duration(record.CreatedAt())*time.Millisecond); err != nil {
		_ = head.Close()
		return false
	}

	return true
}
