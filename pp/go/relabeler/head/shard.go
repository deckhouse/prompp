package head

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
)

type LSS struct {
	input  *cppbridge.LabelSetStorage
	target *cppbridge.LSSWithSnapshot
}

func (w *LSS) Raw() *cppbridge.LabelSetStorage {
	return w.target.LSS()
}

func (w *LSS) AllocatedMemory() uint64 {
	return w.input.AllocatedMemory() + w.target.LSS().AllocatedMemory()
}

func (w *LSS) QueryLabelValues(
	label_name string,
	matchers []model.LabelMatcher,
) *cppbridge.LSSQueryLabelValuesResult {
	return w.target.LSS().QueryLabelValues(label_name, matchers)
}

func (w *LSS) QueryLabelNames(matchers []model.LabelMatcher) *cppbridge.LSSQueryLabelNamesResult {
	return w.target.LSS().QueryLabelNames(matchers)
}

func (w *LSS) Query(matchers []model.LabelMatcher, querySource uint32) *cppbridge.LSSQueryResult {
	return w.target.LSS().Query(matchers, querySource)
}

func (w *LSS) GetLabelSets(labelSetIDs []uint32) *cppbridge.LabelSetStorageGetLabelSetsResult {
	return w.target.LSS().GetLabelSets(labelSetIDs)
}

// GetSnapshot return the actual snapshot.
func (w *LSS) GetSnapshot() *cppbridge.LabelSetSnapshot {
	return w.target.Snapshot()
}

// Outdate marked *LabelSetStorage is outdated.
func (w *LSS) Outdate() {
	w.target.Outdate()
}

// ResetSnapshot resets the current snapshot.
func (w *LSS) ResetSnapshot() {
	w.target.ResetSnapshot()
}

// Input return input LabelSetStorage.
func (w *LSS) Input() *cppbridge.LabelSetStorage {
	return w.input
}

// Target return target LabelSetStorage.
func (w *LSS) Target() *cppbridge.LabelSetStorage {
	return w.target.LSS()
}

// FindFromBuilder label set from builder in lss, return length ls, lsid and bool ok.
//
//nolint:gocritic // unnamedResult not need
func (w *LSS) FindFromBuilder(
	sortedAdd []cppbridge.Label,
	sortedDel []string,
	snapshot *cppbridge.LabelSetSnapshot,
	lsID uint32,
) (uint32, uint16, bool) {
	return w.target.LSS().FindFromBuilder(sortedAdd, sortedDel, snapshot, lsID)
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

//
// shard
//

type shard struct {
	lss               *LSS
	dataStorage       *DataStorage
	wal               *ShardWal
	lssLocker         RWLockable
	dataStorageLocker RWLockable
	id                uint16
}

// newShard init new *shard.
func newShard(
	lss *LSS,
	dataStorage *DataStorage,
	wal *ShardWal,
	shardID uint16,
	withLocker bool,
) *shard {
	s := &shard{
		id:                shardID,
		lss:               lss,
		dataStorage:       dataStorage,
		wal:               wal,
		lssLocker:         &noopRWLockable{},
		dataStorageLocker: &noopRWLockable{},
	}

	if withLocker {
		s.lssLocker = &sync.RWMutex{}
		s.dataStorageLocker = &sync.RWMutex{}
	}

	return s
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

// DataStorageLock lock data storage for write operation.
func (s *shard) DataStorageLock() {
	s.dataStorageLocker.Lock()
}

// DataStorageRLock lock data storage for read operation.
func (s *shard) DataStorageRLock() {
	s.dataStorageLocker.RLock()
}

// DataStorageRUnlock unlock data storage for read operation.
func (s *shard) DataStorageRUnlock() {
	s.dataStorageLocker.RUnlock()
}

// DataStorageUnlock unlock data storage for write operation.
func (s *shard) DataStorageUnlock() {
	s.dataStorageLocker.Unlock()
}

// LSSLock lock lss for write operation.
func (s *shard) LSSLock() {
	s.lssLocker.Lock()
}

// LSSRLock lock lss for read operation.
func (s *shard) LSSRLock() {
	s.lssLocker.RLock()
}

// LSSUnlock unlock lss for read operation.
func (s *shard) LSSRUnlock() {
	s.lssLocker.RUnlock()
}

// LSSUnlock unlock lss for write operation.
func (s *shard) LSSUnlock() {
	s.lssLocker.Unlock()
}

//
// RWLockable
//

// RWLockable implementation [sync.RWMutex].
type RWLockable interface {
	Lock()
	RLock()
	RUnlock()
	Unlock()
}

//
// noopRWLockable
//

// noopRWLockable implementation sync.RWMutex, does nothing.
type noopRWLockable struct{}

// Lock implementation [RWLockable].
func (*noopRWLockable) Lock() {}

// RLock implementation [RWLockable].
func (*noopRWLockable) RLock() {}

// RUnlock implementation [RWLockable].
func (*noopRWLockable) RUnlock() {}

// Unlock implementation [RWLockable].
func (*noopRWLockable) Unlock() {}
