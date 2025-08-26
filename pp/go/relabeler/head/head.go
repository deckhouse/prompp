package head

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/prometheus/pp/go/util/locker"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/util"
)

var (

	// ExtraReadConcurrency number of concurrency read operation, 0 - work without concurrency.
	ExtraReadConcurrency = 0

	UnrecoverableErrorChan = make(chan error)
)

// RelabelerData data for relabeling - inputRelabelers per shard and state.
type RelabelerData struct {
	state           *cppbridge.State
	inputRelabelers []*cppbridge.InputPerShardRelabeler
}

// NewRelabelerData init new RelabelerData.
func NewRelabelerData(
	rCfgs []*cppbridge.RelabelConfig,
	generationHead uint64,
	numberOfShards uint16,
) (*RelabelerData, error) {
	rd := &RelabelerData{
		inputRelabelers: make([]*cppbridge.InputPerShardRelabeler, 0, numberOfShards),
	}

	if err := rd.Reconfigure(rCfgs, generationHead, numberOfShards); err != nil {
		return nil, err
	}

	return rd, nil
}

// State return State of relabeler.
func (rd *RelabelerData) State(generationHead uint64) *cppbridge.State {
	if rd.state == nil {
		rd.state = cppbridge.NewState(uint16(len(rd.inputRelabelers))) // #nosec G115 // no overflow
		rd.state.Reconfigure(
			rd.generationRelabeler(),
			generationHead,
			uint16(len(rd.inputRelabelers)), // #nosec G115 // no overflow
		)
	}

	return rd.state
}

// InputRelabelerByShard return InputRelabeler by shard.
func (rd *RelabelerData) InputRelabelerByShard(shardID uint16) *cppbridge.InputPerShardRelabeler {
	return rd.inputRelabelers[shardID]
}

// Reconfigure update configuration on InputRelabeler and State.
func (rd *RelabelerData) Reconfigure(
	rCfgs []*cppbridge.RelabelConfig,
	generationHead uint64,
	numberOfShards uint16,
) error {
	if err := rd.reconfigureInputRelabelers(rCfgs, numberOfShards); err != nil {
		return err
	}

	if rd.state != nil {
		rd.state.Reconfigure(rd.generationRelabeler(), generationHead, numberOfShards)
	}

	return nil
}

// generationRelabeler return current(shardID == 0) relabeler's generation.
func (rd *RelabelerData) generationRelabeler() uint64 {
	return rd.inputRelabelers[0].Generation()
}

// Reconfigure update configuration on InputRelabeler.
func (rd *RelabelerData) reconfigureInputRelabelers(
	rCfgs []*cppbridge.RelabelConfig,
	numberOfShards uint16,
) error {
	sr, err := rd.updateOrCreateStatelessRelabeler(rCfgs)
	if err != nil {
		return err
	}

	if len(rd.inputRelabelers) == int(numberOfShards) {
		return nil
	}

	if len(rd.inputRelabelers) > int(numberOfShards) {
		// cut
		rd.inputRelabelers = rd.inputRelabelers[:numberOfShards]
	}

	if len(rd.inputRelabelers) < int(numberOfShards) {
		// grow
		rd.inputRelabelers = append(
			rd.inputRelabelers,
			make([]*cppbridge.InputPerShardRelabeler, int(numberOfShards)-len(rd.inputRelabelers))...,
		)
	}

	for shardID := range rd.inputRelabelers {
		if rd.inputRelabelers[shardID] != nil {
			// TODO may be depreacate ResetTo and always recreate
			rd.inputRelabelers[shardID].ResetTo(numberOfShards)
			continue
		}

		if rd.inputRelabelers[shardID], err = cppbridge.NewInputPerShardRelabeler(
			sr,
			numberOfShards,
			uint16(shardID), // #nosec G115 // no overflow
		); err != nil {
			return err
		}
	}

	return nil
}

// updateOrCreateStatelessRelabeler check inputRelabeler(shardID == 0) for key
// and update configs for StatelessRelabeler, if not exist - create new.
func (rd *RelabelerData) updateOrCreateStatelessRelabeler(
	rCfgs []*cppbridge.RelabelConfig,
) (*cppbridge.StatelessRelabeler, error) {
	if len(rd.inputRelabelers) == 0 || rd.inputRelabelers[0] == nil {
		return cppbridge.NewStatelessRelabeler(rCfgs)
	}

	sr := rd.inputRelabelers[0].StatelessRelabeler()
	if sr.EqualConfigs(rCfgs) {
		return sr, nil
	}

	if err := sr.ResetTo(rCfgs); err != nil {
		return nil, err
	}

	return sr, nil
}

type LastAppendedSegmentIDSetter interface {
	SetLastAppendedSegmentID(segmentID uint32)
}

type NoOpLastAppendedSegmentIDSetter struct{}

func (NoOpLastAppendedSegmentIDSetter) SetLastAppendedSegmentID(segmentID uint32) {}

type Head struct {
	id         string
	dir        string
	generation uint64
	readOnly   bool

	relabelersData map[string]*RelabelerData
	rdMutex        sync.Mutex

	shards             []*shard
	lssTaskChs         []chan *relabeler.GenericTask
	dataStorageTaskChs []chan *relabeler.GenericTask
	queryLocker        *locker.Weighted

	numberOfShards uint16
	stopc          chan struct{}
	wg             sync.WaitGroup

	// stat
	appendedSegmentCount prometheus.Counter
	memoryInUse          *prometheus.GaugeVec
	series               prometheus.Gauge
	walSize              *prometheus.GaugeVec
	// TODO refactoring
	queueLSS         *prometheus.GaugeVec
	queueDataStorage *prometheus.GaugeVec

	tasksCreated *prometheus.CounterVec
	tasksDone    *prometheus.CounterVec
	tasksLive    *prometheus.CounterVec
	tasksExecute *prometheus.CounterVec
}

func New(
	id string,
	dir string,
	generation uint64,
	inputRelabelerConfigs []*config.InputRelabelerConfig,
	lsses []*LSS,
	wals []*ShardWal,
	dataStorages []*DataStorage,
	unloadedDataStorages []*UnloadedDataStorage,
	queriedSeriesStorages []*QueriedSeriesStorage,
	numberOfShards uint16,
	registerer prometheus.Registerer,
) (*Head, error) {
	lssTaskChs := make([]chan *relabeler.GenericTask, numberOfShards)
	dataStorageTaskChs := make([]chan *relabeler.GenericTask, numberOfShards)
	shards := make([]*shard, numberOfShards)

	// current head workers concurrency
	concurrency := calculateHeadConcurrency(numberOfShards)

	for shardID := uint16(0); shardID < numberOfShards; shardID++ {
		lssTaskChs[shardID] = make(chan *relabeler.GenericTask, 4*concurrency)         // x4 for back pressure
		dataStorageTaskChs[shardID] = make(chan *relabeler.GenericTask, 4*concurrency) // x4 for back pressure
		shards[shardID] = newShard(
			lsses[shardID],
			dataStorages[shardID],
			unloadedDataStorages[shardID],
			queriedSeriesStorages[shardID],
			wals[shardID],
			shardID,
			ExtraReadConcurrency != 0,
		)
	}

	factory := util.NewUnconflictRegisterer(registerer)
	h := &Head{
		id:                 id,
		dir:                dir,
		generation:         generation,
		shards:             shards,
		lssTaskChs:         lssTaskChs,
		dataStorageTaskChs: dataStorageTaskChs,
		queryLocker:        locker.NewWeighted(2 * concurrency), // x2 for back pressure

		stopc:          make(chan struct{}),
		wg:             sync.WaitGroup{},
		relabelersData: make(map[string]*RelabelerData, len(inputRelabelerConfigs)),
		numberOfShards: numberOfShards,
		// stat
		appendedSegmentCount: factory.NewCounter(prometheus.CounterOpts{
			Name: "prompp_appended_segment_count",
			Help: "Number of appended segments.",
		}),
		memoryInUse: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_head_cgo_memory_bytes",
				Help: "Current value memory in use in bytes.",
			},
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

		queueLSS: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_head_queue_lss_tasks_size",
				Help: "The size of the queue lss tasks of the current head.",
			},
			[]string{"shard_id"},
		),
		queueDataStorage: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_head_queue_data_storage_tasks_size",
				Help: "The size of the queue data storage tasks of the current head.",
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

	if err := h.reconfigure(inputRelabelerConfigs, numberOfShards); err != nil {
		return nil, err
	}

	h.run()

	runtime.SetFinalizer(h, func(h *Head) {
		logger.Debugf("head {%d} destroyed", generation)
	})

	return h, nil
}

func (h *Head) ID() string {
	return h.id
}

func (h *Head) Generation() uint64 {
	return h.generation
}

// String serialize as string.
func (h *Head) String() string {
	return fmt.Sprintf("{ id: %s, generation: %d }", h.id, h.generation)
}

func (h *Head) CommitToWal() error {
	if h.readOnly {
		return fmt.Errorf("committing read only head")
	}

	t := h.CreateTask(
		relabeler.LSSWalCommit,
		func(shard relabeler.Shard) error {
			shard.LSSLock()
			defer shard.LSSUnlock()

			return shard.Wal().Commit()
		},
		relabeler.ForLSSTask,
	)
	h.Enqueue(t)

	return t.Wait()
}

func (h *Head) Flush() error {
	t := h.CreateTask(
		relabeler.LSSWalFlush,
		func(shard relabeler.Shard) error {
			shard.LSSLock()
			defer shard.LSSUnlock()

			return shard.Wal().Flush()
		},
		relabeler.ForLSSTask,
	)
	h.Enqueue(t)

	return t.Wait()
}

// MergeOutOfOrderChunks merge chunks with out of order data chunks.
func (h *Head) MergeOutOfOrderChunks() {
	t := h.CreateTask(
		relabeler.DSMergeOutOfOrderChunks,
		func(shard relabeler.Shard) error {
			shard.DataStorageLock()
			shard.DataStorage().MergeOutOfOrderChunks()
			shard.DataStorageUnlock()

			return nil
		},
		relabeler.ForDataStorageTask,
	)
	h.Enqueue(t)

	_ = t.Wait()
}

func (h *Head) NumberOfShards() uint16 {
	return h.numberOfShards
}

func (h *Head) Stop() {
	if h.readOnly {
		return
	}
	h.readOnly = true

	release, _ := h.queryLocker.LockWithPriority(context.Background())
	h.queryLocker.Resize(10 * h.Concurrency()) // x10 readonly
	h.stop()
	release()

	generationStr := strconv.FormatUint(h.generation, 10)
	for relabelerID := range h.relabelersData {
		// clear unnecessary
		h.memoryInUse.DeletePartialMatch(prometheus.Labels{
			"generation": generationStr,
			"allocator":  fmt.Sprintf("input_relabeler_%s", relabelerID),
		})
	}
	h.relabelersData = nil
}

func (h *Head) Reconfigure(
	ctx context.Context,
	inputRelabelerConfigs []*config.InputRelabelerConfig,
	numberOfShards uint16,
) error {
	if h.readOnly {
		return fmt.Errorf("reconfiguring read only head")
	}

	release, err := h.queryLocker.LockWithPriority(ctx)
	if err != nil {
		return fmt.Errorf("[Head] Reconfigure: query locker: %w", err)
	}
	defer release()

	h.queryLocker.Resize(2 * h.Concurrency()) // x2 for back pressure
	h.stop()
	if err := h.reconfigure(inputRelabelerConfigs, numberOfShards); err != nil {
		return err
	}
	h.run()

	return nil
}

func (h *Head) WriteMetrics(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	status := h.Status(0)
	h.series.Set(float64(status.HeadStats.NumSeries))

	if ctx.Err() != nil {
		return
	}

	generationStr := strconv.FormatUint(h.generation, 10)
	tw := relabeler.NewTaskWaiter(2)

	tDataStorageHeadAllocatedMemory := h.CreateTask(
		relabeler.DSAllocatedMemory,
		func(shard relabeler.Shard) error {
			shard.DataStorageRLock()
			am := shard.DataStorage().AllocatedMemory()
			shard.DataStorageRUnlock()

			h.memoryInUse.With(
				prometheus.Labels{
					"generation": generationStr,
					"allocator":  "data_storage",
					"id":         strconv.FormatUint(uint64(shard.ShardID()), 10),
				},
			).Set(float64(am))

			return nil
		},
		relabeler.ForDataStorageTask,
	)
	h.Enqueue(tDataStorageHeadAllocatedMemory)

	tLSSHeadAllocatedMemory := h.CreateTask(
		relabeler.LSSAllocatedMemory,
		func(shard relabeler.Shard) error {
			shard.LSSRLock()
			am := shard.LSS().AllocatedMemory()
			shard.LSSRUnlock()

			h.memoryInUse.With(
				prometheus.Labels{
					"generation": generationStr,
					"allocator":  "main_lss",
					"id":         strconv.FormatUint(uint64(shard.ShardID()), 10),
				},
			).Set(float64(am))

			return nil
		},
		relabeler.ForLSSTask,
	)
	h.Enqueue(tLSSHeadAllocatedMemory)

	tw.Add(tLSSHeadAllocatedMemory)
	tw.Add(tDataStorageHeadAllocatedMemory)
	_ = tw.Wait()

	if h.readOnly {
		return
	}

	if ctx.Err() != nil {
		return
	}

	// do not write metrics if the head is read-only.
	for shardID := uint16(0); shardID < h.numberOfShards; shardID++ {
		shardIDStr := strconv.FormatUint(uint64(shardID), 10)

		h.walSize.With(
			prometheus.Labels{"shard_id": shardIDStr},
		).Set(float64(h.shards[shardID].wal.CurrentSize()))

		h.queueLSS.With(prometheus.Labels{
			"shard_id": shardIDStr,
		}).Set(float64(len(h.lssTaskChs[shardID])))

		h.queueDataStorage.With(prometheus.Labels{
			"shard_id": shardIDStr,
		}).Set(float64(len(h.dataStorageTaskChs[shardID])))
	}
}

func (h *Head) Status(limit int) relabeler.HeadStatus {
	shardStatuses := make([]*cppbridge.HeadStatus, h.NumberOfShards())
	for i := range shardStatuses {
		shardStatuses[i] = cppbridge.NewHeadStatus()
	}

	tw := relabeler.NewTaskWaiter(2)

	tLSSHeadStatus := h.CreateTask(
		relabeler.LSSHeadStatus,
		func(shard relabeler.Shard) error {
			shard.LSSRLock()
			shardStatuses[shard.ShardID()].FromLSS(shard.LSS().Raw(), limit)
			shard.LSSRUnlock()

			return nil
		},
		relabeler.ForLSSTask,
	)
	h.Enqueue(tLSSHeadStatus)

	if limit != 0 {
		tDataStorageHeadStatus := h.CreateTask(
			relabeler.DSHeadStatus,
			func(shard relabeler.Shard) error {
				shard.DataStorageRLock()
				shardStatuses[shard.ShardID()].FromDataStorage(shard.DataStorage().Raw())
				shard.DataStorageRUnlock()

				return nil
			},
			relabeler.ForDataStorageTask,
		)
		h.Enqueue(tDataStorageHeadStatus)
		tw.Add(tDataStorageHeadStatus)
	}

	tw.Add(tLSSHeadStatus)
	_ = tw.Wait()

	headStatus := relabeler.HeadStatus{
		HeadStats: relabeler.HeadStats{
			MinTime: math.MaxInt64,
			MaxTime: math.MinInt64,
		},
	}

	seriesStats := make(map[string]uint64)
	labelsStats := make(map[string]uint64)
	memoryStats := make(map[string]uint64)
	countStats := make(map[string]uint64)

	for _, shardStatus := range shardStatuses {
		headStatus.HeadStats.NumSeries += uint64(shardStatus.NumSeries)
		if limit == 0 {
			continue
		}

		headStatus.HeadStats.ChunkCount += int64(shardStatus.ChunkCount)
		if headStatus.HeadStats.MaxTime < shardStatus.TimeInterval.Max {
			headStatus.HeadStats.MaxTime = shardStatus.TimeInterval.Max
		}
		if headStatus.HeadStats.MinTime > shardStatus.TimeInterval.Min {
			headStatus.HeadStats.MinTime = shardStatus.TimeInterval.Min
		}

		headStatus.HeadStats.NumLabelPairs += int(shardStatus.NumLabelPairs)

		for _, stat := range shardStatus.SeriesCountByMetricName {
			seriesStats[stat.Name] += uint64(stat.Count)
		}
		for _, stat := range shardStatus.LabelValueCountByLabelName {
			labelsStats[stat.Name] += uint64(stat.Count)
		}
		for _, stat := range shardStatus.MemoryInBytesByLabelName {
			memoryStats[stat.Name] += uint64(stat.Size)
		}
		for _, stat := range shardStatus.SeriesCountByLabelValuePair {
			countStats[stat.Name+"="+stat.Value] += uint64(stat.Count)
		}
	}

	if limit == 0 {
		return headStatus
	}

	headStatus.SeriesCountByMetricName = getSortedStats(seriesStats, limit)
	headStatus.LabelValueCountByLabelName = getSortedStats(labelsStats, limit)
	headStatus.MemoryInBytesByLabelName = getSortedStats(memoryStats, limit)
	headStatus.SeriesCountByLabelValuePair = getSortedStats(countStats, limit)

	return headStatus
}

func getSortedStats(stats map[string]uint64, limit int) []relabeler.HeadStat {
	result := make([]relabeler.HeadStat, 0, len(stats))
	for k, v := range stats {
		result = append(result, relabeler.HeadStat{
			Name:  k,
			Value: v,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Value > result[j].Value
	})

	if len(result) > limit {
		return result[:limit]
	}

	return result
}

// Rotate head. Implementation relabeler.Head.
func (*Head) Rotate() error {
	return nil
}

// CopySeriesFrom copy series from other head.
func (h *Head) CopySeriesFrom(other relabeler.Head) {
	t := other.CreateTask(
		relabeler.LSSCopyAddedSeries,
		func(shard relabeler.Shard) error {
			shard.LSSRLock()
			shard.LSS().Raw().CopyAddedSeries(h.shards[shard.ShardID()].lss.Raw())
			shard.LSSRUnlock()

			return nil
		},
		relabeler.ForLSSTask,
	)
	h.Enqueue(t)
	_ = t.Wait()
}

// Close wals and clear metrics.
func (h *Head) Close() error {
	h.memoryInUse.DeletePartialMatch(prometheus.Labels{"generation": strconv.FormatUint(h.generation, 10)})

	var err error
	for _, s := range h.shards {
		err = errors.Join(err, s.wal.Close())

		if s.unloadedDataStorage != nil {
			err = errors.Join(s.unloadedDataStorage.Close())
		}

		if s.queriedSeriesStorage != nil {
			err = errors.Join(s.queriedSeriesStorage.Close())
		}
	}
	return err
}

func (h *Head) Discard() error {
	return nil
}

func (h *Head) stop() {
	close(h.stopc)
	h.wg.Wait()
	h.stopc = make(chan struct{})
}

// CreateTask create a task for operations on the head shards.
func (h *Head) CreateTask(
	taskName string,
	fn relabeler.ShardFn,
	onLss bool,
) *relabeler.GenericTask {
	if h.readOnly {
		return relabeler.NewReadOnlyGenericTask(fn)
	}

	ls := prometheus.Labels{"type_task": taskName}
	return relabeler.NewGenericTask(
		fn,
		h.tasksCreated.With(ls),
		h.tasksDone.With(ls),
		h.tasksLive.With(ls),
		h.tasksExecute.With(ls),
		onLss,
	)
}

// Enqueue the task to be executed on head.
func (h *Head) Enqueue(t *relabeler.GenericTask) {
	t.SetShardsNumber(h.numberOfShards)

	if h.readOnly {
		h.readOnlyForEachShard(t)
		return
	}

	if t.ForLSS() {
		for _, taskCh := range h.lssTaskChs {
			taskCh <- t
		}
	} else {
		for _, taskCh := range h.dataStorageTaskChs {
			taskCh <- t
		}
	}
}

// EnqueueOnShard the task to be executed on head on specific shard.
func (h *Head) EnqueueOnShard(t *relabeler.GenericTask, shardID uint16) {
	t.SetShardsNumber(1)

	if h.readOnly {
		h.readOnlyOnShard(t, h.shards[shardID])
		return
	}

	if t.ForLSS() {
		h.lssTaskChs[shardID] <- t
	} else {
		h.dataStorageTaskChs[shardID] <- t
	}
}

// RLockQuery locks for query to [Head].
func (h *Head) RLockQuery(ctx context.Context) (runlock func(), err error) {
	return h.queryLocker.RLock(ctx)
}

// Concurrency return current head workers concurrency.
func (h *Head) Concurrency() int64 {
	return calculateHeadConcurrency(h.numberOfShards)
}

// readOnlyForEachShard run generic task on read only head without queue on all shards.
func (h *Head) readOnlyForEachShard(t *relabeler.GenericTask) {
	for _, s := range h.shards {
		h.readOnlyOnShard(t, s)
	}
}

// readOnlyOnShard run generic task on read only head without queue on specific shard.
func (h *Head) readOnlyOnShard(t *relabeler.GenericTask, s *shard) {
	go func(sd *shard) { t.ExecuteOnShard(sd) }(s)
}

// Append incoming data to head.
func (h *Head) Append(
	ctx context.Context,
	incomingData *relabeler.IncomingData,
	incomingState *cppbridge.State,
	relabelerID string,
	commitToWal bool,
) ([][]*cppbridge.InnerSeries, cppbridge.RelabelerStats, error) {
	if h.readOnly {
		return nil, cppbridge.RelabelerStats{}, fmt.Errorf("appending to read only head")
	}

	rd, state, err := h.resolveRelabelersData(incomingState, relabelerID)
	if err != nil {
		return nil, cppbridge.RelabelerStats{}, err
	}

	shardedInnerSeries := NewShardedInnerSeries(h.numberOfShards)
	shardedRelabeledSeries := NewShardedRelabeledSeries(h.numberOfShards)

	stats, err := h.inputRelabelingStage(
		ctx,
		state,
		rd,
		relabeler.NewDestructibleIncomingData(incomingData, int(h.numberOfShards)),
		shardedInnerSeries,
		shardedRelabeledSeries,
	)
	if err != nil {
		// reset msr.rotateWG on error
		return nil, stats, fmt.Errorf("failed input relabeling stage: %w", err)
	}

	if !shardedRelabeledSeries.IsEmpty() {
		shardedStateUpdates := NewShardedStateUpdates(h.numberOfShards)
		if err = h.appendRelabelerSeriesStage(
			ctx,
			rd,
			shardedInnerSeries,
			shardedRelabeledSeries,
			shardedStateUpdates,
		); err != nil {
			return nil, stats, fmt.Errorf("failed append relabeler series stage: %w", err)
		}

		if err = h.updateRelabelerStateStage(
			ctx,
			state,
			rd,
			shardedStateUpdates,
		); err != nil {
			return nil, stats, fmt.Errorf("failed update relabeler stage: %w", err)
		}
	}

	tw := relabeler.NewTaskWaiter(2)

	tAppend := h.CreateTask(
		relabeler.DSAppendInnerSeries,
		func(shard relabeler.Shard) error {
			shard.DataStorageLock()
			shard.DataStorage().AppendInnerSeriesSlice(shardedInnerSeries.DataByShard(shard.ShardID()))
			shard.DataStorageUnlock()

			return nil
		},
		relabeler.ForDataStorageTask,
	)
	h.Enqueue(tAppend)

	var atomiclimitExhausted uint32
	tWalWrite := h.CreateTask(
		relabeler.LSSWalWrite,
		func(shard relabeler.Shard) error {
			shard.LSSLock()
			limitExhausted, errWrite := shard.Wal().Write(shardedInnerSeries.DataByShard(shard.ShardID()))
			shard.LSSUnlock()
			if errWrite != nil {
				return fmt.Errorf("shard %d: %w", shard.ShardID(), errWrite)
			}

			if limitExhausted {
				atomic.AddUint32(&atomiclimitExhausted, 1)
			}

			return nil
		},
		relabeler.ForLSSTask,
	)
	h.Enqueue(tWalWrite)

	tw.Add(tAppend)
	tw.Add(tWalWrite)

	if err := tw.Wait(); err != nil {
		logger.Errorf("failed to write wal: %v", err)
	}

	if commitToWal || atomiclimitExhausted > 0 {
		t := h.CreateTask(
			relabeler.LSSWalCommit,
			func(shard relabeler.Shard) error {
				shard.LSSLock()
				defer shard.LSSUnlock()

				return shard.Wal().Commit()
			},
			relabeler.ForLSSTask,
		)
		h.Enqueue(t)

		if err := t.Wait(); err != nil {
			logger.Errorf("failed to commit wal: %v", err)
		}
	}

	return shardedInnerSeries.Data(), stats, nil
}

func (h *Head) resolveRelabelersData(
	state *cppbridge.State,
	relabelerID string,
) (*RelabelerData, *cppbridge.State, error) {
	h.rdMutex.Lock()
	defer h.rdMutex.Unlock()

	rd, ok := h.relabelersData[relabelerID]
	if !ok {
		return nil, nil, fmt.Errorf("relabeler ID not exist: %s", relabelerID)
	}

	if state != nil {
		state.Reconfigure(rd.generationRelabeler(), h.generation, h.numberOfShards)
	}

	if state == nil {
		state = rd.State(h.generation)
	}

	return rd, state, nil
}

// inputRelabelingStage first stage - relabeling.
func (h *Head) inputRelabelingStage(
	ctx context.Context,
	state *cppbridge.State,
	rd *RelabelerData,
	incomingData *relabeler.DestructibleIncomingData,
	shardedInnerSeries *ShardedInnerSeries,
	shardedRelabeledSeries *ShardedRelabeledSeries,
) (cppbridge.RelabelerStats, error) {
	stats := make([]cppbridge.RelabelerStats, h.numberOfShards)
	t := h.CreateTask(
		relabeler.LSSInputRelabeling,
		func(shard relabeler.Shard) error {
			var (
				shardID          = shard.ShardID()
				err              error
				hasReallocations bool
				ok               bool
			)

			shard.LSSRLock()
			if state.TrackStaleness() {
				stats[shardID], ok, err = rd.InputRelabelerByShard(
					shardID,
				).InputRelabelingWithStalenansFromCache(
					ctx,
					shard.LSS().Input(),
					shard.LSS().Target(),
					state.CacheByShard(shardID),
					state.RelabelerOptions(),
					state.StaleNansStateByShard(shardID),
					state.DefTimestamp(),
					incomingData.Data().ShardedData(),
					shardedInnerSeries.DataBySourceShard(shardID),
				)
			} else {
				stats[shardID], ok, err = rd.InputRelabelerByShard(shardID).InputRelabelingFromCache(
					ctx,
					shard.LSS().Input(),
					shard.LSS().Target(),
					state.CacheByShard(shardID),
					state.RelabelerOptions(),
					incomingData.Data().ShardedData(),
					shardedInnerSeries.DataBySourceShard(shardID),
				)
			}
			shard.LSSRUnlock()

			if err != nil {
				incomingData.Destroy()
				return fmt.Errorf("shard %d: %w", shardID, err)
			}

			if ok {
				incomingData.Destroy()
				return nil
			}

			shard.LSSLock()
			defer shard.LSSUnlock()
			rstats := cppbridge.RelabelerStats{}

			if state.TrackStaleness() {
				rstats, hasReallocations, err = rd.InputRelabelerByShard(shardID).InputRelabelingWithStalenans(
					ctx,
					shard.LSS().Input(),
					shard.LSS().Target(),
					state.CacheByShard(shardID),
					state.RelabelerOptions(),
					state.StaleNansStateByShard(shardID),
					state.DefTimestamp(),
					incomingData.Data().ShardedData(),
					shardedInnerSeries.DataBySourceShard(shardID),
					shardedRelabeledSeries.DataByShard(shardID),
				)
			} else {
				rstats, hasReallocations, err = rd.InputRelabelerByShard(shardID).InputRelabeling(
					ctx,
					shard.LSS().Input(),
					shard.LSS().Target(),
					state.CacheByShard(shardID),
					state.RelabelerOptions(),
					incomingData.Data().ShardedData(),
					shardedInnerSeries.DataBySourceShard(shardID),
					shardedRelabeledSeries.DataByShard(shardID),
				)
			}

			incomingData.Destroy()
			if err != nil {
				return fmt.Errorf("shard %d: %w", shardID, err)
			}

			stats[shardID].SamplesAdded += rstats.SamplesAdded
			stats[shardID].SeriesAdded += rstats.SeriesAdded
			stats[shardID].SeriesDrop += rstats.SeriesDrop

			if hasReallocations {
				shard.LSS().ResetSnapshot()
			}

			return nil
		},
		relabeler.ForLSSTask,
	)
	h.Enqueue(t)

	resStats := cppbridge.RelabelerStats{}
	if err := t.Wait(); err != nil {
		return resStats, err
	}

	for _, s := range stats {
		resStats.SamplesAdded += s.SamplesAdded
		resStats.SeriesAdded += s.SeriesAdded
		resStats.SeriesDrop += s.SeriesDrop
	}

	return resStats, nil
}

// appendRelabelerSeriesStage second stage - append to lss relabeling ls.
func (h *Head) appendRelabelerSeriesStage(
	ctx context.Context,
	rd *RelabelerData,
	shardedInnerSeries *ShardedInnerSeries,
	shardedRelabeledSeries *ShardedRelabeledSeries,
	shardedStateUpdates *ShardedStateUpdates,
) error {
	t := h.CreateTask(
		relabeler.LSSAppendRelabelerSeries,
		func(shard relabeler.Shard) error {
			shardID := shard.ShardID()
			relabeledSeries, ok := shardedRelabeledSeries.DataBySourceShard(shardID)
			if !ok {
				return nil
			}

			shard.LSSLock()
			defer shard.LSSUnlock()

			hasReallocations, err := rd.InputRelabelerByShard(shardID).AppendRelabelerSeries(
				ctx,
				shard.LSS().Target(),
				shardedInnerSeries.DataByShard(shardID),
				relabeledSeries,
				shardedStateUpdates.DataByShard(shardID),
			)
			if err != nil {
				return fmt.Errorf("shard %d: %w", shardID, err)
			}

			if hasReallocations {
				shard.LSS().ResetSnapshot()
			}

			return nil
		},
		relabeler.ForLSSTask,
	)
	h.Enqueue(t)

	return t.Wait()
}

// updateRelabelerStateStage third stage - update state cache.
func (h *Head) updateRelabelerStateStage(
	ctx context.Context,
	state *cppbridge.State,
	rd *RelabelerData,
	shardedStateUpdates *ShardedStateUpdates,
) error {
	for shardID := uint16(0); shardID < h.numberOfShards; shardID++ {
		updates, ok := shardedStateUpdates.DataBySourceShard(shardID)
		if !ok {
			continue
		}

		err := rd.InputRelabelerByShard(shardID).UpdateRelabelerState(
			ctx,
			state.CacheByShard(shardID),
			updates,
		)
		if err != nil {
			return fmt.Errorf("shard %d: %w", shardID, err)
		}
	}

	return nil
}

// run loop for each shard.
func (h *Head) run() {
	workers := 1 + ExtraReadConcurrency
	h.wg.Add(2 * workers * int(h.numberOfShards))
	for shardID := uint16(0); shardID < h.numberOfShards; shardID++ {
		for i := 0; i < workers; i++ {
			go func(sid uint16) {
				defer h.wg.Done()
				h.shardLoop(h.lssTaskChs[sid], h.stopc, h.shards[sid])
			}(shardID)

			go func(sid uint16) {
				defer h.wg.Done()
				h.shardLoop(h.dataStorageTaskChs[sid], h.stopc, h.shards[sid])
			}(shardID)
		}
	}
}

// shardLoop run shard loop for operation.
func (*Head) shardLoop(
	taskCH chan *relabeler.GenericTask,
	stopc chan struct{},
	s *shard,
) {
	for {
		select {
		case <-stopc:
			return

		case task := <-taskCH:
			task.ExecuteOnShard(s)
		}
	}
}

// UnloadUnusedSeriesData - unload unused series data in all dataStorages
func (h *Head) UnloadUnusedSeriesData() {
	task := h.CreateTask(
		relabeler.DSUnloadUnusedSeriesData,
		func(shard relabeler.Shard) error {
			if shard.UnloadedDataStorage() == nil {
				return nil
			}

			unloader := shard.DataStorage().CreateUnusedSeriesDataUnloader()

			shard.DataStorageRLock()
			snapshot := unloader.CreateSnapshot()
			queriedSeries := shard.DataStorage().GetQueriedSeriesBitset()
			shard.DataStorageRUnlock()

			header, err := shard.UnloadedDataStorage().WriteSnapshot(snapshot)
			if err != nil {
				return fmt.Errorf("unable to write unloaded series data snapshot: %v", err)
			}

			shard.DataStorageLock()
			shard.UnloadedDataStorage().WriteIndex(header)
			unloader.Unload()
			shard.DataStorageUnlock()

			if err = shard.QueriedSeriesStorage().Write(queriedSeries, time.Now().UnixMilli()); err != nil {
				return fmt.Errorf("unable to write queried series data: %v", err)
			}

			return nil
		},
		relabeler.ForDataStorageTask,
	)
	h.Enqueue(task)
	if err := task.Wait(); err != nil {
		logger.Warnf("unable to unload unused series data: %v", err)
	}
}

// CreateDataStorageLoadAndQueryTask - add querier to pool for data load and create task for data load if needed
func (h *Head) CreateDataStorageLoadAndQueryTask(shardID uint16, querier uintptr) *relabeler.GenericTask {
	return h.shards[shardID].loadAndQueryTask.Add(querier, func() *relabeler.GenericTask {
		task := h.CreateTask(
			relabeler.DSLoadUnusedSeriesDataAndQuery,
			func(shard relabeler.Shard) error {
				shard.DataStorageLock()
				queriers := shard.LoadAndQueryTask().Release()
				loader := shard.DataStorage().CreateLoader(queriers)
				err := shard.UnloadedDataStorage().ForEachSnapshot(func(snapshot []byte, isLast bool) {
					loader.Load(snapshot, isLast)
				})
				shard.DataStorageUnlock()

				if err != nil {
					return err
				}

				shard.DataStorageRLock()
				shard.DataStorage().QueryFinal(queriers)
				shard.DataStorageRUnlock()

				return err
			},
			relabeler.ForDataStorageTask,
		)
		h.EnqueueOnShard(task, shardID)
		return task
	})
}

// calculateHeadConcurrency calculate current head workers concurrency.
func calculateHeadConcurrency(numberOfShards uint16) int64 {
	// 2 - lss and datastorage
	return 2 * int64(1+ExtraReadConcurrency) * int64(numberOfShards)
}

// Raw returns raw [Head].
func (h *Head) Raw() relabeler.Head {
	return h
}

func (h *Head) UnrecoverableError(err error) {
	logger.Warnf("Unrecoverable error: %v", err)

	UnrecoverableErrorChan <- UnrecoverableError{err}
}

// UnrecoverableError error if Head get unrecoverable error.
type UnrecoverableError struct {
	err error
}

// Error implements error.
func (err UnrecoverableError) Error() string {
	return fmt.Sprintf("Unrecoverable error: %v", err.err)
}

// Is implements errors.Is interface.
func (UnrecoverableError) Is(target error) bool {
	_, ok := target.(UnrecoverableError)
	return ok
}
