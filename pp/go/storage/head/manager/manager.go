package manager

// import (
// 	"context"
// 	"errors"
// 	"fmt"
// 	"time"

// 	"github.com/prometheus/client_golang/prometheus"

// 	"github.com/prometheus/prometheus/pp/go/cppbridge"
// 	"github.com/prometheus/prometheus/pp/go/storage/catalog"
// 	"github.com/prometheus/prometheus/pp/go/storage/logger"
// 	"github.com/prometheus/prometheus/pp/go/util"
// )

// const (
// 	// DSMergeOutOfOrderChunks name of task.
// 	DSMergeOutOfOrderChunks = "data_storage_merge_out_of_order_chunks"

// 	// WalCommit name of task.
// 	WalCommit = "wal_commit"
// )

// //
// // Timer
// //

// // Timer implementation timer.
// type Timer interface {
// 	Chan() <-chan time.Time
// 	Reset()
// 	Stop()
// }

// //
// // GenericTask
// //

// // GenericTask the minimum required task [Generic] implementation.
// type GenericTask interface {
// 	// Wait for the task to complete on all shards.
// 	Wait() error
// }

// //
// // DataStorage
// //

// // DataStorage the minimum required [DataStorage] implementation.
// type DataStorage interface {
// 	// MergeOutOfOrderChunks merge chunks with out of order data chunks.
// 	MergeOutOfOrderChunks()
// }

// //
// // LSS
// //

// // LSS the minimum required [LSS] implementation.
// type LSS interface {
// 	// WithRLock calls fn on raws [cppbridge.LabelSetStorage] with read lock.
// 	WithRLock(fn func(target, input *cppbridge.LabelSetStorage) error) error
// }

// //
// // Wal
// //

// // Wal the minimum required Wal implementation for a [Shard].
// type Wal interface {
// 	// Commit finalize segment from encoder and add to wal.
// 	// It is necessary to lock the LSS for reading for the commit.
// 	Commit() error

// 	// Flush wal segment writer, write all buffered data to storage.
// 	Flush() error
// }

// //
// // Shard
// //

// // Shard the minimum required head [Shard] implementation.
// type Shard[TDataStorage DataStorage, TLSS LSS, TWal Wal] interface {
// 	// DataStorage returns shard [DataStorage].
// 	DataStorage() TDataStorage

// 	// LSS returns shard labelset storage [LSS].
// 	LSS() TLSS

// 	// ShardID returns the shard ID.
// 	ShardID() uint16

// 	// Wal returns write-ahead log.
// 	Wal() TWal
// }

// //
// // Head
// //

// // Head the minimum required [Head] implementation.
// type Head[
// 	TGenericTask GenericTask,
// 	TDataStorage DataStorage,
// 	TLSS LSS,
// 	TWal Wal,
// 	TShard, TGShard Shard[TDataStorage, TLSS, TWal],
// ] interface {
// 	// Close closes wals, query semaphore for the inability to get query and clear metrics.
// 	Close() error

// 	// CreateTask create a task for operations on the [Head] shards.
// 	CreateTask(taskName string, shardFn func(shard TGShard) error) TGenericTask

// 	// Enqueue the task to be executed on shards [Head].
// 	Enqueue(t TGenericTask)

// 	// Generation returns current generation of [Head].
// 	Generation() uint64

// 	// NumberOfShards returns current number of shards in to [Head].
// 	NumberOfShards() uint16

// 	// RangeShards returns an iterator over the [Head] [Shard]s, through which the shard can be directly accessed.
// 	RangeShards() func(func(TShard) bool)

// 	// SetReadOnly sets the read-only flag for the [Head].
// 	SetReadOnly()
// }

// //
// // ActiveHeadContainer
// //

// // ActiveHeadContainer container for active [Head], the minimum required [ActiveHeadContainer] implementation.
// type ActiveHeadContainer[
// 	TGenericTask GenericTask,
// 	TDataStorage DataStorage,
// 	TLSS LSS,
// 	TWal Wal,
// 	TShard, TGShard Shard[TDataStorage, TLSS, TWal],
// 	THead Head[TGenericTask, TDataStorage, TLSS, TWal, TShard, TGShard],
// ] interface {
// 	// Close closes [ActiveHeadContainer] for the inability work with [Head].
// 	Close() error

// 	// Get the active head [Head].
// 	Get() THead

// 	// Replace the active head [Head] with a new head.
// 	Replace(ctx context.Context, newHead THead) error

// 	// With calls fn(h Head).
// 	With(ctx context.Context, fn func(h THead) error) error
// }

// //
// // Keeper
// //

// type Keeper[
// 	TGenericTask GenericTask,
// 	TDataStorage DataStorage,
// 	TLSS LSS,
// 	TWal Wal,
// 	TShard, TGShard Shard[TDataStorage, TLSS, TWal],
// 	THead Head[TGenericTask, TDataStorage, TLSS, TWal, TShard, TGShard],
// ] interface {
// 	Add(head THead)
// 	RangeQueriableHeads(mint, maxt int64) func(func(THead) bool)
// }

// //
// // Loader
// //

// // Loader loads [Head] from wal, the minimum required [Loader] implementation.
// type Loader[
// 	TGenericTask GenericTask,
// 	TDataStorage DataStorage,
// 	TLSS LSS,
// 	TWal Wal,
// 	TShard, TGShard Shard[TDataStorage, TLSS, TWal],
// 	THead Head[TGenericTask, TDataStorage, TLSS, TWal, TShard, TGShard],
// ] interface {
// 	// UploadHead upload [THead] from wal by head ID.
// 	UploadHead(headRecord *catalog.Record, generation uint64) (head THead, corrupted bool)
// }

// // HeadBuilder building new [Head] with parameters, the minimum required [HeadBuilder] implementation.
// type HeadBuilder[
// 	TGenericTask GenericTask,
// 	TDataStorage DataStorage,
// 	TLSS LSS,
// 	TWal Wal,
// 	TShard, TGShard Shard[TDataStorage, TLSS, TWal],
// 	THead Head[TGenericTask, TDataStorage, TLSS, TWal, TShard, TGShard],
// ] interface {
// 	// Build new [Head].
// 	Build(generation uint64, numberOfShards uint16) (THead, error)
// }

// type Manager[
// 	TGenericTask GenericTask,
// 	TDataStorage DataStorage,
// 	TLSS LSS,
// 	TWal Wal,
// 	TShard, TGShard Shard[TDataStorage, TLSS, TWal],
// 	THead Head[TGenericTask, TDataStorage, TLSS, TWal, TShard, TGShard],
// ] struct {
// 	activeHead  ActiveHeadContainer[TGenericTask, TDataStorage, TLSS, TWal, TShard, TGShard, THead]
// 	keeper      Keeper[TGenericTask, TDataStorage, TLSS, TWal, TShard, TGShard, THead]
// 	headBuilder HeadBuilder[TGenericTask, TDataStorage, TLSS, TWal, TShard, TGShard, THead]
// 	headLoader  Loader[TGenericTask, TDataStorage, TLSS, TWal, TShard, TGShard, THead]
// 	rotateTimer Timer
// 	commitTimer Timer
// 	mergeTimer  Timer

// 	numberOfShards uint16

// 	// TODO closer vs shutdowner
// 	closer     *util.Closer
// 	shutdowner *util.GracefulShutdowner

// 	rotateCounter prometheus.Counter
// 	counter       *prometheus.CounterVec
// }

// // NewManager init new [Manager] of [Head]s.
// func NewManager[
// 	TGenericTask GenericTask,
// 	TDataStorage DataStorage,
// 	TLSS LSS,
// 	TWal Wal,
// 	TShard, TGShard Shard[TDataStorage, TLSS, TWal],
// 	THead Head[TGenericTask, TDataStorage, TLSS, TWal, TShard, TGShard],
// ](
// 	activeHead ActiveHeadContainer[TGenericTask, TDataStorage, TLSS, TWal, TShard, TGShard, THead],
// 	headBuilder HeadBuilder[TGenericTask, TDataStorage, TLSS, TWal, TShard, TGShard, THead],
// 	headLoader Loader[TGenericTask, TDataStorage, TLSS, TWal, TShard, TGShard, THead],
// 	numberOfShards uint16,
// 	registerer prometheus.Registerer,
// ) *Manager[TGenericTask, TDataStorage, TLSS, TWal, TShard, TGShard, THead] {
// 	factory := util.NewUnconflictRegisterer(registerer)
// 	return &Manager[TGenericTask, TDataStorage, TLSS, TWal, TShard, TGShard, THead]{
// 		activeHead:  activeHead,
// 		headBuilder: headBuilder,
// 		headLoader:  headLoader,

// 		numberOfShards: numberOfShards,

// 		counter: factory.NewCounterVec(
// 			prometheus.CounterOpts{
// 				Name: "prompp_head_event_count",
// 				Help: "Number of head events",
// 			},
// 			[]string{"type"},
// 		),
// 	}
// }

// // ApplyConfig update config.
// func (m *Manager[TGenericTask, TDataStorage, TLSS, TWal, TShard, TGShard, THead]) ApplyConfig(
// 	ctx context.Context,
// 	numberOfShards uint16,
// ) error {
// 	logger.Infof("reconfiguration start")
// 	defer logger.Infof("reconfiguration completed")

// 	m.numberOfShards = numberOfShards

// 	h := m.activeHead.Get()
// 	if h.NumberOfShards() == numberOfShards {
// 		return nil
// 	}

// 	return m.rotate(ctx)
// }

// // MergeOutOfOrderChunks merge chunks with out of order data chunks.
// func (m *Manager[TGenericTask, TDataStorage, TLSS, TWal, TShard, TGShard, THead]) MergeOutOfOrderChunks(
// 	ctx context.Context,
// ) error {
// 	return m.activeHead.With(ctx, func(h THead) error {
// 		mergeOutOfOrderChunksWithHead(h)

// 		return nil
// 	})
// }

// // Run starts processing of the [Manager].
// // TODO implementation.
// func (m *Manager[TGenericTask, TDataStorage, TLSS, TWal, TShard, TGShard, THead]) Run(ctx context.Context) error {
// 	go m.loop(ctx)
// 	return nil
// }

// // Shutdown safe shutdown [Manager].
// func (m *Manager[TGenericTask, TDataStorage, TLSS, TWal, TShard, TGShard, THead]) Shutdown(ctx context.Context) error {
// 	// TODO
// 	// cgogcErr := rr.cgogc.Shutdown(ctx)
// 	// err := rr.shutdowner.Shutdown(ctx)
// 	activeHeadErr := m.activeHead.Close()

// 	h := m.activeHead.Get()
// 	commitErr := commitAndFlushViaRange(h)

// 	headCloseErr := h.Close()

// 	return errors.Join(activeHeadErr, commitErr, headCloseErr)
// }

// // commitToWal commit and flush the accumulated data into the wal.
// func (m *Manager[TGenericTask, TDataStorage, TLSS, TWal, TShard, TGShard, THead]) commitToWal(
// 	ctx context.Context,
// ) error {
// 	return m.activeHead.With(ctx, func(h THead) error {
// 		t := h.CreateTask(
// 			WalCommit,
// 			func(shard TGShard) error {
// 				swal := shard.Wal()

// 				// wal contains LSS and it is necessary to lock the LSS for reading for the commit.
// 				if err := shard.LSS().WithRLock(func(_, _ *cppbridge.LabelSetStorage) error {
// 					return swal.Commit()
// 				}); err != nil {
// 					return err
// 				}

// 				return swal.Flush()
// 			},
// 		)
// 		h.Enqueue(t)

// 		return t.Wait()
// 	})
// }

// // TODO implementation.
// func (m *Manager[TGenericTask, TDataStorage, TLSS, TWal, TShard, TGShard, THead]) loop(ctx context.Context) {
// 	defer m.closer.Done()

// 	for {
// 		select {
// 		case <-m.closer.Signal():
// 			return

// 		case <-m.commitTimer.Chan():
// 			if err := m.commitToWal(ctx); err != nil {
// 				logger.Errorf("wal commit failed: %v", err)
// 			}
// 			m.commitTimer.Reset()

// 		case <-m.mergeTimer.Chan():
// 			if err := m.MergeOutOfOrderChunks(ctx); err != nil {
// 				logger.Errorf("merge out of order chunks failed: %v", err)
// 			}
// 			m.mergeTimer.Reset()

// 		case <-m.rotateTimer.Chan():
// 			logger.Debugf("start rotation")

// 			if err := m.rotate(ctx); err != nil {
// 				logger.Errorf("rotation failed: %v", err)
// 			}
// 			m.rotateCounter.Inc()

// 			m.rotateTimer.Reset()
// 			m.commitTimer.Reset()
// 			m.mergeTimer.Reset()
// 		}
// 	}
// }

// func (m *Manager[TGenericTask, TDataStorage, TLSS, TWal, TShard, TGShard, THead]) rotate(ctx context.Context) error {
// 	oldHead := m.activeHead.Get()

// 	newHead, err := m.headBuilder.Build(oldHead.Generation()+1, m.numberOfShards)
// 	if err != nil {
// 		return fmt.Errorf("failed to build a new head: %w", err)
// 	}

// 	// TODO CopySeriesFrom only old nunber of shards == new
// 	// newHead.CopySeriesFrom(oldHead)

// 	m.keeper.Add(oldHead)

// 	// TODO if replace error?
// 	err = m.activeHead.Replace(ctx, newHead)
// 	if err != nil {
// 		return fmt.Errorf("failed to replace old to new head: %w", err)
// 	}

// 	mergeOutOfOrderChunksWithHead(oldHead)

// 	if err := commitAndFlushViaRange(oldHead); err != nil {
// 		logger.Warnf("failed commit and flush to wal: %s", err)
// 	}

// 	oldHead.SetReadOnly()

// 	return nil
// }

// // WithAppendableHead
// // TODO implementation.
// func (m *Manager[TGenericTask, TDataStorage, TLSS, TWal, TShard, TGShard, THead]) WithAppendableHead(
// 	ctx context.Context,
// 	fn func(h THead) error,
// ) error {
// 	return m.activeHead.With(ctx, fn)
// }

// // RangeQueriableHeads
// // TODO implementation.
// func (m *Manager[TGenericTask, TDataStorage, TLSS, TWal, TShard, TGShard, THead]) RangeQueriableHeads(
// 	mint, maxt int64,
// ) func(func(THead) bool) {
// 	// ahead := m.activeHead.Get()
// 	// for h := range m.keeper.RangeQueriableHeads(mint, maxt) {
// 	// TODO
// 	// if h == ahead {
// 	//  continue
// 	// }
// 	// }

// 	return nil
// }

// // mergeOutOfOrderChunksWithHead merge chunks with out of order data chunks for [Head].
// func mergeOutOfOrderChunksWithHead[
// 	TGenericTask GenericTask,
// 	TDataStorage DataStorage,
// 	TLSS LSS,
// 	TWal Wal,
// 	TShard, TGShard Shard[TDataStorage, TLSS, TWal],
// 	THead Head[TGenericTask, TDataStorage, TLSS, TWal, TShard, TGShard],
// ](h THead) {
// 	t := h.CreateTask(
// 		DSMergeOutOfOrderChunks,
// 		func(shard TGShard) error {
// 			shard.DataStorage().MergeOutOfOrderChunks()

// 			return nil
// 		},
// 	)
// 	h.Enqueue(t)

// 	_ = t.Wait()
// }

// // commitAndFlushViaRange finalize segment from encoder and add to wal
// // and flush wal segment writer, write all buffered data to storage.
// func commitAndFlushViaRange[
// 	TGenericTask GenericTask,
// 	TDataStorage DataStorage,
// 	TLSS LSS,
// 	TWal Wal,
// 	TShard, TGShard Shard[TDataStorage, TLSS, TWal],
// 	THead Head[TGenericTask, TDataStorage, TLSS, TWal, TShard, TGShard],
// ](h THead) error {
// 	errs := make([]error, 0, h.NumberOfShards()*2)
// 	for shard := range h.RangeShards() {
// 		if err := shard.Wal().Commit(); err != nil {
// 			errs = append(errs, fmt.Errorf("commit shard id %d: %w", shard.ShardID(), err))
// 		}

// 		if err := shard.Wal().Flush(); err != nil {
// 			errs = append(errs, fmt.Errorf("flush shard id %d: %w", shard.ShardID(), err))
// 		}
// 	}

// 	return errors.Join(errs...)
// }
