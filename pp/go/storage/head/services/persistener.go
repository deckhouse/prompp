package services

import (
	"slices"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/block"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/logger"
)

//go:generate -command moq go run github.com/matryer/moq --rm --skip-ensure --pkg mock --out

//
// HeadBlockWriter
//

// HeadBlockWriter writes block on disk from [Head].
//
//go:generate moq mock/persistener.go . HeadBlockWriter
type HeadBlockWriter[TShard Shard] interface {
	Write(shard TShard) ([]block.WrittenBlock, error)
}

type Persistener[
	TTask Task,
	TShard, TGoShard Shard,
	THeadBlockWriter HeadBlockWriter[TShard],
	THead Head[TTask, TShard, TGoShard],
] struct {
	catalog     *catalog.Catalog
	blockWriter THeadBlockWriter

	clock               clockwork.Clock
	tsdbRetentionPeriod time.Duration
	retentionPeriod     time.Duration
}

func NewPersistener[
	TTask Task,
	TShard, TGoShard Shard,
	THeadBlockWriter HeadBlockWriter[TShard],
	THead Head[TTask, TShard, TGoShard],
](
	catalog *catalog.Catalog,
	blockWriter THeadBlockWriter,
	clock clockwork.Clock,
	tsdbRetentionPeriod time.Duration,
	retentionPeriod time.Duration,
) *Persistener[TTask, TShard, TGoShard, THeadBlockWriter, THead] {
	return &Persistener[TTask, TShard, TGoShard, THeadBlockWriter, THead]{
		catalog:             catalog,
		blockWriter:         blockWriter,
		clock:               clock,
		tsdbRetentionPeriod: tsdbRetentionPeriod,
		retentionPeriod:     retentionPeriod,
	}
}

func (p *Persistener[TTask, TShard, TGoShard, THeadBlockWriter, THead]) Persist(heads []THead) (outdatedHeads []THead) {
	for _, head := range heads {
		if !head.IsReadOnly() {
			continue
		}

		if record, err := p.catalog.Get(head.ID()); err == nil {
			if record.Status() == catalog.StatusPersisted {
				if p.persistedHeadIsOutdated(record.UpdatedAt()) {
					outdatedHeads = append(outdatedHeads, head)
				}

				continue
			}
		}

		if p.HeadIsOutdated(head) {
			outdatedHeads = append(outdatedHeads, head)
			continue
		}

		if err := p.flushHead(head); err == nil {
			if err = p.persistHead(head); err == nil {
				if _, err = p.catalog.SetStatus(head.ID(), catalog.StatusPersisted); err != nil {
					logger.Errorf("keeper: set head status in catalog %s: %v", head.ID(), err)
				}
			}
		}
	}

	return
}

func (p *Persistener[TTask, TShard, TGoShard, THeadBlockWriter, THead]) headTimeInterval(
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

func (p *Persistener[TTask, TShard, TGoShard, THeadBlockWriter, THead]) HeadIsOutdated(head THead) bool {
	return p.clock.Since(time.UnixMilli(p.headTimeInterval(head).MaxT)) >= p.tsdbRetentionPeriod
}

func (p *Persistener[TTask, TShard, TGoShard, THeadBlockWriter, THead]) persistedHeadIsOutdated(
	persistTimeMs int64,
) bool {
	return p.clock.Since(time.UnixMilli(persistTimeMs)) >= p.retentionPeriod
}

func (p *Persistener[TTask, TShard, TGoShard, THeadBlockWriter, THead]) flushHead(head THead) error {
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

type Loader[
	TTask Task,
	TShard, TGoShard Shard,
	THead Head[TTask, TShard, TGoShard],
] interface {
	Load(headRecord *catalog.Record, generation uint64) (THead, bool)
}

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
	clock clockwork.Clock,
	mediator Mediator,
	tsdbRetentionPeriod time.Duration,
	retentionPeriod time.Duration,
) *PersistenerService[TTask, TShard, TGoShard, THeadBlockWriter, THead, TKeeper, TLoader] {
	return &PersistenerService[TTask, TShard, TGoShard, THeadBlockWriter, THead, TKeeper, TLoader]{
		persistener: NewPersistener[TTask, TShard, TGoShard, THeadBlockWriter, THead](
			hcatalog,
			blockWriter,
			clock,
			tsdbRetentionPeriod,
			retentionPeriod,
		),
		keeper:   hkeeper,
		loader:   loader,
		catalog:  hcatalog,
		mediator: mediator,
	}
}

func (pg *PersistenerService[TTask, TShard, TGoShard, THeadBlockWriter, THead, TKeeper, TLoader]) Run() {
	logger.Infof("The PersistenerService is running.")

	for range pg.mediator.C() {
		pg.ProcessHeads()
	}

	logger.Infof("The PersistenerService stopped.")
}

func (pg *PersistenerService[TTask, TShard, TGoShard, THeadBlockWriter, THead, TKeeper, TLoader]) ProcessHeads() {
	heads := pg.keeper.Heads()
	pg.persistHeads(heads)
	pg.loadRotatedHeadsInKeeper(heads)
}

func (pg *PersistenerService[
	TTask,
	TShard,
	TGoShard,
	THeadBlockWriter,
	THead,
	TKeeper,
	TLoader,
]) persistHeads(heads []THead) {
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
	if err := pg.keeper.Add(head, time.Duration(record.CreatedAt())*time.Millisecond); err != nil {
		_ = head.Close()
		return false
	}

	return true
}
