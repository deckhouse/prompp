package head

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/storage/head/task"
	"github.com/prometheus/prometheus/pp/go/storage/logger"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/prometheus/pp/go/util/locker"
)

// ExtraWorkers number of extra workers for operation on shards.
var ExtraWorkers = 0

// defaultNumberOfWorkers default number of workers.
const defaultNumberOfWorkers = 2

// const (
// 	// DSAllocatedMemory name of task.
// 	DSAllocatedMemory = "data_storage_allocated_memory"

// 	// DSHeadStatus name of task.
// 	DSHeadStatus = "data_storage_head_status"

// 	// LSSAllocatedMemory name of task.
// 	LSSAllocatedMemory = "lss_allocated_memory"

// 	// LSSHeadStatus name of task.
// 	LSSHeadStatus = "lss_head_status"
// )

//
// Shard
//

// Shard the minimum required head Shard implementation.
type Shard interface {
	// LSS() *LSS

	// ShardID returns the shard ID.
	ShardID() uint16

	// Close closes the wal segmentWriter.
	Close() error
}

//
// Head
//

type Head[TShard Shard, TGorutineShard Shard] struct {
	id         string
	generation uint64

	gshardCtor     func(TShard) TGorutineShard
	shards         []TShard
	taskChs        []chan *task.Generic[TGorutineShard]
	querySemaphore *locker.Weighted

	stopc chan struct{}
	wg    sync.WaitGroup

	readOnly       uint32
	numberOfShards uint16

	// stat
	memoryInUse *prometheus.GaugeVec
	series      prometheus.Gauge
	walSize     *prometheus.GaugeVec
	queueSize   *prometheus.GaugeVec

	tasksCreated *prometheus.CounterVec
	tasksDone    *prometheus.CounterVec
	tasksLive    *prometheus.CounterVec
	tasksExecute *prometheus.CounterVec
}

// NewHead init new [Head].
func NewHead[TShard Shard, TGoroutineShard Shard](
	id string,
	shards []TShard,
	gshardCtor func(TShard) TGoroutineShard,
	numberOfShards uint16,
	registerer prometheus.Registerer,
) *Head[TShard, TGoroutineShard] {
	taskChs := make([]chan *task.Generic[TGoroutineShard], numberOfShards)
	concurrency := calculateHeadConcurrency(numberOfShards) // current head workers concurrency

	for shardID := uint16(0); shardID < numberOfShards; shardID++ {
		taskChs[shardID] = make(chan *task.Generic[TGoroutineShard], 4*concurrency) // x4 for back pressure
	}

	factory := util.NewUnconflictRegisterer(registerer)
	h := &Head[TShard, TGoroutineShard]{
		id:             id,
		gshardCtor:     gshardCtor,
		shards:         shards,
		taskChs:        taskChs,
		numberOfShards: uint16(len(shards)), // #nosec G115 // no overflow
		memoryInUse: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_head_cgo_memory_bytes",
				Help: "Current value memory in use in bytes.",
			},
			// TODO generation -> h.id
			[]string{"generation", "allocator", "id"},
		),
		series: factory.NewGauge(prometheus.GaugeOpts{
			Name: "prompp_head_series",
			Help: "Total number of series in the heads block.",
		}),
		walSize: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_head_current_wal_size",
				Help: "The size of the wall of the current head.",
			},
			[]string{"shard_id"},
		),

		queueSize: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_head_queue_tasks_size",
				Help: "The size of the queue of tasks of the current head.",
			},
			[]string{"shard_id"},
		),

		tasksCreated: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "prompp_head_task_created_count",
				Help: "Number of created tasks.",
			},
			[]string{"type_task"},
		),
		tasksDone: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "prompp_head_task_done_count",
				Help: "Number of done tasks.",
			},
			[]string{"type_task"},
		),
		tasksLive: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "prompp_head_task_live_duration_microseconds_sum",
				Help: "The duration of the live task in microseconds.",
			},
			[]string{"type_task"},
		),
		tasksExecute: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "prompp_head_task_execute_duration_microseconds_sum",
				Help: "The duration of the task execution in microseconds.",
			},
			[]string{"type_task"},
		),
	}

	h.run()

	runtime.SetFinalizer(h, func(h *Head[TShard, TGoroutineShard]) {
		logger.Debugf("head %s destroyed", h.String())
	})

	return h
}

// AcquireQuery acquires the [Head] semaphore with a weight of 1,
// blocking until resources are available or ctx is done.
// On success, returns nil. On failure, returns ctx.Err() and leaves the semaphore unchanged.
func (h *Head[TShard, TGorutineShard]) AcquireQuery(ctx context.Context) (release func(), err error) {
	return h.querySemaphore.RLock(ctx)
}

// Close wals and clear metrics.
func (h *Head[TShard, TGorutineShard]) Close() error {
	h.memoryInUse.DeletePartialMatch(prometheus.Labels{"generation": strconv.FormatUint(h.generation, 10)})

	close(h.stopc)
	h.wg.Wait()

	var err error
	for _, s := range h.shards {
		err = errors.Join(err, s.Close())
	}

	return err
}

// Concurrency return current head workers concurrency.
func (h *Head[TShard, TGorutineShard]) Concurrency() int64 {
	return calculateHeadConcurrency(h.numberOfShards)
}

// CreateTask create a task for operations on the [Head] shards.
func (h *Head[TShard, TGorutineShard]) CreateTask(
	taskName string,
	shardFn func(shard TGorutineShard) error,
) *task.Generic[TGorutineShard] {
	ls := prometheus.Labels{"type_task": taskName}

	return task.NewGeneric(
		shardFn,
		h.tasksCreated.With(ls),
		h.tasksDone.With(ls),
		h.tasksLive.With(ls),
		h.tasksExecute.With(ls),
	)
}

// Enqueue the task to be executed on shards [Head].
func (h *Head[TShard, TGorutineShard]) Enqueue(t *task.Generic[TGorutineShard]) {
	t.SetShardsNumber(h.numberOfShards)

	for _, taskCh := range h.taskChs {
		taskCh <- t
	}
}

// EnqueueOnShard the task to be executed on head on specific shard.
func (h *Head[TShard, TGorutineShard]) EnqueueOnShard(t *task.Generic[TGorutineShard], shardID uint16) {
	t.SetShardsNumber(1)

	h.taskChs[shardID] <- t
}

// ID returns id [Head].
func (h *Head[TShard, TGorutineShard]) ID() string {
	return h.id
}

// IsReadOnly returns true if the [Head] has switched to read-only.
func (h *Head[TShard, TGorutineShard]) IsReadOnly() bool {
	return atomic.LoadUint32(&h.readOnly) > 0
}

// NumberOfShards returns current number of shards in to [Head].
func (h *Head[TShard, TGorutineShard]) NumberOfShards() uint16 {
	return h.numberOfShards
}

// SetReadOnly sets the read-only flag for the [Head].
func (h *Head[TShard, TGorutineShard]) SetReadOnly() {
	atomic.StoreUint32(&h.readOnly, 1)
}

// String serialize as string.
func (h *Head[TShard, TGorutineShard]) String() string {
	return fmt.Sprintf("{id: %s, generation: %d}", h.id, h.generation)
}

// run loop for each shard.
func (h *Head[TShard, TGorutineShard]) run() {
	workers := defaultNumberOfWorkers + ExtraWorkers
	h.wg.Add(workers * int(h.numberOfShards))
	for shardID := uint16(0); shardID < h.numberOfShards; shardID++ {
		for i := 0; i < workers; i++ {
			go func(sid uint16) {
				defer h.wg.Done()
				h.shardLoop(h.taskChs[sid], h.stopc, h.shards[sid])
			}(shardID)
		}
	}
}

// shardLoop run shard loop for operation.
func (h *Head[TShard, TGorutineShard]) shardLoop(
	taskCH chan *task.Generic[TGorutineShard],
	stopc chan struct{},
	s TShard,
) {
	// TODO PerGoroutineRelabeler
	pgs := h.gshardCtor(s)

	for {
		select {
		case <-stopc:
			return

		case t := <-taskCH:
			t.ExecuteOnShard(pgs)
		}
	}
}

// calculateHeadConcurrency calculate current head workers concurrency.
func calculateHeadConcurrency(numberOfShards uint16) int64 {
	return int64(defaultNumberOfWorkers+ExtraWorkers) * int64(numberOfShards)
}

// TODO Flush CommitToWal ?

// TODO Who?
// // getSortedStats returns sorted statistics for the [Head].
// func getSortedStats(stats map[string]uint64, limit int) []storage.HeadStat {
// 	result := make([]storage.HeadStat, 0, len(stats))
// 	for k, v := range stats {
// 		result = append(result, storage.HeadStat{
// 			Name:  k,
// 			Value: v,
// 		})
// 	}

// 	sort.Slice(result, func(i, j int) bool {
// 		return result[i].Value > result[j].Value
// 	})

// 	if len(result) > limit {
// 		return result[:limit]
// 	}

// 	return result
// }

// func (h *Head[TShard, TGorutineShard]) Status(limit int) storage.HeadStatus {
// 	shardStatuses := make([]*cppbridge.HeadStatus, h.NumberOfShards())
// 	for i := range shardStatuses {
// 		shardStatuses[i] = cppbridge.NewHeadStatus()
// 	}

// 	tw := task.NewTaskWaiter[*task.Generic[TGorutineShard]](2)

// 	tLSSHeadStatus := h.CreateTask(
// 		LSSHeadStatus,
// 		func(shard TGorutineShard) error {
// 			shard.LSSRLock()
// 			shardStatuses[shard.ShardID()].FromLSS(shard.LSS().Raw(), limit)
// 			shard.LSSRUnlock()

// 			return nil
// 		},
// 	)
// 	h.Enqueue(tLSSHeadStatus)

// 	if limit != 0 {
// 		tDataStorageHeadStatus := h.CreateTask(
// 			DSHeadStatus,
// 			func(shard TGorutineShard) error {
// 				shard.DataStorageRLock()
// 				shardStatuses[shard.ShardID()].FromDataStorage(shard.DataStorage().Raw())
// 				shard.DataStorageRUnlock()

// 				return nil
// 			},
// 		)
// 		h.Enqueue(tDataStorageHeadStatus)
// 		tw.Add(tDataStorageHeadStatus)
// 	}

// 	tw.Add(tLSSHeadStatus)
// 	_ = tw.Wait()

// 	headStatus := storage.HeadStatus{
// 		HeadStats: storage.HeadStats{
// 			MinTime: math.MaxInt64,
// 			MaxTime: math.MinInt64,
// 		},
// 	}

// 	seriesStats := make(map[string]uint64)
// 	labelsStats := make(map[string]uint64)
// 	memoryStats := make(map[string]uint64)
// 	countStats := make(map[string]uint64)

// 	for _, shardStatus := range shardStatuses {
// 		headStatus.HeadStats.NumSeries += uint64(shardStatus.NumSeries)
// 		if limit == 0 {
// 			continue
// 		}

// 		headStatus.HeadStats.ChunkCount += int64(shardStatus.ChunkCount)
// 		if headStatus.HeadStats.MaxTime < shardStatus.TimeInterval.Max {
// 			headStatus.HeadStats.MaxTime = shardStatus.TimeInterval.Max
// 		}
// 		if headStatus.HeadStats.MinTime > shardStatus.TimeInterval.Min {
// 			headStatus.HeadStats.MinTime = shardStatus.TimeInterval.Min
// 		}

// 		headStatus.HeadStats.NumLabelPairs += int(shardStatus.NumLabelPairs)

// 		for _, stat := range shardStatus.SeriesCountByMetricName {
// 			seriesStats[stat.Name] += uint64(stat.Count)
// 		}
// 		for _, stat := range shardStatus.LabelValueCountByLabelName {
// 			labelsStats[stat.Name] += uint64(stat.Count)
// 		}
// 		for _, stat := range shardStatus.MemoryInBytesByLabelName {
// 			memoryStats[stat.Name] += uint64(stat.Size)
// 		}
// 		for _, stat := range shardStatus.SeriesCountByLabelValuePair {
// 			countStats[stat.Name+"="+stat.Value] += uint64(stat.Count)
// 		}
// 	}

// 	if limit == 0 {
// 		return headStatus
// 	}

// 	headStatus.SeriesCountByMetricName = getSortedStats(seriesStats, limit)
// 	headStatus.LabelValueCountByLabelName = getSortedStats(labelsStats, limit)
// 	headStatus.MemoryInBytesByLabelName = getSortedStats(memoryStats, limit)
// 	headStatus.SeriesCountByLabelValuePair = getSortedStats(countStats, limit)

// 	return headStatus
// }

// func (h *Head[TShard, TGorutineShard]) WriteMetrics(ctx context.Context) {
// 	if ctx.Err() != nil {
// 		return
// 	}

// 	status := h.Status(0)
// 	h.series.Set(float64(status.HeadStats.NumSeries))

// 	if ctx.Err() != nil {
// 		return
// 	}

// 	generationStr := strconv.FormatUint(h.generation, 10)
// 	tw := task.NewTaskWaiter[*task.Generic[TGorutineShard]](2)

// 	tDataStorageHeadAllocatedMemory := h.CreateTask(
// 		DSAllocatedMemory,
// 		func(shard TGorutineShard) error {
// 			shard.DataStorageRLock()
// 			am := shard.DataStorage().AllocatedMemory()
// 			shard.DataStorageRUnlock()

// 			h.memoryInUse.With(
// 				prometheus.Labels{
// 					"generation": generationStr,
// 					"allocator":  "data_storage",
// 					"id":         strconv.FormatUint(uint64(shard.ShardID()), 10),
// 				},
// 			).Set(float64(am))

// 			return nil
// 		},
// 	)
// 	h.Enqueue(tDataStorageHeadAllocatedMemory)

// 	tLSSHeadAllocatedMemory := h.CreateTask(
// 		LSSAllocatedMemory,
// 		func(shard TGorutineShard) error {
// 			shard.LSSRLock()
// 			am := shard.LSS().AllocatedMemory()
// 			shard.LSSRUnlock()

// 			h.memoryInUse.With(
// 				prometheus.Labels{
// 					"generation": generationStr,
// 					"allocator":  "main_lss",
// 					"id":         strconv.FormatUint(uint64(shard.ShardID()), 10),
// 				},
// 			).Set(float64(am))

// 			return nil
// 		},
// 	)
// 	h.Enqueue(tLSSHeadAllocatedMemory)

// 	tw.Add(tLSSHeadAllocatedMemory)
// 	tw.Add(tDataStorageHeadAllocatedMemory)
// 	_ = tw.Wait()

// 	if h.readOnly {
// 		return
// 	}

// 	if ctx.Err() != nil {
// 		return
// 	}

// 	// do not write metrics if the head is read-only.
// 	for shardID := uint16(0); shardID < h.numberOfShards; shardID++ {
// 		shardIDStr := strconv.FormatUint(uint64(shardID), 10)

// 		h.walSize.With(
// 			prometheus.Labels{"shard_id": shardIDStr},
// 		).Set(float64(h.shards[shardID].wal.CurrentSize()))

// 		h.queueSize.With(prometheus.Labels{
// 			"shard_id": shardIDStr,
// 		}).Set(float64(len(h.taskChs[shardID])))
// 	}
// }
