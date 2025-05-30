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

	dataStorages        []*DataStorage
	wals                []*ShardWal
	lsses               []*LSS
	shards              []*shard
	exclusiveTaskChs    []chan *GenericTask
	nonExclusiveTaskChs []chan *GenericTask
	sMutex              sync.RWMutex

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
	queueExclusive    *prometheus.GaugeVec
	queueNonExclusive *prometheus.GaugeVec

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
	exclusive := make([]chan *GenericTask, numberOfShards)
	nonExclusive := make([]chan *GenericTask, numberOfShards)
	shards := make([]*shard, numberOfShards)

	for shardID := uint16(0); shardID < numberOfShards; shardID++ {
		exclusive[shardID] = make(chan *GenericTask, chanBufferSize)
		nonExclusive[shardID] = make(chan *GenericTask, chanBufferSize)
		shards[shardID] = &shard{
			id:          shardID,
			lss:         lsses[shardID],
			dataStorage: dataStorages[shardID],
			wal:         wals[shardID],
		}
	}

	factory := util.NewUnconflictRegisterer(registerer)
	h := &Head{
		id:                  id,
		generation:          generation,
		lsses:               lsses,
		wals:                wals,
		dataStorages:        dataStorages,
		shards:              shards,
		exclusiveTaskChs:    exclusive,
		nonExclusiveTaskChs: nonExclusive,

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
		queueExclusive: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_head_queue_exclusive_size",
				Help: "The size of the queue exclusive of the current head.",
			},
			[]string{"shard_id"},
		),
		queueNonExclusive: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_head_queue_non_exclusive_size",
				Help: "The size of the queue non exclusive of the current head.",
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
	return h.ForEachShard(relabeler.WalCommit, func(shard relabeler.Shard) error {
		return shard.Wal().Commit()
	})
}

func (h *Head) Flush() error {
	return h.ForEachShard(relabeler.WalFlush, func(shard relabeler.Shard) error {
		return shard.Wal().Flush()
	})
}

func (h *Head) OnShard(shardID uint16, typeTask relabeler.TypeTask, fn relabeler.ShardFn) error {
	return h.onShard(shardID, typeTask, fn)
}

// MergeOutOfOrderChunks merge chunks with out of order data chunks.
func (h *Head) MergeOutOfOrderChunks() {
	_ = h.ForEachShard(relabeler.DataStorageMergeOutOfOrderChunks, func(shard relabeler.Shard) error {
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
	_ = h.ForEachShard(relabeler.HeadLSSAllocatedMemory, func(shard relabeler.Shard) error {
		h.memoryInUse.With(
			prometheus.Labels{
				"generation": generationStr,
				"allocator":  "main_lss",
				"id":         strconv.FormatUint(uint64(shard.ShardID()), 10),
			},
		).Set(float64(shard.LSS().AllocatedMemory()))

		return nil
	})

	_ = h.ForEachShard(relabeler.HeadDataStorageAllocatedMemory, func(shard relabeler.Shard) error {
		h.memoryInUse.With(
			prometheus.Labels{
				"generation": generationStr,
				"allocator":  "data_storage",
				"id":         strconv.FormatUint(uint64(shard.ShardID()), 10),
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

		h.queueExclusive.With(prometheus.Labels{
			"shard_id": shardIDStr,
		}).Set(float64(len(h.exclusiveTaskChs[shardID])))

		h.queueNonExclusive.With(prometheus.Labels{
			"shard_id": shardIDStr,
		}).Set(float64(len(h.nonExclusiveTaskChs[shardID])))
	}
}

func (h *Head) Status(limit int) relabeler.HeadStatus {
	shardStatuses := make([]*cppbridge.HeadStatus, h.NumberOfShards())
	for i := range shardStatuses {
		shardStatuses[i] = cppbridge.NewHeadStatus()
	}

	if limit != 0 {
		_ = h.ForEachShard(relabeler.DataStorageHeadStatus, func(shard relabeler.Shard) error {
			shardStatuses[shard.ShardID()].FromDataStorage(shard.DataStorage().Raw())

			return nil
		})
	}

	_ = h.ForEachShard(relabeler.LSSHeadStatus, func(shard relabeler.Shard) error {
		shardStatuses[shard.ShardID()].FromLSS(shard.LSS().Raw(), limit)

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
	_ = other.ForEachShard(relabeler.HeadCopyAddedSeries, func(shard relabeler.Shard) error {
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
	h.exclusiveTaskChs[shardID] <- task

	return task.Wait()
}

func (h *Head) stop() {
	close(h.stopc)
	h.wg.Wait()
	h.stopc = make(chan struct{})
}

func (h *Head) run() {
	readGoCount := 2
	for shardID := uint16(0); shardID < h.numberOfShards; shardID++ {
		h.wg.Add(readGoCount + 1)
		go func(sid uint16) {
			defer h.wg.Done()
			h.exclusiveShardLoop(sid, h.exclusiveTaskChs[sid], h.stopc)
		}(shardID)

		for i := 0; i < readGoCount; i++ {
			go func(sid uint16) {
				defer h.wg.Done()
				h.nonExclusiveShardLoop(sid, h.nonExclusiveTaskChs[sid], h.stopc)
			}(shardID)
		}
	}
}

// ForEachShard run func generic task on exclusive or non-exclusive queue by typeTask.
func (h *Head) ForEachShard(typeTask relabeler.TypeTask, fn relabeler.ShardFn) error {
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

	if typeTask.IsExclusive() {
		for _, exclusiveTaskCh := range h.exclusiveTaskChs {
			exclusiveTaskCh <- t
		}
	} else {
		for _, nonExclusiveTaskCh := range h.nonExclusiveTaskChs {
			nonExclusiveTaskCh <- t
		}
	}

	return t.Wait()
}

// readOnlyForEachShard run func generic task on read only head without queue.
func (h *Head) readOnlyForEachShard(t *GenericTask) error {
	for shardID := uint16(0); shardID < h.numberOfShards; shardID++ {
		s := h.shards[shardID]
		go func(sd *shard) {
			t.ExecuteOnShard(sd)
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

	_ = h.ForEachShard(relabeler.DataStorageAppendInnerSeries, func(shard relabeler.Shard) error {
		shard.DataStorage().AppendInnerSeriesSlice(shardedInnerSeries.DataByShard(shard.ShardID()))

		return nil
	})

	var atomiclimitExhausted uint32
	err = h.ForEachShard(relabeler.WalWrite, func(shard relabeler.Shard) error {
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
		err = h.ForEachShard(relabeler.WalCommit, func(shard relabeler.Shard) error {
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

	err := h.ForEachShard(relabeler.HeadInputRelabeling, func(shard relabeler.Shard) error {
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
	err := h.ForEachShard(relabeler.HeadAppendRelabelerSeries, func(shard relabeler.Shard) error {
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

	// return h.ForEachShard(relabeler.HeadUpdateRelabelerState, func(shard relabeler.Shard) error {
	// 	updates, ok := shardedStateUpdates.DataBySourceShard(shard.ShardID())
	// 	if !ok {
	// 		return nil
	// 	}

	// 	err := rd.InputRelabelerByShard(shard.ShardID()).UpdateRelabelerState(
	// 		ctx,
	// 		state.CacheByShard(shard.ShardID()),
	// 		updates,
	// 	)
	// 	if err != nil {
	// 		return fmt.Errorf("shard %d: %w", shard.ShardID(), err)
	// 	}

	// 	return nil
	// })
}

// exclusiveShardLoop run shard loop for exclusive(write lock) operation.
func (h *Head) exclusiveShardLoop(
	shardID uint16,
	exclusiveCH chan *GenericTask,
	stopc chan struct{},
) {
	s := h.shards[shardID]

	for {
		select {
		case <-stopc:
			return

		case task := <-exclusiveCH:
			h.sMutex.Lock()
			task.ExecuteOnShard(s)
			h.sMutex.Unlock()
		}
	}
}

// nonExclusiveShardLoop run shard loop for non-exclusive(read lock) operation.
func (h *Head) nonExclusiveShardLoop(
	shardID uint16,
	nonExclusiveCH chan *GenericTask,
	stopc chan struct{},
) {
	s := h.shards[shardID]

	for {
		select {
		case <-stopc:
			return

		case task := <-nonExclusiveCH:
			h.sMutex.RLock()
			task.ExecuteOnShard(s)
			h.sMutex.RUnlock()
		}
	}
}
