package appender

import (
	"errors"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/prometheus/pp/go/relabeler/querier"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/prometheus/storage"
)

const (
	DefaultInitialDelay       = time.Minute
	DefaultProcessingInterval = time.Minute
	DefaultQueueSize          = 1
)

type WriteNotifier interface {
	NotifyWritten()
}

type WritableHead struct {
	relabeler.Head
	processingDeadline time.Time
	deleteAt           *time.Time
	flushed            bool
	rotated            bool
	converted          bool
}

func (h *WritableHead) Flush() error {
	if !h.flushed {
		logger.Debugf("flushing: %s", h.String())
		if err := h.Head.Flush(); err != nil {
			return err
		}
		h.flushed = true
	}
	return nil
}

func (h *WritableHead) Rotate() error {
	if !h.rotated {
		logger.Debugf("rotating: %s", h.String())
		if err := h.Head.Rotate(); err != nil {
			return err
		}
		h.rotated = true
	}
	return nil
}

func (h *WritableHead) IsOutdated(now time.Time) bool {
	logger.Debugf("now: %v, processingDeadline: %v", now, h.processingDeadline)
	return now.After(h.processingDeadline)
}

func (h *WritableHead) Convert(blockWriter relabeler.BlockWriter) (bool, error) {
	previousState := h.converted
	if !h.converted {
		logger.Debugf("converting: %s", h.String())
		if err := h.Head.WriteTo(blockWriter); err != nil {
			return false, err
		}
		h.converted = true
	}

	return previousState != h.converted, nil
}

func (h *WritableHead) SetDeleteAt(deleteAt time.Time) {
	h.processingDeadline = deleteAt
	h.deleteAt = &deleteAt
}

func (h *WritableHead) IsDeletable(now time.Time) bool {
	return h.deleteAt != nil && now.After(*h.deleteAt)
}

func (h *WritableHead) Converted() bool {
	return h.converted
}

// QueryableStorage hold reference to finalized heads and writes blocks from them. Also allows query not yet not
// persisted heads.
type QueryableStorage struct {
	blockWriter   relabeler.BlockWriter
	writeNotifier WriteNotifier
	mtx           sync.Mutex
	heads         []*WritableHead

	closer *util.Closer

	clock                            clockwork.Clock
	initialDelay                     time.Duration
	processingInterval               time.Duration
	retentionDuration                time.Duration
	afterConversionRetentionDuration time.Duration
	queueSize                        int

	headPersistenceDuration prometheus.Histogram
	querierMetrics          *querier.Metrics
}

// NewQueryableStorageWithWriteNotifier - QueryableStorage constructor.
func NewQueryableStorageWithWriteNotifier(
	blockWriter relabeler.BlockWriter,
	registerer prometheus.Registerer,
	querierMetrics *querier.Metrics,
	writeNotifier WriteNotifier,
	clock clockwork.Clock,
	initialDelay time.Duration,
	processingInterval time.Duration,
	retentionDuration time.Duration,
	afterConversionRetentionDuration time.Duration,
	queueSize int,
	heads ...relabeler.Head,
) *QueryableStorage {
	factory := util.NewUnconflictRegisterer(registerer)
	persistableHeads := make([]*WritableHead, 0, len(heads))
	for _, h := range heads {
		stats := h.Status(1)
		persistableHeads = append(persistableHeads, &WritableHead{
			Head:               h,
			processingDeadline: time.UnixMilli(stats.HeadStats.MaxTime).Add(retentionDuration),
		})
	}
	qs := &QueryableStorage{
		blockWriter:                      blockWriter,
		writeNotifier:                    writeNotifier,
		heads:                            persistableHeads,
		closer:                           util.NewCloser(),
		clock:                            clock,
		initialDelay:                     initialDelay,
		processingInterval:               processingInterval,
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

	ticker := qs.clock.NewTicker(qs.processingInterval)
	defer ticker.Stop()

	var toBeSkippedTickCount int
	for {

		if toBeSkippedTickCount == 0 {
			start := qs.clock.Now()
			qs.process()
			duration := qs.clock.Since(start)
			if duration >= (qs.processingInterval / 2) {
				toBeSkippedTickCount = int(duration/qs.processingInterval) + 1
			}
		} else {
			toBeSkippedTickCount -= 1
		}

		select {
		case <-ticker.Chan():
		case <-qs.closer.Signal():
			return
		}
	}
}

func (qs *QueryableStorage) process() {
	qs.mtx.Lock()
	lenHeads := len(qs.heads)
	if lenHeads == 0 {
		// quick exit
		qs.mtx.Unlock()
		return
	}
	writableHeads := make([]*WritableHead, lenHeads)
	copy(writableHeads, qs.heads)
	qs.mtx.Unlock()

	shouldNotify := false
	var toDelete []*WritableHead
	displaceUntilIndex := len(writableHeads) - qs.queueSize
	for index, writableHead := range writableHeads {
		displaceable := index < displaceUntilIndex
		processed, converted := qs.ProcessHead(writableHead, displaceable)
		if converted && !shouldNotify {
			shouldNotify = true
		}

		if processed {
			toDelete = append(toDelete, writableHead)
			logger.Debugf("head is added to delete list: %s", writableHead.String())
		}
	}

	if shouldNotify {
		qs.writeNotifier.NotifyWritten()
	}

	qs.delete(toDelete)
}

func (qs *QueryableStorage) ProcessHead(writableHead *WritableHead, displaceable bool) (processed bool, converted bool) {
	logger.Debugf("deletable check: %s", writableHead.String())
	if writableHead.IsDeletable(qs.clock.Now()) {
		logger.Debugf("deletable check failed: head is deletable: %s", writableHead.String())
		err := errors.Join(
			writableHead.Flush(),
			writableHead.Rotate(),
			writableHead.Close(),
			writableHead.Discard(),
		)
		if err != nil {
			logger.Errorf("QUERYABLE STORAGE: something happened during head close: %v", err)
		}
		logger.Infof("QUERYABLE STORAGE: head %s closed and discarded", writableHead.String())
		return true, false
	}

	logger.Debugf("outdated check: %s", writableHead.String())
	if writableHead.IsOutdated(qs.clock.Now()) {
		logger.Debugf("outdated check failed: head is outdated: %s", writableHead.String())
		err := errors.Join(
			writableHead.Flush(),
			writableHead.Rotate(),
			writableHead.Close(),
			writableHead.Discard(),
		)
		if err != nil {
			logger.Errorf("QUERYABLE STORAGE: something happened during head close: %v", err)
		}
		logger.Warnf("QUERYABLE STORAGE: head %s closed and discarded without conversion (outdated)", writableHead.String())
		return true, false
	}

	logger.Debugf("displaced check: %s", writableHead.String())
	if displaceable && !writableHead.Converted() {
		logger.Debugf("displaced check failed: head is displaced: %s", writableHead.String())
		err := errors.Join(
			writableHead.Flush(),
			writableHead.Rotate(),
			writableHead.Close(),
			writableHead.Discard(),
		)
		if err != nil {
			logger.Errorf("QUERYABLE STORAGE: something happened during head close: %v", err)
		}
		logger.Warnf("QUERYABLE STORAGE: head %s closed and discarded without conversion (displaced)", writableHead.String())
		return true, false
	}

	if err := writableHead.Flush(); err != nil {
		logger.Errorf("QUERYABLE STORAGE: failed to flush head %s: %s", writableHead.String(), err.Error())
		return false, false
	}

	if err := writableHead.Rotate(); err != nil {
		logger.Errorf("QUERYABLE STORAGE: failed to rotate head %s: %s", writableHead.String(), err.Error())
		return false, false
	}
	start := qs.clock.Now()
	converted, err := writableHead.Convert(qs.blockWriter)
	if err != nil {
		logger.Errorf("QUERYABLE STORAGE: failed to convert head %s: %s", writableHead.String(), err.Error())
		return false, false
	}

	if converted {
		qs.headPersistenceDuration.Observe(float64(qs.clock.Since(start).Milliseconds()))
		logger.Infof("QUERYABLE STORAGE: head %s persisted, duration: %v", writableHead.String(), qs.clock.Since(start))
		writableHead.SetDeleteAt(qs.clock.Now().Add(qs.afterConversionRetentionDuration))
		return false, true
	}

	return false, false
}

func (qs *QueryableStorage) NewWritableHead(head relabeler.Head) *WritableHead {
	stats := head.Status(1)
	return &WritableHead{
		Head:               head,
		processingDeadline: time.UnixMilli(stats.HeadStats.MaxTime).Add(qs.retentionDuration),
	}
}

// Add - Storage interface implementation.
func (qs *QueryableStorage) Add(head relabeler.Head) {
	writableHead := qs.NewWritableHead(head)
	qs.mtx.Lock()
	qs.heads = append(qs.heads, writableHead)
	qs.mtx.Unlock()
}

func (qs *QueryableStorage) delete(heads []*WritableHead) {
	if len(heads) == 0 {
		return
	}

	qs.mtx.Lock()
	defer qs.mtx.Unlock()

	toDeleteMap := make(map[string]struct{})
	for _, h := range heads {
		toDeleteMap[h.ID()] = struct{}{}
	}

	var result []*WritableHead
	for _, h := range qs.heads {
		if _, ok := toDeleteMap[h.ID()]; ok {
			continue
		}
		result = append(result, h)
	}

	qs.heads = result
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
