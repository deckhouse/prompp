package appender

import (
	"context"
	"sync"
	"sync/atomic"
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
	writeRetryTimeout        = 5 * time.Minute
	maxAddIter        uint32 = 5
)

type WriteNotifier interface {
	NotifyWritten()
}

// BlockWriter writes block on disk.
type BlockWriter interface {
	Write(block block.Block) error
}

// QueryableStorage hold reference to finalized heads and writes blocks from them. Also allows query not yet not
// persisted heads.
type QueryableStorage struct {
	blockWriter          BlockWriter
	writeNotifier        WriteNotifier
	mtx                  sync.Mutex
	heads                []relabeler.Head
	headRetentionTimeout time.Duration

	writeTimer   clockwork.Timer
	writeTimeout time.Duration
	addCount     uint32
	closer       *util.Closer

	clock                   clockwork.Clock
	maxRetentionDuration    time.Duration
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
	maxRetentionDuration time.Duration,
	headRetentionTimeout time.Duration,
	writeTimeout time.Duration,
	heads ...relabeler.Head,
) *QueryableStorage {
	factory := util.NewUnconflictRegisterer(registerer)
	qs := &QueryableStorage{
		blockWriter:          blockWriter,
		writeNotifier:        writeNotifier,
		heads:                heads,
		writeTimer:           clock.NewTimer(0),
		writeTimeout:         writeTimeout,
		closer:               util.NewCloser(),
		clock:                clock,
		maxRetentionDuration: maxRetentionDuration,
		headRetentionTimeout: headRetentionTimeout,
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

	// skip 0 start
	<-qs.writeTimer.Chan()

	return qs
}

// Run loop for converting heads.
func (qs *QueryableStorage) Run() {
	go qs.loop()
}

func (qs *QueryableStorage) loop() {
	defer qs.closer.Done()

	retryTimer := qs.clock.NewTimer(0)
	// skip 0 start
	<-retryTimer.Chan()

	for {
		if !qs.write() {
			if !retryTimer.Stop() {
				// in the new version of go cleaning of C is not required
				select {
				case <-retryTimer.Chan():
				default:
				}
			}

			// try write after timeout
			retryTimer.Reset(writeRetryTimeout)
		}

		select {
		case <-qs.writeTimer.Chan():
			atomic.StoreUint32(&qs.addCount, 0)
		case <-retryTimer.Chan():
		case <-qs.closer.Signal():
			logger.Infof("QUERYABLE STORAGE: done")
			return
		}
	}
}

func (qs *QueryableStorage) write() bool {
	qs.mtx.Lock()
	lenHeads := len(qs.heads)
	if lenHeads == 0 {
		// quick exit
		qs.mtx.Unlock()
		return true
	}
	heads := make([]relabeler.Head, lenHeads)
	copy(heads, qs.heads)
	qs.mtx.Unlock()

	successful := true
	shouldNotify := false
	persisted := make([]string, 0, lenHeads)
	for _, head := range heads {
		start := qs.clock.Now()
		if qs.headIsOutdated(head) {
			persisted = append(persisted, head.ID())
			shouldNotify = true
			continue
		}
		if err := head.Flush(); err != nil {
			logger.Errorf("QUERYABLE STORAGE: failed to flush head %s: %s", head.String(), err.Error())
			successful = false
			continue
		}
		if err := head.Rotate(); err != nil {
			logger.Errorf("QUERYABLE STORAGE: failed to rotate head %s: %s", head.String(), err.Error())
			successful = false
			continue
		}

		tBlockWrite := head.CreateTask(
			relabeler.BlockWrite,
			func(shard relabeler.Shard) error {
				shard.LSSLock()
				defer shard.LSSUnlock()

				return qs.blockWriter.Write(relabeler.NewBlock(shard.LSS().Raw(), shard.DataStorage().Raw()))
			},
			relabeler.ForLSSTask,
		)
		head.Enqueue(tBlockWrite)
		if err := tBlockWrite.Wait(); err != nil {
			logger.Errorf("QUERYABLE STORAGE: failed to write head %s: %s", head.String(), err.Error())
			successful = false
			continue
		}

		qs.headPersistenceDuration.Observe(float64(qs.clock.Since(start).Milliseconds()))
		persisted = append(persisted, head.ID())
		shouldNotify = true
		logger.Infof("QUERYABLE STORAGE: head %s persisted, duration: %v", head.String(), qs.clock.Since(start))
	}

	if shouldNotify {
		qs.writeNotifier.NotifyWritten()
	}

	time.AfterFunc(qs.headRetentionTimeout, func() {
		select {
		case <-qs.closer.Signal():
			return
		default:
			qs.shrink(persisted...)
		}
	})

	return successful
}

func (qs *QueryableStorage) headIsOutdated(head relabeler.Head) bool {
	headMaxTimestampMs := head.Status(1).HeadStats.MaxTime
	return qs.clock.Now().Sub(time.Unix(headMaxTimestampMs/1000, 0)) > qs.maxRetentionDuration
}

// Add - Storage interface implementation.
func (qs *QueryableStorage) Add(head relabeler.Head) {
	qs.mtx.Lock()
	qs.heads = append(qs.heads, head)
	logger.Infof("QUERYABLE STORAGE: head %s added", head.String())
	qs.mtx.Unlock()

	if atomic.AddUint32(&qs.addCount, 1) < maxAddIter {
		qs.writeTimer.Reset(qs.writeTimeout)
	}
}

func (qs *QueryableStorage) Close() error {
	return qs.closer.Close()
}

// WriteMetrics - MetricWriterTarget interface implementation.
func (qs *QueryableStorage) WriteMetrics(ctx context.Context) {
	qs.mtx.Lock()
	heads := make([]relabeler.Head, len(qs.heads))
	copy(heads, qs.heads)
	qs.mtx.Unlock()

	for _, head := range heads {
		head.WriteMetrics(ctx)
	}
}

// Querier - storage.Queryable interface implementation.
func (qs *QueryableStorage) Querier(mint, maxt int64) (storage.Querier, error) {
	qs.mtx.Lock()
	heads := make([]relabeler.Head, len(qs.heads))
	copy(heads, qs.heads)
	qs.mtx.Unlock()

	var queriers []storage.Querier
	for _, head := range heads {
		h := head.Raw()
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
	copy(heads, qs.heads)
	qs.mtx.Unlock()

	var queriers []storage.ChunkQuerier
	for _, head := range heads {
		h := head.Raw()
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

func (qs *QueryableStorage) shrink(persisted ...string) {
	qs.mtx.Lock()
	defer qs.mtx.Unlock()

	persistedMap := make(map[string]struct{})
	for _, headID := range persisted {
		persistedMap[headID] = struct{}{}
	}

	var heads []relabeler.Head
	for _, head := range qs.heads {
		if _, ok := persistedMap[head.ID()]; ok {
			_ = head.Close()
			_ = head.Discard()
			logger.Infof("QUERYABLE STORAGE: head %s persisted, closed and discarded", head.String())
			continue
		}
		heads = append(heads, head)
	}
	qs.heads = heads
}

type noOpWriteNotifier struct{}

func (noOpWriteNotifier) NotifyWritten() {}
