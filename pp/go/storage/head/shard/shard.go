// Shard - TODO write description on package

package shard

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
)

//
// Shard
//

// Shard bridge to labelset storage, data storage and wal.
type Shard[TWal any] struct {
	lss               *LSS
	dataStorage       *DataStorage
	wal               TWal
	lssLocker         sync.RWMutex
	dataStorageLocker sync.RWMutex
	// write -> append samples walLocker.Lock
	// commit -> lssLocker.Rlock walLocker.Lock
	// flush -> walLocker.Lock
	walLocker sync.Mutex
	id        uint16
}

// NewShard init new [Shard].
func NewShard[TWal any](
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

// ShardID returns the shard ID.
func (s *Shard[TWal]) ShardID() uint16 {
	return s.id
}
