// Shard - TODO write description on package

package shard

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
)

// Wal the minimum required Wal implementation for a [Shard].
type Wal interface {
	// Commit finalize segment from encoder and write to wal.
	Commit() error
	// WalFlush flush all contetnt into wal.
	Flush() error
	// WalWrite append the incoming inner series to wal encoder.
	Write(innerSeriesSlice []*cppbridge.InnerSeries) (bool, error)
}

//
// Shard
//

// Shard bridge to labelset storage, data storage and wal.
type Shard[TWal Wal] struct {
	lss               *LSS
	dataStorage       *DataStorage
	wal               TWal
	lssLocker         sync.RWMutex
	dataStorageLocker sync.RWMutex
	walLocker         sync.Mutex
	id                uint16
}

// NewShard init new [Shard].
func NewShard[TWal Wal](
	lss *LSS,
	dataStorage *DataStorage,
	wal TWal,
	shardID uint16,
) *Shard[TWal] {
	return &Shard[TWal]{
		id:                shardID,
		lss:               lss,
		dataStorage:       dataStorage,
		wal:               wal,
		lssLocker:         sync.RWMutex{},
		dataStorageLocker: sync.RWMutex{},
		walLocker:         sync.Mutex{},
	}
}

// AppendInnerSeriesSlice add InnerSeries to [DataStorage].
func (s *Shard[TWal]) AppendInnerSeriesSlice(innerSeriesSlice []*cppbridge.InnerSeries) {
	s.dataStorageLocker.Lock()
	s.dataStorage.AppendInnerSeriesSlice(innerSeriesSlice)
	s.dataStorageLocker.Unlock()
}

// AppendRelabelerSeries add relabeled ls to lss, add to result and add to cache update(second stage).
func (s *Shard[TWal]) AppendRelabelerSeries(
	ctx context.Context,
	relabeler *cppbridge.InputPerShardRelabeler,
	shardsInnerSeries []*cppbridge.InnerSeries,
	shardsRelabeledSeries []*cppbridge.RelabeledSeries,
	shardsRelabelerStateUpdate []*cppbridge.RelabelerStateUpdate,
) (bool, error) {
	s.lssLocker.Lock()
	defer s.lssLocker.Unlock()

	return relabeler.AppendRelabelerSeries(
		ctx,
		s.lss.Target(),
		shardsInnerSeries,
		shardsRelabeledSeries,
		shardsRelabelerStateUpdate,
	)
}

// CopyAddedSeries copy label sets which were added via FindOrEmplace to destination.
func (s *Shard[TWal]) CopyAddedSeries(destination *Shard[TWal]) {
	s.lssLocker.RLock()
	s.lss.CopyAddedSeries(destination.lss.Target())
	s.lssLocker.RUnlock()
}

// DataStorageAllocatedMemory return size of allocated memory for [DataStorage].
func (s *Shard[TWal]) DataStorageAllocatedMemory() uint64 {
	s.dataStorageLocker.RLock()
	am := s.dataStorage.AllocatedMemory()
	s.dataStorageLocker.RUnlock()

	return am
}

// DataStorageInstantQuery returns samples for instant query from [DataStorage].
func (s *Shard[TWal]) DataStorageInstantQuery(
	maxt, valueNotFoundTimestampValue int64,
	ids []uint32,
) []cppbridge.Sample {
	s.dataStorageLocker.RLock()
	samples := s.dataStorage.InstantQuery(maxt, valueNotFoundTimestampValue, ids)
	s.dataStorageLocker.RUnlock()

	return samples
}

// DataStorageQuery returns serialized chunks from data storage.
func (s *Shard[TWal]) DataStorageQuery(
	query cppbridge.HeadDataStorageQuery,
) *cppbridge.HeadDataStorageSerializedChunks {
	s.dataStorageLocker.RLock()
	serializedChunks := s.dataStorage.Query(query)
	s.dataStorageLocker.RUnlock()

	return serializedChunks
}

// DataStorageQueryStatus get head status from [DataStorage].
func (s *Shard[TWal]) DataStorageQueryStatus(status *cppbridge.HeadStatus) {
	s.dataStorageLocker.RLock()
	status.FromDataStorage(s.dataStorage.Raw())
	s.dataStorageLocker.RUnlock()
}

// InputRelabeling relabeling incoming hashdex(first stage).
func (s *Shard[TWal]) InputRelabeling(
	ctx context.Context,
	relabeler *cppbridge.InputPerShardRelabeler,
	cache *cppbridge.Cache,
	options cppbridge.RelabelerOptions,
	shardedData cppbridge.ShardedData,
	shardsInnerSeries []*cppbridge.InnerSeries,
	shardsRelabeledSeries []*cppbridge.RelabeledSeries,
) (cppbridge.RelabelerStats, bool, error) {
	s.lssLocker.Lock()
	defer s.lssLocker.Unlock()

	return relabeler.InputRelabeling(
		ctx,
		s.lss.Input(),
		s.lss.Target(),
		cache,
		options,
		shardedData,
		shardsInnerSeries,
		shardsRelabeledSeries,
	)
}

// InputRelabelingFromCache relabeling incoming hashdex(first stage) from cache.
func (s *Shard[TWal]) InputRelabelingFromCache(
	ctx context.Context,
	relabeler *cppbridge.InputPerShardRelabeler,
	cache *cppbridge.Cache,
	options cppbridge.RelabelerOptions,
	shardedData cppbridge.ShardedData,
	shardsInnerSeries []*cppbridge.InnerSeries,
) (cppbridge.RelabelerStats, bool, error) {
	s.lssLocker.RLock()
	defer s.lssLocker.RUnlock()

	return relabeler.InputRelabelingFromCache(
		ctx,
		s.lss.Input(),
		s.lss.Target(),
		cache,
		options,
		shardedData,
		shardsInnerSeries,
	)
}

// InputRelabelingWithStalenans relabeling incoming hashdex(first stage) with state stalenans.
func (s *Shard[TWal]) InputRelabelingWithStalenans(
	ctx context.Context,
	relabeler *cppbridge.InputPerShardRelabeler,
	cache *cppbridge.Cache,
	options cppbridge.RelabelerOptions,
	staleNansState *cppbridge.StaleNansState,
	defTimestamp int64,
	shardedData cppbridge.ShardedData,
	shardsInnerSeries []*cppbridge.InnerSeries,
	shardsRelabeledSeries []*cppbridge.RelabeledSeries,
) (cppbridge.RelabelerStats, bool, error) {
	s.lssLocker.Lock()
	defer s.lssLocker.Unlock()

	return relabeler.InputRelabelingWithStalenans(
		ctx,
		s.lss.Input(),
		s.lss.Target(),
		cache,
		options,
		staleNansState,
		defTimestamp,
		shardedData,
		shardsInnerSeries,
		shardsRelabeledSeries,
	)
}

// InputRelabelingWithStalenansFromCache relabeling incoming hashdex(first stage) from cache with state stalenans.
func (s *Shard[TWal]) InputRelabelingWithStalenansFromCache(
	ctx context.Context,
	relabeler *cppbridge.InputPerShardRelabeler,
	cache *cppbridge.Cache,
	options cppbridge.RelabelerOptions,
	staleNansState *cppbridge.StaleNansState,
	defTimestamp int64,
	shardedData cppbridge.ShardedData,
	shardsInnerSeries []*cppbridge.InnerSeries,
) (cppbridge.RelabelerStats, bool, error) {
	s.lssLocker.RLock()
	defer s.lssLocker.RUnlock()

	return relabeler.InputRelabelingWithStalenansFromCache(
		ctx,
		s.lss.Input(),
		s.lss.Target(),
		cache,
		options,
		staleNansState,
		defTimestamp,
		shardedData,
		shardsInnerSeries,
	)
}

// LSSAllocatedMemory return size of allocated memory for labelset storages.
func (s *Shard[TWal]) LSSAllocatedMemory() uint64 {
	s.lssLocker.RLock()
	am := s.lss.AllocatedMemory()
	s.lssLocker.RUnlock()

	return am
}

// LSSQueryStatus get head status from lss.
func (s *Shard[TWal]) LSSQueryStatus(status *cppbridge.HeadStatus, limit int) {
	s.lssLocker.RLock()
	status.FromLSS(s.lss.Target(), limit)
	s.lssLocker.RUnlock()
}

// MergeOutOfOrderChunks merge chunks with out of order data chunks in [DataStorage].
func (s *Shard[TWal]) MergeOutOfOrderChunks() {
	s.dataStorageLocker.Lock()
	s.dataStorage.MergeOutOfOrderChunks()
	s.dataStorageLocker.Unlock()
}

// QueryLabelNames returns all the unique label names present in lss in sorted order.
func (s *Shard[TWal]) QueryLabelNames(
	matchers []model.LabelMatcher,
	dedupAdd func(shardID uint16, snapshot *cppbridge.LabelSetSnapshot, values []string),
) error {
	s.lssLocker.RLock()
	queryLabelNamesResult := s.lss.QueryLabelNames(matchers)
	snapshot := s.lss.GetSnapshot()
	s.lssLocker.RUnlock()

	if queryLabelNamesResult.Status() != cppbridge.LSSQueryStatusMatch {
		return fmt.Errorf("no matches on shard: %d", s.id)
	}

	dedupAdd(s.id, snapshot, queryLabelNamesResult.Names())
	runtime.KeepAlive(queryLabelNamesResult)

	return nil
}

// QueryLabelValues query labels values to lss and add values to
// the dedup-container that matches the given label matchers.
func (s *Shard[TWal]) QueryLabelValues(
	name string,
	matchers []model.LabelMatcher,
	dedupAdd func(shardID uint16, snapshot *cppbridge.LabelSetSnapshot, values []string),
) error {
	s.lssLocker.RLock()
	queryLabelValuesResult := s.lss.QueryLabelValues(name, matchers)
	snapshot := s.lss.GetSnapshot()
	s.lssLocker.RUnlock()

	if queryLabelValuesResult.Status() != cppbridge.LSSQueryStatusMatch {
		return fmt.Errorf("no matches on shard: %d", s.id)
	}

	dedupAdd(s.id, snapshot, queryLabelValuesResult.Values())
	runtime.KeepAlive(queryLabelValuesResult)

	return nil
}

// QuerySelector returns a created selector that matches the given label matchers.
func (s *Shard[TWal]) QuerySelector(matchers []model.LabelMatcher) (uintptr, *cppbridge.LabelSetSnapshot, error) {
	s.lssLocker.RLock()
	defer s.lssLocker.RUnlock()

	selector, status := s.lss.QuerySelector(matchers)
	switch status {
	case cppbridge.LSSQueryStatusMatch:
		return selector, s.lss.GetSnapshot(), nil

	case cppbridge.LSSQueryStatusNoMatch:
		return 0, nil, nil

	default:
		return 0, nil, fmt.Errorf(
			"failed to query selector from shard: %d, query status: %d", s.id, status,
		)
	}
}

// ShardID returns the shard ID.
func (s *Shard[TWal]) ShardID() uint16 {
	return s.id
}

// WalCommit finalize segment from encoder and write to wal.
func (s *Shard[TWal]) WalCommit() error {
	s.lssLocker.RLock()
	s.walLocker.Lock()

	err := s.wal.Commit()

	s.walLocker.Unlock()
	s.lssLocker.RUnlock()

	return err
}

// WalFlush flush all contetnt into wal.
func (s *Shard[TWal]) WalFlush() error {
	s.walLocker.Lock()

	err := s.wal.Flush()

	s.walLocker.Unlock()

	return err
}

// WalWrite append the incoming inner series to wal encoder.
func (s *Shard[TWal]) WalWrite(innerSeriesSlice []*cppbridge.InnerSeries) (bool, error) {
	s.walLocker.Lock()

	limitExhausted, err := s.wal.Write(innerSeriesSlice)

	s.walLocker.Unlock()

	return limitExhausted, err
}
