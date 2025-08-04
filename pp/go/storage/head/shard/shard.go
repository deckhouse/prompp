package shard

import "sync"

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
// Shard
//

// Shard
type Shard struct {
	lss               *LSS
	dataStorage       *DataStorage
	wal               *Wal
	lssLocker         RWLockable
	dataStorageLocker RWLockable
	id                uint16
}

// NewShard init new [Shard].
func NewShard(
	lss *LSS,
	dataStorage *DataStorage,
	wal *Wal,
	shardID uint16,
	withLocker bool,
) *Shard {
	s := &Shard{
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
