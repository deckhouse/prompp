package manager

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/logger"
	"github.com/prometheus/prometheus/pp/go/util"
)

type Timer interface {
	Chan() <-chan time.Time
	Reset()
	Stop()
}

type Head interface {
	// Generation returns current generation of [Head].
	Generation() uint64

	// NumberOfShards returns current number of shards in to [Head].
	NumberOfShards() uint16

	// SetReadOnly sets the read-only flag for the [Head].
	SetReadOnly()
}

// ActiveHeadContainer container for active [Head], the minimum required [ActiveHeadContainer] implementation.
type ActiveHeadContainer[THead Head] interface {
	// Get the active head [Head].
	Get() THead

	// Replace the active head [Head] with a new head.
	Replace(ctx context.Context, newHead THead) error

	// With calls fn(h Head).
	With(ctx context.Context, fn func(h THead) error) error
}

type Keeper[THead Head] interface {
	Add(head THead)
	RangeQueriableHeads(mint, maxt int64) func(func(THead) bool)
}

// Loader loads [Head] from wal, the minimum required [Loader] implementation.
type Loader[THead Head] interface {
	// UploadHead upload [THead] from wal by head ID.
	UploadHead(headRecord *catalog.Record, generation uint64) (head THead, corrupted bool)
}

// HeadBuilder building new [Head] with parameters, the minimum required [HeadBuilder] implementation.
type HeadBuilder[THead Head] interface {
	// Build new [Head].
	Build(generation uint64, numberOfShards uint16) (THead, error)
}

type Manager[THead Head] struct {
	activeHead  ActiveHeadContainer[THead]
	keeper      Keeper[THead]
	headBuilder HeadBuilder[THead]
	headLoader  Loader[THead]
	rotateTimer Timer
	commitTimer Timer
	mergeTimer  Timer

	numberOfShards uint16

	// TODO closer vs shutdowner
	closer     *util.Closer
	shutdowner *util.GracefulShutdowner

	rotateCounter prometheus.Counter
	counter       *prometheus.CounterVec
}

// NewManager init new [Manager] of [Head]s.
func NewManager[THead Head](
	activeHead ActiveHeadContainer[THead],
	headBuilder HeadBuilder[THead],
	headLoader Loader[THead],
	numberOfShards uint16,
	registerer prometheus.Registerer,
) *Manager[THead] {
	factory := util.NewUnconflictRegisterer(registerer)
	return &Manager[THead]{
		activeHead:  activeHead,
		headBuilder: headBuilder,
		headLoader:  headLoader,

		numberOfShards: numberOfShards,

		counter: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "prompp_head_event_count",
				Help: "Number of head events",
			},
			[]string{"type"},
		),
	}
}

// ApplyConfig update config.
func (m *Manager[THead]) ApplyConfig(
	ctx context.Context,
	numberOfShards uint16,
) error {
	logger.Infof("reconfiguration start")
	defer logger.Infof("reconfiguration completed")

	m.numberOfShards = numberOfShards

	h := m.activeHead.Get()
	if h.NumberOfShards() != numberOfShards {
		// TODO rotate
	}

	return nil
}

// MergeOutOfOrderChunks merge chunks with out of order data chunks.
func (m *Manager[THead]) MergeOutOfOrderChunks(ctx context.Context) error {
	// TODO ?
	// return m.activeHead.With(ctx, func(h storage.Head) error {
	// 	h.MergeOutOfOrderChunks()

	// 	return nil
	// })

	return nil
}

// Run starts processing of the [Manager].
// TODO implementation.
func (m *Manager[THead]) Run(ctx context.Context) error {
	go m.loop(ctx)
	return nil
}

// Shutdown safe shutdown [Manager].
func (m *Manager[THead]) Shutdown(ctx context.Context) error {
	return nil
}

// commitToWal commit the accumulated data into the wal.
func (m *Manager[THead]) commitToWal(ctx context.Context) error {
	// TODO ?
	// return m.activeHead.With(ctx, func(h storage.Head) error {
	// 	return h.CommitToWal()
	// })
	return nil
}

// TODO implementation.
func (m *Manager[THead]) loop(ctx context.Context) {
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

func (m *Manager[THead]) rotate(ctx context.Context) error {
	oldHead := m.activeHead.Get()

	newHead, err := m.headBuilder.Build(oldHead.Generation()+1, m.numberOfShards)
	if err != nil {
		return fmt.Errorf("failed to build a new head: %w", err)
	}

	// TODO
	// newHead.CopySeriesFrom(oldHead)

	m.keeper.Add(oldHead)

	// TODO if replace error?
	err = m.activeHead.Replace(ctx, newHead)
	if err != nil {
		return fmt.Errorf("failed to replace old to new head: %w", err)
	}

	oldHead.SetReadOnly()

	return nil
}

// WithAppendableHead
// TODO implementation.
func (m *Manager[THead]) WithAppendableHead(ctx context.Context, fn func(h THead) error) error {
	return m.activeHead.With(ctx, fn)
}

// RangeQueriableHeads
// TODO implementation.
func (m *Manager[THead]) RangeQueriableHeads(mint, maxt int64) func(func(THead) bool) {
	// ahead := m.activeHead.Get()
	// for h := range m.keeper.RangeQueriableHeads(mint, maxt) {
	// TODO
	// if h == ahead {
	//  continue
	// }
	// }

	return nil
}
