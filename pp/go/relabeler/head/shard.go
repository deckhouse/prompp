package head

import (
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
)

// chanBufferSize size of channels buffer.
const chanBufferSize = 64

type LSS struct {
	input  *cppbridge.LabelSetStorage
	target *cppbridge.LabelSetStorage
	// snapshot *cppbridge.LabelSetSnapshot
	snapshot unsafe.Pointer
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
		// w.snapshot =  w.target.CreateLabelSetSnapshot()
		atomic.StorePointer(
			&w.snapshot,
			unsafe.Pointer(w.target.CreateLabelSetSnapshot(w)), // #nosec G103 // it's meant to be that way
		)
	})

	// return w.snapshot
	return w.FastSnapshot()
}

// FastSnapshot return the actual snapshot or nil if not exist.
func (w *LSS) FastSnapshot() *cppbridge.LabelSetSnapshot {
	return (*cppbridge.LabelSetSnapshot)(atomic.LoadPointer(&w.snapshot))
}

// ResetSnapshot resets the current snapshot.
func (w *LSS) ResetSnapshot() {
	w.snapshot = nil
	w.once = sync.Once{}
}

// Input return input LabelSetStorage.
func (w *LSS) Input() *cppbridge.LabelSetStorage {
	return w.input
}

// Target return target LabelSetStorage.
func (w *LSS) Target() *cppbridge.LabelSetStorage {
	return w.target
}

// FindFromBuilder label set from builder in lss, return length ls, lsid and bool ok.
//
//nolint:gocritic // unnamedResult not need
func (w *LSS) FindFromBuilder(
	sortedAdd []cppbridge.Label,
	sortedDel []string,
	snapshot *cppbridge.LabelSetSnapshot,
	lsID uint32,
) (uint64, uint32, bool) {
	return w.target.FindFromBuilder(sortedAdd, sortedDel, snapshot, lsID)
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
