// Shard - TODO write description on package

package shard

import (
	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

// Wal the minimum required Wal implementation for a [Shard].
type Wal interface {
	// Commit finalize segment from encoder and write to wal.
	Commit() error

	// WalFlush flush all contetnt into wal.
	Flush() error

	// WalWrite append the incoming inner series to wal encoder.
	Write(innerSeriesSlice []*cppbridge.InnerSeries) (bool, error)

	// Close closes the wal segmentWriter.
	Close() error
}

//
// Shard
//

// Shard bridge to labelset storage, data storage and wal.
type Shard[TWal Wal] struct {
	lss         *LSS
	dataStorage *DataStorage
	wal         TWal
	id          uint16
}

// NewShard init new [Shard].
func NewShard[TWal Wal](
	lss *LSS,
	dataStorage *DataStorage,
	wal TWal,
	shardID uint16,
) *Shard[TWal] {
	return &Shard[TWal]{
		id:          shardID,
		lss:         lss,
		dataStorage: dataStorage,
		wal:         wal,
	}
}

// Close closes the wal segmentWriter.
func (s *Shard[TWal]) Close() error {
	return s.wal.Close()
}

// DataStorage returns shard [DataStorage].
func (s *Shard[TWal]) DataStorage() *DataStorage {
	return s.dataStorage
}

// LSS returns shard labelset storage [LSS].
func (s *Shard[TWal]) LSS() *LSS {
	return s.lss
}

// ShardID returns the shard ID.
func (s *Shard[TWal]) ShardID() uint16 {
	return s.id
}

// Wal returns write-ahead log.
func (s *Shard[TWal]) Wal() TWal {
	return s.wal
}

// WalCommit finalize segment from encoder and write to wal.
func (s *Shard[TWal]) WalCommit() error {
	return s.lss.WithRLock(func(_, _ *cppbridge.LabelSetStorage) error {
		return s.wal.Commit()
	})
}

// WalFlush flush all contetnt into wal.
func (s *Shard[TWal]) WalFlush() error {
	return s.wal.Flush()
}

// WalWrite append the incoming inner series to wal encoder.
func (s *Shard[TWal]) WalWrite(innerSeriesSlice []*cppbridge.InnerSeries) (bool, error) {
	return s.wal.Write(innerSeriesSlice)
}

//
// PerGoroutineShard
//

// PerGoroutineShard wrapper of shard with [PerGoroutineRelabeler] for goroutines.
type PerGoroutineShard[TWal Wal] struct {
	relabeler *cppbridge.PerGoroutineRelabeler
	*Shard[TWal]
}

// NewPerGoroutineShard init new [PerGoroutineShard].
func NewPerGoroutineShard[TWal Wal](s *Shard[TWal], numberOfShards uint16) *PerGoroutineShard[TWal] {
	return &PerGoroutineShard[TWal]{
		relabeler: cppbridge.NewPerGoroutineRelabeler(numberOfShards, s.ShardID()),
		Shard:     s,
	}
}

// Relabeler returns relabeler for shard goroutines.
func (s *PerGoroutineShard[TWal]) Relabeler() *cppbridge.PerGoroutineRelabeler {
	return s.relabeler
}

// // InputRelabeling relabeling incoming hashdex(first stage).
// func (s *Shard[TWal]) InputRelabeling(
// 	ctx context.Context,
// 	relabeler *cppbridge.InputPerShardRelabeler,
// 	cache *cppbridge.Cache,
// 	options cppbridge.RelabelerOptions,
// 	shardedData cppbridge.ShardedData,
// 	shardsInnerSeries []*cppbridge.InnerSeries,
// 	shardsRelabeledSeries []*cppbridge.RelabeledSeries,
// ) (cppbridge.RelabelerStats, bool, error) {
// 	s.lssLocker.Lock()
// 	defer s.lssLocker.Unlock()

// 	return relabeler.InputRelabeling(
// 		ctx,
// 		s.lss.Input(),
// 		s.lss.Target(),
// 		cache,
// 		options,
// 		shardedData,
// 		shardsInnerSeries,
// 		shardsRelabeledSeries,
// 	)
// }

// // InputRelabelingFromCache relabeling incoming hashdex(first stage) from cache.
// func (s *Shard[TWal]) InputRelabelingFromCache(
// 	ctx context.Context,
// 	relabeler *cppbridge.InputPerShardRelabeler,
// 	cache *cppbridge.Cache,
// 	options cppbridge.RelabelerOptions,
// 	shardedData cppbridge.ShardedData,
// 	shardsInnerSeries []*cppbridge.InnerSeries,
// ) (cppbridge.RelabelerStats, bool, error) {
// 	s.lssLocker.RLock()
// 	defer s.lssLocker.RUnlock()

// 	return relabeler.InputRelabelingFromCache(
// 		ctx,
// 		s.lss.Input(),
// 		s.lss.Target(),
// 		cache,
// 		options,
// 		shardedData,
// 		shardsInnerSeries,
// 	)
// }

// // InputRelabelingWithStalenans relabeling incoming hashdex(first stage) with state stalenans.
// func (s *Shard[TWal]) InputRelabelingWithStalenans(
// 	ctx context.Context,
// 	relabeler *cppbridge.InputPerShardRelabeler,
// 	cache *cppbridge.Cache,
// 	options cppbridge.RelabelerOptions,
// 	staleNansState *cppbridge.StaleNansState,
// 	defTimestamp int64,
// 	shardedData cppbridge.ShardedData,
// 	shardsInnerSeries []*cppbridge.InnerSeries,
// 	shardsRelabeledSeries []*cppbridge.RelabeledSeries,
// ) (cppbridge.RelabelerStats, bool, error) {
// 	s.lssLocker.Lock()
// 	defer s.lssLocker.Unlock()

// 	return relabeler.InputRelabelingWithStalenans(
// 		ctx,
// 		s.lss.Input(),
// 		s.lss.Target(),
// 		cache,
// 		options,
// 		staleNansState,
// 		defTimestamp,
// 		shardedData,
// 		shardsInnerSeries,
// 		shardsRelabeledSeries,
// 	)
// }

// // InputRelabelingWithStalenansFromCache relabeling incoming hashdex(first stage) from cache with state stalenans.
// func (s *Shard[TWal]) InputRelabelingWithStalenansFromCache(
// 	ctx context.Context,
// 	relabeler *cppbridge.InputPerShardRelabeler,
// 	cache *cppbridge.Cache,
// 	options cppbridge.RelabelerOptions,
// 	staleNansState *cppbridge.StaleNansState,
// 	defTimestamp int64,
// 	shardedData cppbridge.ShardedData,
// 	shardsInnerSeries []*cppbridge.InnerSeries,
// ) (cppbridge.RelabelerStats, bool, error) {
// 	s.lssLocker.RLock()
// 	defer s.lssLocker.RUnlock()

// 	return relabeler.InputRelabelingWithStalenansFromCache(
// 		ctx,
// 		s.lss.Input(),
// 		s.lss.Target(),
// 		cache,
// 		options,
// 		staleNansState,
// 		defTimestamp,
// 		shardedData,
// 		shardsInnerSeries,
// 	)
// }
