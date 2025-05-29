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

	"github.com/prometheus/prometheus/pp/go/relabeler/logger"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/util"
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
	generation uint64
	readOnly   bool

	relabelersData map[string]*RelabelerData
	rdMutex        sync.Mutex

	dataStorages []*DataStorage
	wals         []*ShardWal
	lsses        []*LSS

	priotityTaskCh    []chan *GenericTask
	nonPriorityTaskCh []chan *GenericTask

	numberOfShards uint16
	stopc          chan struct{}
	wg             *sync.WaitGroup

	// stat
	registerer           prometheus.Registerer
	appendedSegmentCount prometheus.Counter
	memoryInUse          *prometheus.GaugeVec
	series               prometheus.Gauge
	queried              *prometheus.GaugeVec
	walSize              *prometheus.GaugeVec
	// TODO refactoring
	queuePriotity    *prometheus.GaugeVec
	queueNonPriority *prometheus.GaugeVec

	tasksCreated *prometheus.CounterVec
	tasksDone    *prometheus.CounterVec
	tasksLive    *prometheus.CounterVec
	tasksExecute *prometheus.CounterVec
}

func New(
	id string,
	generation uint64,
	inputRelabelerConfigs []*config.InputRelabelerConfig,
	lsses []*LSS,
	wals []*ShardWal,
	dataStorages []*DataStorage,
	numberOfShards uint16,
	registerer prometheus.Registerer,
) (*Head, error) {
	priotity := make([]chan *GenericTask, numberOfShards)
	nonPriority := make([]chan *GenericTask, numberOfShards)

	for shardID := uint16(0); shardID < numberOfShards; shardID++ {
		priotity[shardID] = make(chan *GenericTask, chanBufferSize)
		nonPriority[shardID] = make(chan *GenericTask, chanBufferSize)
	}

	factory := util.NewUnconflictRegisterer(registerer)
	h := &Head{
		id:           id,
		generation:   generation,
		lsses:        lsses,
		wals:         wals,
		dataStorages: dataStorages,

		priotityTaskCh:    priotity,
		nonPriorityTaskCh: nonPriority,

		stopc:          make(chan struct{}),
		wg:             &sync.WaitGroup{},
		relabelersData: make(map[string]*RelabelerData, len(inputRelabelerConfigs)),
		numberOfShards: numberOfShards,
		// stat
		registerer: registerer,
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
		queried: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_head_queried_series",
				Help: "Total number of queried series in the heads block.",
			},
			[]string{"caller"},
		),
		walSize: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_head_current_wal_size",
				Help: "The size of the wall of the current head.",
			},
			[]string{"shard_id"},
		),
		queuePriotity: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_head_queue_priotity_size",
				Help: "The size of the queue priotity of the current head.",
			},
			[]string{"shard_id"},
		),
		queueNonPriority: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_head_queue_non_priority_size",
				Help: "The size of the queue non priority of the current head.",
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
	return h.PriorityForEachShard(relabeler.CommitToWal, func(shard relabeler.Shard) error {
		return shard.Wal().Commit()
	})
}

func (h *Head) Flush() error {
	return h.PriorityForEachShard(relabeler.WalFlush, func(shard relabeler.Shard) error {
		return shard.Wal().Flush()
	})
}

func (h *Head) OnShard(shardID uint16, typeTask relabeler.TypeTask, fn relabeler.ShardFn) error {
	return h.onShard(shardID, typeTask, fn)
}

// MergeOutOfOrderChunks merge chunks with out of order data chunks.
func (h *Head) MergeOutOfOrderChunks() {
	_ = h.PriorityForEachShard(relabeler.DataStorageMergeOutOfOrderChunks, func(shard relabeler.Shard) error {
		shard.DataStorage().MergeOutOfOrderChunks()
		return nil
	})
}

func (h *Head) NumberOfShards() uint16 {
	return h.numberOfShards
}

func (h *Head) Stop() {
	if h.readOnly {
		return
	}
	h.readOnly = true
	h.stop()
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

func (h *Head) Reconfigure(inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16) error {
	if h.readOnly {
		return fmt.Errorf("reconfiguring read only head")
	}

	h.stop()
	if err := h.reconfigure(inputRelabelerConfigs, numberOfShards); err != nil {
		return err
	}
	h.run()
	return nil
}

func (h *Head) WriteMetrics() {
	status := h.Status(0)
	h.series.Set(float64(status.HeadStats.NumSeries))
	h.queried.With(
		prometheus.Labels{"caller": "rule"},
	).Set(float64(status.HeadStats.RuleQueriedSeries))
	h.queried.With(
		prometheus.Labels{"caller": "federate"},
	).Set(float64(status.HeadStats.FederateQueriedSeries))
	h.queried.With(
		prometheus.Labels{"caller": "other"},
	).Set(float64(status.HeadStats.OtherQueriedSeries))

	generationStr := strconv.FormatUint(h.generation, 10)
	_ = h.PriorityForEachShard(relabeler.HeadAllocatedMemory, func(shard relabeler.Shard) error {
		shardID := strconv.FormatUint(uint64(shard.ShardID()), 10)
		h.memoryInUse.With(
			prometheus.Labels{
				"generation": generationStr,
				"allocator":  "main_lss",
				"id":         shardID,
			},
		).Set(float64(shard.LSS().AllocatedMemory()))

		h.memoryInUse.With(
			prometheus.Labels{
				"generation": generationStr,
				"allocator":  "data_storage",
				"id":         shardID,
			},
		).Set(float64(shard.DataStorage().AllocatedMemory()))

		return nil
	})

	if h.readOnly {
		return
	}

	// do not write metrics if the head is read-only.
	for shardID := uint16(0); shardID < h.numberOfShards; shardID++ {
		shardIDStr := strconv.FormatUint(uint64(shardID), 10)

		h.walSize.With(
			prometheus.Labels{"shard_id": shardIDStr},
		).Set(float64(h.wals[shardID].CurrentSize()))

		h.queuePriotity.With(prometheus.Labels{
			"shard_id": shardIDStr,
		}).Set(float64(len(h.priotityTaskCh[shardID])))

		h.queueNonPriority.With(prometheus.Labels{
			"shard_id": shardIDStr,
		}).Set(float64(len(h.nonPriorityTaskCh[shardID])))
	}
}

func (h *Head) Status(limit int) relabeler.HeadStatus {
	shardStatuses := make([]*cppbridge.HeadStatus, h.NumberOfShards())
	_ = h.PriorityForEachShard(relabeler.HeadStatusType, func(shard relabeler.Shard) error {
		shardStatuses[shard.ShardID()] = cppbridge.GetHeadStatus(
			shard.LSS().Raw().Pointer(),
			shard.DataStorage().Raw().Pointer(),
			limit,
		)
		return nil
	})

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
		headStatus.HeadStats.RuleQueriedSeries += int64(shardStatus.RuleQueriedSeries)
		headStatus.HeadStats.FederateQueriedSeries += int64(shardStatus.FederateQueriedSeries)
		headStatus.HeadStats.OtherQueriedSeries += int64(shardStatus.OtherQueriedSeries)
		if limit == 0 {
			continue
		}

		headStatus.HeadStats.ChunkCount += int64(shardStatus.ChunkCount)
		headStatus.HeadStats.NumLabelPairs += int(shardStatus.NumLabelPairs)

		if headStatus.HeadStats.MaxTime < shardStatus.TimeInterval.Max {
			headStatus.HeadStats.MaxTime = shardStatus.TimeInterval.Max
		}
		if headStatus.HeadStats.MinTime > shardStatus.TimeInterval.Min {
			headStatus.HeadStats.MinTime = shardStatus.TimeInterval.Min
		}

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
	_ = other.PriorityForEachShard(relabeler.HeadCopyAddedSeries, func(shard relabeler.Shard) error {
		shard.LSS().Raw().CopyAddedSeries(h.lsses[shard.ShardID()].Raw())
		return nil
	})
}

// Close wals and clear metrics.
func (h *Head) Close() error {
	h.memoryInUse.DeletePartialMatch(prometheus.Labels{"generation": strconv.FormatUint(h.generation, 10)})
	var err error
	for _, wal := range h.wals {
		err = errors.Join(err, wal.Close())
	}
	return err
}

func (h *Head) Discard() error {
	return nil
}

func (h *Head) onShard(shardID uint16, typeTask relabeler.TypeTask, fn relabeler.ShardFn) error {
	if h.readOnly {
		s := &shard{
			id:          shardID,
			lss:         h.lsses[shardID],
			dataStorage: h.dataStorages[shardID],
		}

		return fn(s)
	}

	ls := prometheus.Labels{"type_task": typeTask.String()}
	task := NewSingleGenericTask(
		fn,
		h.tasksCreated.With(ls),
		h.tasksDone.With(ls),
		h.tasksLive.With(ls),
		h.tasksExecute.With(ls),
		h.numberOfShards,
	)
	h.priotityTaskCh[shardID] <- task

	return task.Wait()
}

func (h *Head) stop() {
	close(h.stopc)
	h.wg.Wait()
	h.stopc = make(chan struct{})
}

func (h *Head) run() {
	for shardID := uint16(0); shardID < h.numberOfShards; shardID++ {
		h.wg.Add(1)
		go func(shardID uint16) {
			defer h.wg.Done()
			h.shardLoop(shardID, h.priotityTaskCh[shardID], h.nonPriorityTaskCh[shardID], h.stopc)
		}(shardID)
	}
}

// PriorityForEachShard run func generic task on priority queue.
func (h *Head) PriorityForEachShard(typeTask relabeler.TypeTask, fn relabeler.ShardFn) error {
	if h.readOnly {
		return h.readOnlyForEachShard(NewReadOnlyGenericTask(fn, h.numberOfShards))
	}

	ls := prometheus.Labels{"type_task": typeTask.String()}
	t := NewGenericTask(
		fn,
		h.tasksCreated.With(ls),
		h.tasksDone.With(ls),
		h.tasksLive.With(ls),
		h.tasksExecute.With(ls),
		h.numberOfShards,
	)
	for _, priotityTaskCh := range h.priotityTaskCh {
		priotityTaskCh <- t
	}

	return t.Wait()
}

// NonPriorityForEachShard run func generic task on non priority queue.
func (h *Head) NonPriorityForEachShard(typeTask relabeler.TypeTask, fn relabeler.ShardFn) error {
	if h.readOnly {
		return h.readOnlyForEachShard(NewReadOnlyGenericTask(fn, h.numberOfShards))
	}

	ls := prometheus.Labels{"type_task": typeTask.String()}
	t := NewGenericTask(
		fn,
		h.tasksCreated.With(ls),
		h.tasksDone.With(ls),
		h.tasksLive.With(ls),
		h.tasksExecute.With(ls),
		h.numberOfShards,
	)
	for _, shardGenericReadTaskCh := range h.nonPriorityTaskCh {
		shardGenericReadTaskCh <- t
	}

	return t.Wait()
}

// readOnlyForEachShard run func generic task on read only head without queue.
func (h *Head) readOnlyForEachShard(t *GenericTask) error {
	for shardID := uint16(0); shardID < h.numberOfShards; shardID++ {
		s := &shard{
			id:          shardID,
			lss:         h.lsses[shardID],
			dataStorage: h.dataStorages[shardID],
			wal:         h.wals[shardID],
		}
		go func(shard *shard) {
			t.ExecuteOnShard(shard)
		}(s)
	}

	return t.Wait()
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

	var atomiclimitExhausted uint32

	err = h.PriorityForEachShard(relabeler.WalDataStorageAdd, func(shard relabeler.Shard) error {
		shard.DataStorage().AppendInnerSeriesSlice(shardedInnerSeries.DataByShard(shard.ShardID()))

		limitExhausted, errWrite := shard.Wal().Write(shardedInnerSeries.DataByShard(shard.ShardID()))
		if errWrite != nil {
			return fmt.Errorf("shard %d: %w", shard.ShardID(), errWrite)
		}

		if limitExhausted {
			atomic.AddUint32(&atomiclimitExhausted, 1)
		}

		return nil
	})
	if err != nil {
		logger.Errorf("failed to write wal: %v", err)
	}

	if commitToWal || atomiclimitExhausted > 0 {
		err = h.PriorityForEachShard(relabeler.WalCommit, func(shard relabeler.Shard) error {
			return shard.Wal().Commit()
		})
		if err != nil {
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

	err := h.PriorityForEachShard(relabeler.HeadInputRelabeling, func(shard relabeler.Shard) error {
		var (
			err              error
			hasReallocations bool
		)

		if state.TrackStaleness() {
			stats[shard.ShardID()], hasReallocations, err = rd.InputRelabelerByShard(
				shard.ShardID(),
			).InputRelabelingWithStalenans(
				ctx,
				shard.LSS().Input(),
				shard.LSS().Target(),
				state.CacheByShard(shard.ShardID()),
				state.RelabelerOptions(),
				state.StaleNansStateByShard(shard.ShardID()),
				state.DefTimestamp(),
				incomingData.Data().ShardedData(),
				shardedInnerSeries.DataBySourceShard(shard.ShardID()),
				shardedRelabeledSeries.DataByShard(shard.ShardID()),
			)
		} else {
			stats[shard.ShardID()], hasReallocations, err = rd.InputRelabelerByShard(
				shard.ShardID(),
			).InputRelabeling(
				ctx,
				shard.LSS().Input(),
				shard.LSS().Target(),
				state.CacheByShard(shard.ShardID()),
				state.RelabelerOptions(),
				incomingData.Data().ShardedData(),
				shardedInnerSeries.DataBySourceShard(shard.ShardID()),
				shardedRelabeledSeries.DataByShard(shard.ShardID()),
			)
		}

		incomingData.Destroy()
		if err != nil {
			return fmt.Errorf("shard %d: %w", shard.ShardID(), err)
		}

		if hasReallocations {
			shard.LSS().ResetSnapshot()
		}

		return nil
	})
	resStats := cppbridge.RelabelerStats{}

	if err != nil {
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
	err := h.PriorityForEachShard(relabeler.HeadAppendRelabelerSeries, func(shard relabeler.Shard) error {
		relabeledSeries, ok := shardedRelabeledSeries.DataBySourceShard(shard.ShardID())
		if !ok {
			return nil
		}

		hasReallocations, err := rd.InputRelabelerByShard(shard.ShardID()).AppendRelabelerSeries(
			ctx,
			shard.LSS().Target(),
			shardedInnerSeries.DataByShard(shard.ShardID()),
			relabeledSeries,
			shardedStateUpdates.DataByShard(shard.ShardID()),
		)
		if err != nil {
			return fmt.Errorf("shard %d: %w", shard.ShardID(), err)
		}

		if hasReallocations {
			shard.LSS().ResetSnapshot()
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
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

// shardLoop run relabeling on the shard.
//
//revive:disable-next-line:function-length long but readable.
//revive:disable-next-line:cognitive-complexity long but understandable.
//revive:disable-next-line:cyclomatic long but understandable.
func (h *Head) shardLoop(
	shardID uint16,
	priotity chan *GenericTask,
	nonPriority chan *GenericTask,
	stopc chan struct{},
) {
	sd := &shard{
		id:          shardID,
		lss:         h.lsses[shardID],
		dataStorage: h.dataStorages[shardID],
		wal:         h.wals[shardID],
	}
	forceNonPriority := 0

	for {
		select {
		case <-stopc:
			return

		case task := <-priotity:
			task.ExecuteOnShard(sd)

			if len(nonPriority) == 0 {
				continue
			}

			forceNonPriority++
			if forceNonPriority >= 10 {
				forceNonPriority = 0

				(<-nonPriority).ExecuteOnShard(sd)
			}

		default:
			select {
			case <-stopc:
				return

			case task := <-nonPriority:
				task.ExecuteOnShard(sd)

			case task := <-priotity:
				task.ExecuteOnShard(sd)

				if len(nonPriority) == 0 {
					continue
				}

				forceNonPriority++
				if forceNonPriority >= 10 {
					forceNonPriority = 0

					(<-nonPriority).ExecuteOnShard(sd)
				}
			}

		}
	}
}
