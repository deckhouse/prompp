// Shard - TODO write description on package

package shard

import (
	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

// Wal the minimum required Wal implementation for a [Shard].
type Wal interface {
	// Commit finalize segment from encoder and write to wal.
	Commit() error

	// CurrentSize returns current wal size.
	CurrentSize() int64

	// Flush flush all contetnt into wal.
	Flush() error

	// Write append the incoming inner series to wal encoder.
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

// AppendInnerSeriesSlice add InnerSeries to [DataStorage].
func (s *Shard[TWal]) AppendInnerSeriesSlice(innerSeriesSlice []*cppbridge.InnerSeries) {
	s.dataStorage.AppendInnerSeriesSlice(innerSeriesSlice)
}

// Close closes the wal segmentWriter.
func (s *Shard[TWal]) Close() error {
	return s.wal.Close()
}

// DSAllocatedMemory return size of allocated memory for [DataStorage].
func (s *Shard[TWal]) DSAllocatedMemory() uint64 {
	return s.dataStorage.AllocatedMemory()
}

// DataStorage returns shard [DataStorage].
func (s *Shard[TWal]) DataStorage() *DataStorage {
	return s.dataStorage
}

// LSS returns shard labelset storage [LSS].
func (s *Shard[TWal]) LSS() *LSS {
	return s.lss
}

// LSSAllocatedMemory return size of allocated memory for labelset storages.
func (s *Shard[TWal]) LSSAllocatedMemory() uint64 {
	return s.lss.AllocatedMemory()
}

// MergeOutOfOrderChunks merge chunks with out of order data chunks in [DataStorage].
func (s *Shard[TWal]) MergeOutOfOrderChunks() {
	s.dataStorage.MergeOutOfOrderChunks()
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

// WalCurrentSize returns current [Wal] size.
func (s *Shard[TWal]) WalCurrentSize() int64 {
	return s.wal.CurrentSize()
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
