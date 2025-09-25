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
	events                  *prometheus.CounterVec
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
		events: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "prompp_head_event_count",
				Help: "Number of head events",
			},
			[]string{"type"},
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
//revive:disable-next-line:cognitive-complexity // long but readable.
//revive:disable-next-line:cyclomatic // long but readable.
func (p *Persistener[TTask, TShard, TGoShard, THeadBlockWriter, THead]) Persist(heads []THead) (outdatedHeads []THead) {
	shouldNotify := false
	for _, head := range heads {
		start := p.clock.Now()
		if !head.IsReadOnly() {
			continue
		}

		if record, err := p.catalog.Get(head.ID()); err != nil {
			logger.Errorf("[Persistener]: failed get head %s from catalog: %v", head.ID(), err)
		} else if record.Status() == catalog.StatusPersisted {
			if p.persistedHeadIsOutdated(record.UpdatedAt()) {
				outdatedHeads = append(outdatedHeads, head)
			}

			continue
		}

		if p.HeadIsOutdated(head) {
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
		p.events.With(prometheus.Labels{"type": "persisted"}).Inc()
		p.headPersistenceDuration.Observe(float64(p.clock.Since(start).Milliseconds()))
		shouldNotify = true
	}

	if shouldNotify {
		p.writeNotifier.NotifyWritten()
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
	TKeeper Keeper[TTask, TShard, TGoShard, THead],
	TLoader Loader[TTask, TShard, TGoShard, THead],
] struct {
	persistener *Persistener[TTask, TShard, TGoShard, THeadBlockWriter, THead]
	keeper      TKeeper
	loader      TLoader
	catalog     *catalog.Catalog
	mediator    Mediator
}

// NewPersistenerService init new [PersistenerService].
func NewPersistenerService[
	TTask Task,
	TShard, TGoShard Shard,
	THeadBlockWriter HeadBlockWriter[TShard],
	THead Head[TTask, TShard, TGoShard],
	TKeeper Keeper[TTask, TShard, TGoShard, THead],
	TLoader Loader[TTask, TShard, TGoShard, THead],
](
	hkeeper TKeeper,
	loader TLoader,
	hcatalog *catalog.Catalog,
	blockWriter THeadBlockWriter,
	writeNotifier WriteNotifier,
	clock clockwork.Clock,
	mediator Mediator,
	tsdbRetentionPeriod time.Duration,
	retentionPeriod time.Duration,
	registerer prometheus.Registerer,
) *PersistenerService[TTask, TShard, TGoShard, THeadBlockWriter, THead, TKeeper, TLoader] {
	return &PersistenerService[TTask, TShard, TGoShard, THeadBlockWriter, THead, TKeeper, TLoader]{
		persistener: NewPersistener[TTask, TShard, TGoShard, THeadBlockWriter, THead](
			hcatalog,
			blockWriter,
			writeNotifier,
			clock,
			tsdbRetentionPeriod,
			retentionPeriod,
			registerer,
		),
		keeper:   hkeeper,
		loader:   loader,
		catalog:  hcatalog,
		mediator: mediator,
	}
}

// Run starts the [PersistenerService].
func (pg *PersistenerService[TTask, TShard, TGoShard, THeadBlockWriter, THead, TKeeper, TLoader]) Run() {
	logger.Infof("The PersistenerService is running.")

	for range pg.mediator.C() {
		pg.ProcessHeads()
	}

	logger.Infof("The PersistenerService stopped.")
}

// ProcessHeads process persist [Head]s.
func (pg *PersistenerService[TTask, TShard, TGoShard, THeadBlockWriter, THead, TKeeper, TLoader]) ProcessHeads() {
	heads := pg.keeper.Heads()
	pg.persistHeads(heads)
	pg.loadRotatedHeadsInKeeper(heads)
}

func (pg *PersistenerService[TTask, TShard, TGoShard, THeadBlockWriter, THead, TKeeper, TLoader]) persistHeads(
	heads []THead,
) {
	pg.keeper.Remove(pg.persistener.Persist(heads))
}

func (pg *PersistenerService[
	TTask,
	TShard,
	TGoShard,
	THeadBlockWriter,
	THead,
	TKeeper,
	TLoader,
]) loadRotatedHeadsInKeeper(keeperHeads []THead) {
	if !pg.keeper.HasSlot() {
		return
	}

	headExists := func(id string) bool {
		return slices.ContainsFunc(keeperHeads, func(head THead) bool {
			return head.ID() == id
		})
	}

	records := pg.catalog.List(func(record *catalog.Record) bool {
		return record.Status() == catalog.StatusRotated && !headExists(record.ID())
	}, catalog.LessByUpdateAt)

	for _, record := range records {
		if !pg.loadAndAddHeadToKeeper(record) {
			break
		}
	}
}

func (pg *PersistenerService[
	TTask,
	TShard,
	TGoShard,
	THeadBlockWriter,
	THead,
	TKeeper,
	TLoader,
]) loadAndAddHeadToKeeper(record *catalog.Record) bool {
	head, _ := pg.loader.Load(record, 0)
	head.SetReadOnly()
	if err := pg.keeper.Add(head, time.Duration(record.CreatedAt())*time.Millisecond); err != nil {
		_ = head.Close()
		return false
	}

	return true
}
