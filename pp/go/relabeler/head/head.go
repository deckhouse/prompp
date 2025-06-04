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

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/model"
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
		rd.state = cppbridge.NewState(uint16(len(rd.inputRelabelers)))
		rd.state.Reconfigure(
			rd.generationRelabeler(),
			generationHead,
			uint16(len(rd.inputRelabelers)),
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
			uint16(shardID),
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

	dataStorages               []*DataStorage
	wals                       []*ShardWal
	lsses                      []*LSS
	stageInputRelabeling       []chan *TaskInputRelabeling
	stageAppendRelabelerSeries []chan *TaskAppendRelabelerSeries
	stageUpdateRelabelers      []chan *TaskUpdateRelabelerState
	genericTaskCh              []chan *GenericTask
	genericReadTaskCh          []chan *GenericReadTask
	numberOfShards             uint16
	// stat
	appendedSegmentCount prometheus.Counter
	memoryInUse          *prometheus.GaugeVec
	series               prometheus.Gauge
	queried              *prometheus.GaugeVec
	walSize              *prometheus.GaugeVec
	queueInputRelabeling *prometheus.GaugeVec
	queueRead            *prometheus.GaugeVec
	queueGeneric         *prometheus.GaugeVec
	stopc                chan struct{}
	wg                   *sync.WaitGroup
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
	stageInputRelabeling := make([]chan *TaskInputRelabeling, numberOfShards)
	stageAppendRelabelerSeries := make([]chan *TaskAppendRelabelerSeries, numberOfShards)
	stageupdateRelabelers := make([]chan *TaskUpdateRelabelerState, numberOfShards)
	genericTaskCh := make([]chan *GenericTask, numberOfShards)
	genericReadTaskCh := make([]chan *GenericReadTask, numberOfShards)

	var shardID uint16
	for ; shardID < numberOfShards; shardID++ {
		stageInputRelabeling[shardID] = make(chan *TaskInputRelabeling, chanBufferSize)
		stageAppendRelabelerSeries[shardID] = make(chan *TaskAppendRelabelerSeries, chanBufferSize)
		stageupdateRelabelers[shardID] = make(chan *TaskUpdateRelabelerState, chanBufferSize)
		genericTaskCh[shardID] = make(chan *GenericTask, chanBufferSize)
		genericReadTaskCh[shardID] = make(chan *GenericReadTask, chanBufferSize)
	}

	factory := util.NewUnconflictRegisterer(registerer)
	h := &Head{
		id:                         id,
		generation:                 generation,
		lsses:                      lsses,
		wals:                       wals,
		dataStorages:               dataStorages,
		stageInputRelabeling:       stageInputRelabeling,
		stageAppendRelabelerSeries: stageAppendRelabelerSeries,
		stageUpdateRelabelers:      stageupdateRelabelers,
		genericTaskCh:              genericTaskCh,
		genericReadTaskCh:          genericReadTaskCh,
		stopc:                      make(chan struct{}),
		wg:                         &sync.WaitGroup{},
		relabelersData:             make(map[string]*RelabelerData, len(inputRelabelerConfigs)),
		numberOfShards:             numberOfShards,
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
		queueInputRelabeling: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_head_queue_input_relabeling_size",
				Help: "The size of the queue input relabeling of the current head.",
			},
			[]string{"shard_id"},
		),
		queueRead: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_head_queue_read_size",
				Help: "The size of the queue read of the current head.",
			},
			[]string{"shard_id"},
		),
		queueGeneric: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_head_queue_generic_size",
				Help: "The size of the queue generic of the current head.",
			},
			[]string{"shard_id"},
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

// Append incoming data to head.
func (h *Head) Append(
	ctx context.Context,
	incomingData *relabeler.IncomingData,
	state *cppbridge.State,
	relabelerID string,
	commitToWal bool,
) ([][]*cppbridge.InnerSeries, cppbridge.RelabelerStats, error) {
	if h.readOnly {
		return nil, cppbridge.RelabelerStats{}, fmt.Errorf("appending to read only head")
	}

	h.rdMutex.Lock()
	rd, ok := h.relabelersData[relabelerID]
	if !ok {
		h.rdMutex.Unlock()
		return nil, cppbridge.RelabelerStats{}, fmt.Errorf("relabeler ID not exist: %s", relabelerID)
	}

	if state != nil {
		state.Reconfigure(rd.generationRelabeler(), h.generation, h.numberOfShards)
	}

	if state == nil {
		state = rd.State(h.generation)
	}
	h.rdMutex.Unlock()

	inputPromise := NewInputRelabelingPromise(h.numberOfShards)
	h.enqueueInputRelabeling(NewTaskInputRelabeling(
		ctx,
		inputPromise,
		relabeler.NewDestructibleIncomingData(incomingData, int(h.numberOfShards)),
		rd,
		state,
	))
	if err := inputPromise.Wait(ctx); err != nil {
		// reset msr.rotateWG on error
		return nil, cppbridge.RelabelerStats{}, fmt.Errorf("failed input promise: %s", err)
	}

	var atomiclimitExhausted uint32
	err := h.forEachShard(func(shard relabeler.Shard) error {
		limitExhausted, err := shard.Wal().Write(inputPromise.ShardsInnerSeries(shard.ShardID()))
		if err != nil {
			return fmt.Errorf("failed to write inner series: %w", err)
		}

		if limitExhausted {
			atomic.AddUint32(&atomiclimitExhausted, 1)
		}

		return nil
	})
	if err != nil {
		logger.Errorf("failed to write wal: %v", err)
	}

	err = h.forEachShard(func(shard relabeler.Shard) error {
		shard.DataStorage().AppendInnerSeriesSlice(inputPromise.ShardsInnerSeries(shard.ShardID()))

		if commitToWal || atomiclimitExhausted > 0 {
			return shard.Wal().Commit()
		}

		return nil
	})
	if err != nil {
		logger.Errorf("failed to commit wal: %v", err)
	}

	return inputPromise.data, inputPromise.Stats(), nil
}

func (h *Head) CommitToWal() error {
	if h.readOnly {
		return fmt.Errorf("commiting read only head")
	}
	return h.ForEachShard(func(shard relabeler.Shard) error {
		return shard.Wal().Commit()
	})
}

func (h *Head) Flush() error {
	return h.ForEachShard(func(shard relabeler.Shard) error {
		return shard.Wal().Flush()
	})
}

func (h *Head) ForEachShard(fn relabeler.ShardFn) error {
	return h.forEachShard(fn)
}

func (h *Head) OnShard(shardID uint16, fn relabeler.ShardFn) error {
	return h.onShard(shardID, fn)
}

// MergeOutOfOrderChunks merge chunks with out of order data chunks.
func (h *Head) MergeOutOfOrderChunks() {
	_ = h.forEachShard(func(shard relabeler.Shard) error {
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
	for relabelerID := range h.relabelersData {
		// clear unnecessary
		h.memoryInUse.DeletePartialMatch(prometheus.Labels{
			"generation": fmt.Sprintf("%d", h.generation),
			"allocator":  fmt.Sprintf("input_relabeler_%s", relabelerID),
		})
	}
	h.relabelersData = nil

	// h.counter.DeletePartialMatch(prometheus.Labels{"id": h.id})
	// h.size.DeletePartialMatch(prometheus.Labels{"id": h.id})
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
	status := h.Status(1)
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

	_ = h.ReadEachShard(func(shard relabeler.Shard) error {
		h.memoryInUse.With(
			prometheus.Labels{
				"generation": fmt.Sprintf("%d", h.generation),
				"allocator":  "main_lss",
				"id":         fmt.Sprintf("%d", shard.ShardID()),
			},
		).Set(float64(shard.LSS().AllocatedMemory()))

		h.memoryInUse.With(
			prometheus.Labels{
				"generation": fmt.Sprintf("%d", h.generation),
				"allocator":  "data_storage",
				"id":         fmt.Sprintf("%d", shard.ShardID()),
			},
		).Set(float64(shard.DataStorage().AllocatedMemory()))

		return nil
	})

	if h.readOnly {
		return
	}

	// do not write metrics if the head is read-only.
	for shardID := uint16(0); shardID < h.numberOfShards; shardID++ {
		h.walSize.With(
			prometheus.Labels{"shard_id": strconv.FormatUint(uint64(shardID), 10)},
		).Set(float64(h.wals[shardID].CurrentSize()))

		h.queueInputRelabeling.With(prometheus.Labels{
			"shard_id": strconv.FormatUint(uint64(shardID), 10),
		}).Set(float64(len(h.stageInputRelabeling[shardID])))

		h.queueRead.With(prometheus.Labels{
			"shard_id": strconv.FormatUint(uint64(shardID), 10),
		}).Set(float64(len(h.genericReadTaskCh[shardID])))

		h.queueGeneric.With(prometheus.Labels{
			"shard_id": strconv.FormatUint(uint64(shardID), 10),
		}).Set(float64(len(h.genericTaskCh[shardID])))
	}
}

func (h *Head) Status(limit int) relabeler.HeadStatus {
	shardStatuses := make([]*cppbridge.HeadStatus, h.NumberOfShards())
	_ = h.ReadEachShard(func(shard relabeler.Shard) error {
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
		if headStatus.HeadStats.MaxTime < shardStatus.TimeInterval.Max {
			headStatus.HeadStats.MaxTime = shardStatus.TimeInterval.Max
		}
		if headStatus.HeadStats.MinTime > shardStatus.TimeInterval.Min {
			headStatus.HeadStats.MinTime = shardStatus.TimeInterval.Min
		}

		headStatus.HeadStats.NumSeries += uint64(shardStatus.NumSeries)
		headStatus.HeadStats.ChunkCount += int64(shardStatus.ChunkCount)
		headStatus.HeadStats.NumLabelPairs += int(shardStatus.NumLabelPairs)
		headStatus.HeadStats.RuleQueriedSeries += int64(shardStatus.RuleQueriedSeries)
		headStatus.HeadStats.FederateQueriedSeries += int64(shardStatus.FederateQueriedSeries)
		headStatus.HeadStats.OtherQueriedSeries += int64(shardStatus.OtherQueriedSeries)

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

func (h *Head) Rotate() error {
	return nil
}

// CopySeriesFrom copy series from other head.
func (h *Head) CopySeriesFrom(other relabeler.Head) {
	_ = other.ForEachShard(func(shard relabeler.Shard) error {
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

// enqueueInputRelabeling send task to shard for input relabeling.
func (h *Head) enqueueInputRelabeling(task *TaskInputRelabeling) {
	for _, s := range h.stageInputRelabeling {
		s <- task
	}
}

func (h *Head) Discard() error {
	return nil
}

// enqueueHeadAppending append all series after input relabeling stage to head.
func (h *Head) forEachShard(fn relabeler.ShardFn) error {
	task := NewGenericTask(fn, h.numberOfShards)
	if h.readOnly {
		for shardID := uint16(0); shardID < h.numberOfShards; shardID++ {
			s := &shard{
				id:          shardID,
				lss:         h.lsses[shardID],
				dataStorage: h.dataStorages[shardID],
				wal:         h.wals[shardID],
			}
			go func(shard *shard) {
				task.ExecuteOnShard(shard)
			}(s)
		}
		task.Wait()
		return errors.Join(task.Errors()...)
	}

	for _, shardGenericTaskCh := range h.genericTaskCh {
		shardGenericTaskCh <- task
	}
	task.Wait()
	return errors.Join(task.Errors()...)
}

func (h *Head) onShard(shardID uint16, fn relabeler.ShardFn) error {
	if h.readOnly {
		s := &shard{
			id:          shardID,
			lss:         h.lsses[shardID],
			dataStorage: h.dataStorages[shardID],
		}

		return fn(s)
	}

	task := NewSingleGenericTask(fn, h.numberOfShards)
	h.genericTaskCh[shardID] <- task
	task.Wait()
	return errors.Join(task.Errors()...)
}

func (h *Head) run() {
	var shardID uint16
	for ; shardID < h.numberOfShards; shardID++ {
		h.wg.Add(1)
		go func(shardID uint16) {
			defer h.wg.Done()
			h.shardLoop(shardID, h.stopc)
		}(shardID)
	}
}

func (h *Head) stop() {
	close(h.stopc)
	h.wg.Wait()
	h.stopc = make(chan struct{})
}

// ReadEachShard execute read fn on each shard.
func (h *Head) ReadEachShard(fn relabeler.ShardFn) error {
	task := NewGenericReadTask(fn, h.numberOfShards)
	if h.readOnly {
		for shardID := uint16(0); shardID < h.numberOfShards; shardID++ {
			s := &shard{
				id:          shardID,
				lss:         h.lsses[shardID],
				dataStorage: h.dataStorages[shardID],
				wal:         h.wals[shardID],
			}
			go func(shard *shard) {
				task.ExecuteOnShard(shard)
			}(s)
		}
		task.Wait()
		return errors.Join(task.Errors()...)
	}

	for _, shardGenericReadTaskCh := range h.genericReadTaskCh {
		shardGenericReadTaskCh <- task
	}
	task.Wait()
	return errors.Join(task.Errors()...)
}

// Find label set in lss, if not found return EmptyLabels.
func (h *Head) Find(mls model.LabelSet) labels.Labels {
	for i := range h.lsses {
		if lssRO, lsID, ok := h.lsses[i].Find(mls); ok {
			return labels.NewLabelsWithLSS(lssRO, lsID, uint16(mls.Len()))
		}
	}

	return labels.EmptyLabels()
}
