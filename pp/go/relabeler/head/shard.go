package head

import (
	"fmt"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
)

// chanBufferSize size of channels buffer.
const chanBufferSize = 64

type LSS struct {
	input    *cppbridge.LabelSetStorage
	target   *cppbridge.LabelSetStorage
	snapshot *cppbridge.LabelSetSnapshot
	once     sync.Once
}

func (w *LSS) Raw() *cppbridge.LabelSetStorage {
	return w.target
}

func (w *LSS) AllocatedMemory() uint64 {
	return w.input.AllocatedMemory() + w.target.AllocatedMemory()
}

func (w *LSS) QueryLabelValues(
	label_name string,
	matchers []model.LabelMatcher,
) *cppbridge.LSSQueryLabelValuesResult {
	return w.target.QueryLabelValues(label_name, matchers)
}

func (w *LSS) QueryLabelNames(matchers []model.LabelMatcher) *cppbridge.LSSQueryLabelNamesResult {
	return w.target.QueryLabelNames(matchers)
}

func (w *LSS) Query(matchers []model.LabelMatcher, querySource uint32) *cppbridge.LSSQueryResult {
	return w.target.Query(matchers, querySource)
}

func (w *LSS) GetLabelSets(labelSetIDs []uint32) *cppbridge.LabelSetStorageGetLabelSetsResult {
	return w.target.GetLabelSets(labelSetIDs)
}

// GetSnapshot return the actual snapshot.
func (w *LSS) GetSnapshot() *cppbridge.LabelSetSnapshot {
	w.once.Do(func() {
		if w.snapshot == nil {
			w.snapshot = w.target.CreateLabelSetSnapshot()
		}
	})

	return w.snapshot
}

// ResetSnapshot resets the current snapshot.
func (w *LSS) ResetSnapshot() {
	w.snapshot = nil
	w.once = sync.Once{}
}

func (w *LSS) Input() *cppbridge.LabelSetStorage {
	return w.input
}

func (w *LSS) Target() *cppbridge.LabelSetStorage {
	return w.target
}

type DataStorage struct {
	dataStorage *cppbridge.HeadDataStorage
	encoder     *cppbridge.HeadEncoder
}

func NewDataStorage() *DataStorage {
	dataStorage := cppbridge.NewHeadDataStorage()
	return &DataStorage{
		dataStorage: dataStorage,
		encoder:     cppbridge.NewHeadEncoderWithDataStorage(dataStorage),
	}
}

func (ds *DataStorage) AppendInnerSeriesSlice(innerSeriesSlice []*cppbridge.InnerSeries) {
	ds.encoder.EncodeInnerSeriesSlice(innerSeriesSlice)
}

func (ds *DataStorage) Raw() *cppbridge.HeadDataStorage {
	return ds.dataStorage
}

func (ds *DataStorage) MergeOutOfOrderChunks() {
	ds.encoder.MergeOutOfOrderChunks()
}

func (ds *DataStorage) Query(query cppbridge.HeadDataStorageQuery) *cppbridge.HeadDataStorageSerializedChunks {
	return ds.dataStorage.Query(query)
}

func (ds *DataStorage) InstantQuery(targetTimestamp, notFoundValueTimestampValue int64, seriesIDs []uint32) []cppbridge.Sample {
	return ds.dataStorage.InstantQuery(targetTimestamp, notFoundValueTimestampValue, seriesIDs)
}

func (ds *DataStorage) AllocatedMemory() uint64 {
	return ds.dataStorage.AllocatedMemory()
}

// reshards changes the number of shards to the required amount.
func (h *Head) reconfigure(
	inputRelabelerConfigs []*config.InputRelabelerConfig,
	numberOfShards uint16,
) error {
	return h.reconfigureRelabelersData(inputRelabelerConfigs, numberOfShards)
}

// reconfigureRelabelersData reconfiguring relabelers data for all shards.
func (h *Head) reconfigureRelabelersData(
	inputRelabelerConfigs []*config.InputRelabelerConfig,
	numberOfShards uint16,
) error {
	updated := make(map[string]struct{})
	for _, cfgs := range inputRelabelerConfigs {
		relabelerID := cfgs.GetName()
		if rd, ok := h.relabelersData[relabelerID]; ok {
			if err := rd.Reconfigure(cfgs.GetConfigs(), h.generation, numberOfShards); err != nil {
				return err
			}
			updated[relabelerID] = struct{}{}
			continue
		}

		rd, err := NewRelabelerData(
			cfgs.GetConfigs(),
			h.generation,
			numberOfShards,
		)
		if err != nil {
			return err
		}
		h.relabelersData[relabelerID] = rd
		updated[relabelerID] = struct{}{}
	}

	for relabelerID := range h.relabelersData {
		if _, ok := updated[relabelerID]; !ok {
			// clear unnecessary
			h.memoryInUse.DeletePartialMatch(prometheus.Labels{
				"allocator": fmt.Sprintf("input_relabeler_%s", relabelerID),
			})
			delete(h.relabelersData, relabelerID)
		}
	}

	return nil
}

// shardLoop run relabeling on the shard.
//
//revive:disable-next-line:function-length long but readable.
//revive:disable-next-line:cognitive-complexity long but understandable.
//revive:disable-next-line:cyclomatic long but understandable.
func (h *Head) shardLoop(shardID uint16, stopc chan struct{}) {
	var (
		readWG = sync.WaitGroup{}
		sd     = &shard{
			id:          shardID,
			lss:         h.lsses[shardID],
			dataStorage: h.dataStorages[shardID],
			wal:         h.wals[shardID],
		}
	)

	for {
		select {
		case <-stopc:
			return
		case task := <-h.stageInputRelabeling[shardID]:
			task.Run(h.lsses[shardID], h.stageAppendRelabelerSeries, shardID, h.numberOfShards)

		case task := <-h.stageAppendRelabelerSeries[shardID]:
			task.Run(h.lsses[shardID], h.stageUpdateRelabelers, shardID)

		case task := <-h.stageUpdateRelabelers[shardID]:
			if err := task.Update(); err != nil {
				task.AddError(shardID, fmt.Errorf("failed input update relabeler state %d: %w", shardID, err))
				continue
			}

		case task := <-h.genericTaskCh[shardID]:
			task.ExecuteOnShard(sd)

		case task := <-h.genericReadTaskCh[shardID]:
			length := len(h.genericReadTaskCh[shardID])
			if length == 0 {
				task.ExecuteOnShard(sd)
				continue
			}

			readWG.Add(length + 1)
			go func(t *GenericReadTask, s *shard) {
				t.ExecuteOnShard(s)
				readWG.Done()
			}(task, sd)

			for i := 0; i < length; i++ {
				task = <-h.genericReadTaskCh[shardID]

				go func(t *GenericReadTask, s *shard) {
					t.ExecuteOnShard(s)
					readWG.Done()
				}(task, sd)
			}

			readWG.Wait()
		}
	}
}

type shard struct {
	id          uint16
	lss         *LSS
	dataStorage *DataStorage
	wal         *ShardWal
}

func (s *shard) ShardID() uint16 {
	return s.id
}

func (s *shard) DataStorage() relabeler.DataStorage {
	return s.dataStorage
}

func (s *shard) LSS() relabeler.LSS {
	return s.lss
}

func (s *shard) Wal() relabeler.Wal {
	return s.wal
}

func (h *Head) shardLoop2(
	shardID uint16,
	priotity chan *GenericTrueTask,
	nonPriority chan *GenericTrueTask,
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
