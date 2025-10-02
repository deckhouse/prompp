// Shard - TODO write description on package

package shard

import (
	"errors"
	"fmt"
	"time"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

// Wal the minimum required Wal implementation for a [Shard].
type Wal interface {
	// Close closes the wal segmentWriter.
	Close() error

	// Commit finalize segment from encoder and write to wal.
	Commit() error

	// CurrentSize returns current wal size.
	CurrentSize() int64

	// Flush flush all contetnt into wal.
	Flush() error

	// Sync commits the current contents of the [Wal].
	Sync() error

	// Write append the incoming inner series to wal encoder.
	Write(innerSeriesSlice []*cppbridge.InnerSeries) (bool, error)
}

//
// Shard
//

// Shard bridge to labelset storage, data storage and wal.
type Shard struct {
	lss                  *LSS
	dataStorage          *DataStorage
	unloadedDataStorage  *UnloadedDataStorage
	queriedSeriesStorage *QueriedSeriesStorage
	loadAndQueryTask     *LoadAndQuerySeriesDataTask
	wal                  Wal
	id                   uint16
}

// NewShard init new [Shard].
func NewShard(
	lss *LSS,
	dataStorage *DataStorage,
	unloadedDataStorage *UnloadedDataStorage,
	queriedSeriesStorage *QueriedSeriesStorage,
	wal Wal,
	shardID uint16,
) *Shard {
	return &Shard{
		id:                   shardID,
		lss:                  lss,
		dataStorage:          dataStorage,
		unloadedDataStorage:  unloadedDataStorage,
		queriedSeriesStorage: queriedSeriesStorage,
		loadAndQueryTask:     &LoadAndQuerySeriesDataTask{},
		wal:                  wal,
	}
}

// AppendInnerSeriesSlice add InnerSeries to [DataStorage].
func (s *Shard) AppendInnerSeriesSlice(innerSeriesSlice []*cppbridge.InnerSeries) {
	s.dataStorage.AppendInnerSeriesSlice(innerSeriesSlice)
}

// Close closes the wal segmentWriter.
func (s *Shard) Close() error {
	err := s.wal.Close()

	if s.unloadedDataStorage != nil {
		err = errors.Join(err, s.unloadedDataStorage.Close())
	}

	if s.queriedSeriesStorage != nil {
		err = errors.Join(err, s.queriedSeriesStorage.Close())
	}

	return err
}

// DSAllocatedMemory return size of allocated memory for [DataStorage].
func (s *Shard) DSAllocatedMemory() uint64 {
	return s.dataStorage.AllocatedMemory()
}

// DataStorage returns shard [DataStorage].
func (s *Shard) DataStorage() *DataStorage {
	return s.dataStorage
}

// LSS returns shard labelset storage [LSS].
func (s *Shard) LSS() *LSS {
	return s.lss
}

// LSSAllocatedMemory return size of allocated memory for labelset storages.
func (s *Shard) LSSAllocatedMemory() uint64 {
	return s.lss.AllocatedMemory()
}

// LSSWithLock calls fn on raws [cppbridge.LabelSetStorage] with write lock.
func (s *Shard) LSSWithLock(fn func(target, input *cppbridge.LabelSetStorage) error) error {
	return s.lss.WithLock(fn)
}

// LSSWithRLock calls fn on raws [cppbridge.LabelSetStorage] with read lock.
func (s *Shard) LSSWithRLock(fn func(target, input *cppbridge.LabelSetStorage) error) error {
	return s.lss.WithRLock(fn)
}

// LSSResetSnapshot resets the current snapshot. Use only WithLock.
func (s *Shard) LSSResetSnapshot() {
	s.lss.ResetSnapshot()
}

// MergeOutOfOrderChunks merge chunks with out of order data chunks in [DataStorage].
func (s *Shard) MergeOutOfOrderChunks() {
	s.dataStorage.MergeOutOfOrderChunks()
}

// ShardID returns the shard ID.
func (s *Shard) ShardID() uint16 {
	return s.id
}

// WalCommit finalize segment from encoder and write to wal.
func (s *Shard) WalCommit() error {
	return s.lss.WithRLock(func(_, _ *cppbridge.LabelSetStorage) error {
		return s.wal.Commit()
	})
}

// WalCurrentSize returns current [Wal] size.
func (s *Shard) WalCurrentSize() int64 {
	return s.wal.CurrentSize()
}

// WalFlush flush all contetnt into wal.
func (s *Shard) WalFlush() error {
	return s.wal.Flush()
}

// WalSync commits the current contents of the [Wal].
func (s *Shard) WalSync() error {
	return s.wal.Sync()
}

// WalWrite append the incoming inner series to wal encoder.
func (s *Shard) WalWrite(innerSeriesSlice []*cppbridge.InnerSeries) (bool, error) {
	return s.wal.Write(innerSeriesSlice)
}

// TimeInterval get time interval from [DataStorage].
func (s *Shard) TimeInterval(invalidateCache bool) cppbridge.TimeInterval {
	return s.dataStorage.TimeInterval(invalidateCache)
}

// UnloadedDataStorage get unloaded data storage
func (s *Shard) UnloadedDataStorage() *UnloadedDataStorage {
	return s.unloadedDataStorage
}

// QueriedSeriesStorage get queried series storage
func (s *Shard) QueriedSeriesStorage() *QueriedSeriesStorage {
	return s.queriedSeriesStorage
}

// LoadAndQuerySeriesDataTask get load and query series data task
func (s *Shard) LoadAndQuerySeriesDataTask() *LoadAndQuerySeriesDataTask {
	return s.loadAndQueryTask
}

// UnloadUnusedSeriesData unload unused series data
func (s *Shard) UnloadUnusedSeriesData() error {
	if s.UnloadedDataStorage() == nil {
		return nil
	}

	unloader := s.DataStorage().CreateUnusedSeriesDataUnloader()

	var snapshot, queriedSeries []byte
	_ = s.DataStorage().WithRLock(func(*cppbridge.HeadDataStorage) error {
		snapshot = unloader.CreateSnapshot()
		queriedSeries = s.DataStorage().GetQueriedSeriesBitset()
		return nil
	})

	header, err := s.UnloadedDataStorage().WriteSnapshot(snapshot)
	if err != nil {
		return fmt.Errorf("unable to write unloaded series data snapshot: %v", err)
	}

	_ = s.DataStorage().WithLock(func(*cppbridge.HeadDataStorage) error {
		s.UnloadedDataStorage().WriteIndex(header)
		unloader.Unload()
		return nil
	})

	if err = s.QueriedSeriesStorage().Write(queriedSeries, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("unable to write queried series data: %v", err)
	}

	return nil
}

// LoadAndQuerySeriesData loads the data and queries the series from the [DataStorage].
func (s *Shard) LoadAndQuerySeriesData() (err error) {
	var queriers []uintptr
	s.loadAndQueryTask.Release(func(q []uintptr) {
		queriers = q
		err = s.DataStorage().WithLock(func(*cppbridge.HeadDataStorage) error {
			loader := s.DataStorage().CreateLoader(queriers)
			return s.UnloadedDataStorage().ForEachSnapshot(loader.Load)
		})
	})
	if err != nil {
		return err
	}

	s.DataStorage().QueryFinal(queriers)

	return nil
}

//
// PerGoroutineShard
//

// PerGoroutineShard wrapper of shard with [PerGoroutineRelabeler] for goroutines.
type PerGoroutineShard struct {
	relabeler *cppbridge.PerGoroutineRelabeler
	*Shard
}

// NewPerGoroutineShard init new [PerGoroutineShard].
func NewPerGoroutineShard[TWal Wal](s *Shard, numberOfShards uint16) *PerGoroutineShard {
	return &PerGoroutineShard{
		relabeler: cppbridge.NewPerGoroutineRelabeler(numberOfShards, s.ShardID()),
		Shard:     s,
	}
}

// Relabeler returns relabeler for shard goroutines.
func (s *PerGoroutineShard) Relabeler() *cppbridge.PerGoroutineRelabeler {
	return s.relabeler
}
