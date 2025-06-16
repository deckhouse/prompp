package appender

import (
	"sync"
	"time"

	"github.com/jonboulle/clockwork"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/block"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/prometheus/pp/go/relabeler/querier"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/prometheus/storage"
)

const (
	flushRotateCloseDiscardTimeout = time.Second * 30
	writeRetryTimeout              = 5 * time.Minute
)

type WriteNotifier interface {
	NotifyWritten()
}

// BlockWriter writes block on disk.
type BlockWriter interface {
	Write(block block.Block) error
}

type WritableHead struct {
	relabeler.Head
	writeBlockedUntil            time.Time
	nextWriteAttemptBlockedUntil *time.Time
	writeDeadline                time.Time
	flushed                      bool
	rotated                      bool
	converted                    bool
	closed                       bool
	discarded                    bool
}

func (h *WritableHead) Flush() error {
	if !h.flushed {
		if err := h.Flush(); err != nil {
			return err
		}
		h.flushed = true
	}
	return nil
}

func (h *WritableHead) Rotate() error {
	if !h.rotated {
		if err := h.Rotate(); err != nil {
			return err
		}
		h.rotated = true
	}
	return nil
}

func (h *WritableHead) IsOutdated(now time.Time) bool {
	return now.After(h.writeDeadline)
}

func (h *WritableHead) IsReadyForConversion(now time.Time) bool {
	return h.flushed && h.rotated && !h.converted && now.After(h.writeBlockedUntil) && now.Before(h.writeDeadline)
}

func (h *WritableHead) Convert(blockWriter BlockWriter) error {
	if !h.converted {
		tBlockWrite := h.CreateTask(
			relabeler.BlockWrite,
			func(shard relabeler.Shard) error {
				return blockWriter.Write(relabeler.NewBlock(shard.LSS().Raw(), shard.DataStorage().Raw()))
			},
			relabeler.ForLSSTask,
			relabeler.ExclusiveTask,
		)
		h.Enqueue(tBlockWrite)
		if err := tBlockWrite.Wait(); err != nil {
			return err
		}
		h.converted = true
	}

	return nil
}

func (h *WritableHead) CloseAndDiscard() error {
	if !h.closed {
		if err := h.Close(); err != nil {
			return err
		}
		h.closed = true
	}

	if !h.discarded {
		if err := h.Discard(); err != nil {
			return err
		}
		h.discarded = true
	}

	return nil
}

// QueryableStorage hold reference to finalized heads and writes blocks from them. Also allows query not yet not
// persisted heads.
type QueryableStorage struct {
	blockWriter   BlockWriter
	writeNotifier WriteNotifier
	mtx           sync.Mutex
	heads         []*WritableHead

	closer *util.Closer

	trigger                          chan struct{}
	clock                            clockwork.Clock
	initialDelay                     time.Duration
	cooldownDuration                 time.Duration
	retentionDuration                time.Duration
	afterConversionRetentionDuration time.Duration
	queueSize                        int

	headPersistenceDuration prometheus.Histogram
	querierMetrics          *querier.Metrics
}

// NewQueryableStorageWithWriteNotifier - QueryableStorage constructor.
func NewQueryableStorageWithWriteNotifier(
	blockWriter BlockWriter,
	registerer prometheus.Registerer,
	querierMetrics *querier.Metrics,
	writeNotifier WriteNotifier,
	clock clockwork.Clock,
	cooldownDuration time.Duration,
	retentionDuration time.Duration,
	afterConversionRetentionDuration time.Duration,
	queueSize int,
	heads ...relabeler.Head,
) *QueryableStorage {
	factory := util.NewUnconflictRegisterer(registerer)
	persistableHeads := make([]*WritableHead, 0, len(heads))
	for _, h := range heads {
		persistableHeads = append(persistableHeads, &WritableHead{
			Head:              h,
			writeBlockedUntil: clock.Now(),
			writeDeadline:     clock.Now().Add(retentionDuration),
			flushed:           false,
			rotated:           false,
		})
	}
	qs := &QueryableStorage{
		blockWriter:                      blockWriter,
		writeNotifier:                    writeNotifier,
		heads:                            persistableHeads,
		closer:                           util.NewCloser(),
		trigger:                          make(chan struct{}, 1),
		clock:                            clock,
		cooldownDuration:                 cooldownDuration,
		retentionDuration:                retentionDuration,
		afterConversionRetentionDuration: afterConversionRetentionDuration,
		queueSize:                        queueSize,
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
		querierMetrics: querierMetrics,
	}

	return qs
}

// Run loop for converting heads.
func (qs *QueryableStorage) Run() {
	go qs.loop()
}

func (qs *QueryableStorage) loop() {
	defer qs.closer.Done()

	select {
	case <-qs.clock.After(qs.initialDelay):
	case <-qs.closer.Signal():
		return
	}

	timer := qs.clock.NewTimer(0)
	timer.Stop()

	var nextDeadline *time.Time
	for {

		nextDeadline = qs.write()
		if nextDeadline != nil {
			timer.Reset(qs.clock.Until(*nextDeadline))
		} else {

		}

		select {
		case <-qs.trigger:
		case <-timer.Chan():
		case <-qs.closer.Signal():
			return
		}
	}
}

func (qs *QueryableStorage) write() (nextDeadline *time.Time) {
	qs.mtx.Lock()
	lenHeads := len(qs.heads)
	if lenHeads == 0 {
		// quick exit
		qs.mtx.Unlock()
		return nil
	}
	writableHeads := make([]*WritableHead, lenHeads)
	copy(writableHeads, qs.heads)
	qs.mtx.Unlock()

	shouldNotify := false
	deadliner := newDeadliner()
	var toDelete []*WritableHead
	displaceUntilIndex := len(writableHeads) - qs.queueSize
	for index, writableHead := range writableHeads {
		start := qs.clock.Now()

		if index < displaceUntilIndex {
			_ = writableHead.Flush()
			_ = writableHead.Rotate()
			_ = writableHead.Close()
			_ = writableHead.Discard()
			logger.Warnf("QUERYABLE STORAGE: head %s closed and discarded without conversion (displaced)", writableHead.String())
			toDelete = append(toDelete, writableHead)
			continue
		}

		if writableHead.IsOutdated(qs.clock.Now()) {
			_ = writableHead.Flush()
			_ = writableHead.Rotate()
			_ = writableHead.Close()
			_ = writableHead.Discard()
			logger.Warnf("QUERYABLE STORAGE: head %s closed and discarded without conversion (outdated)", writableHead.String())
			toDelete = append(toDelete, writableHead)
			continue
		}

		if err := writableHead.Flush(); err != nil {
			logger.Errorf("QUERYABLE STORAGE: failed to flush head %s: %s", writableHead.String(), err.Error())
			deadliner.Add(qs.clock.Now().Add(flushRotateCloseDiscardTimeout))
			continue
		}

		if err := writableHead.Rotate(); err != nil {
			logger.Errorf("QUERYABLE STORAGE: failed to rotate head %s: %s", writableHead.String(), err.Error())
			deadliner.Add(qs.clock.Now().Add(flushRotateCloseDiscardTimeout))
			continue
		}

		if writableHead.IsReadyForConversion(qs.clock.Now()) {
			if err := writableHead.Convert(qs.blockWriter); err != nil {
				logger.Errorf("QUERYABLE STORAGE: failed to convert head %s: %s", writableHead.String(), err.Error())
				deadliner.Add(qs.clock.Now().Add(writeRetryTimeout))
				continue
			}
		} else {

		}

		qs.headPersistenceDuration.Observe(float64(qs.clock.Since(start).Milliseconds()))
		shouldNotify = true
		logger.Infof("QUERYABLE STORAGE: head %s persisted, duration: %v", writableHead.String(), qs.clock.Since(start))

		if err := writableHead.CloseAndDiscard(); err != nil {
			logger.Errorf("QUERYABLE STORAGE: failed to write head %s: %s", writableHead.String(), err.Error())
			deadliner.Add(qs.clock.Now().Add(flushRotateCloseDiscardTimeout))
			continue
		}

		toDelete = append(toDelete, writableHead)
	}

	if shouldNotify {
		qs.writeNotifier.NotifyWritten()
	}

	qs.delete(toDelete)

	return deadliner.Deadline()
}

// Add - Storage interface implementation.
func (qs *QueryableStorage) Add(head relabeler.Head) {
	qs.mtx.Lock()
	writableHead := &WritableHead{
		Head:              head,
		writeBlockedUntil: qs.clock.Now().Add(qs.cooldownDuration),
		writeDeadline:     qs.clock.Now().Add(qs.retentionDuration),
	}
	qs.heads = append(qs.heads, writableHead)
	qs.mtx.Unlock()
	qs.triggerWrite()
}

func (qs *QueryableStorage) delete(heads []*WritableHead) {
	qs.mtx.Lock()
	defer qs.mtx.Unlock()

	headMap := make(map[string]struct{})
	for _, h := range heads {
		headMap[h.ID()] = struct{}{}
	}

	var result []*WritableHead
	for _, h := range qs.heads {
		if _, ok := headMap[h.ID()]; ok {
			continue
		}
		result = append(result, h)
	}

	qs.heads = result
}

func (qs *QueryableStorage) triggerWrite() {
	select {
	case qs.trigger <- struct{}{}:
	default:
	}
}

func (qs *QueryableStorage) Close() error {
	return qs.closer.Close()
}

// WriteMetrics - MetricWriterTarget interface implementation.
func (qs *QueryableStorage) WriteMetrics() {
	qs.mtx.Lock()
	heads := make([]*WritableHead, len(qs.heads))
	copy(heads, qs.heads)
	qs.mtx.Unlock()

	for _, head := range heads {
		head.WriteMetrics()
	}
}

// Querier - storage.Queryable interface implementation.
func (qs *QueryableStorage) Querier(mint, maxt int64) (storage.Querier, error) {
	qs.mtx.Lock()
	heads := make([]relabeler.Head, len(qs.heads))
	for _, wh := range qs.heads {
		heads = append(heads, wh)
	}
	qs.mtx.Unlock()

	var queriers []storage.Querier
	for _, head := range heads {
		h := head
		queriers = append(
			queriers,
			querier.NewQuerier(
				h,
				querier.NoOpShardedDeduplicatorFactory(),
				mint,
				maxt,
				nil,
				qs.querierMetrics,
			),
		)
	}

	q := querier.NewMultiQuerier(
		queriers,
		nil,
	)

	return q, nil
}

func (qs *QueryableStorage) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	qs.mtx.Lock()
	heads := make([]relabeler.Head, len(qs.heads))
	for _, wh := range qs.heads {
		heads = append(heads, wh)
	}
	qs.mtx.Unlock()

	var queriers []storage.ChunkQuerier
	for _, head := range heads {
		h := head
		queriers = append(
			queriers,
			querier.NewChunkQuerier(
				h,
				querier.NoOpShardedDeduplicatorFactory(),
				mint,
				maxt,
				nil,
			),
		)
	}

	return storage.NewMergeChunkQuerier(nil,
		queriers,
		storage.NewConcatenatingChunkSeriesMerger(),
	), nil
}

type noOpWriteNotifier struct{}

func (noOpWriteNotifier) NotifyWritten() {}

type Deadliner struct {
	deadline *time.Time
}

func newDeadliner() *Deadliner {
	return &Deadliner{}
}

func (d *Deadliner) Add(deadline time.Time) {
	if d.deadline == nil || deadline.Before(*d.deadline) {
		d.deadline = &deadline
	}
}

func (d *Deadliner) Deadline() *time.Time {
	return d.deadline
}
