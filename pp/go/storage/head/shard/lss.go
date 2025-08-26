package shard

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
)

// LSS labelset storage for [shard].
type LSS struct {
	input              *cppbridge.LabelSetStorage
	target             *cppbridge.LabelSetStorage
	snapshot           *cppbridge.LabelSetSnapshot
	dstSrcLsIdsMapping *cppbridge.IdsMapping
	locker             sync.RWMutex
	once               sync.Once
}

// NewLSS init new [LSS].
func NewLSS() *LSS {
	return &LSS{
		input:  cppbridge.NewLssStorage(),
		target: cppbridge.NewQueryableLssStorage(),
	}
}

// NewLSS init new [LSS].
func NewLSS() *LSS {
	return &LSS{
		input:  cppbridge.NewLssStorage(),
		target: cppbridge.NewQueryableLssStorage(),
	}
}

// AllocatedMemory return size of allocated memory for labelset storages.
func (l *LSS) AllocatedMemory() uint64 {
	l.locker.RLock()
	am := l.input.AllocatedMemory() + l.target.AllocatedMemory()
	l.locker.RUnlock()

	return am
}

// CopyAddedSeriesTo copy the label sets from the source lss to the destination lss that were added source lss.
func (l *LSS) CopyAddedSeriesTo(destination *LSS) {
	l.locker.RLock()
	snapshot := l.getSnapshot()
	bitsetSeries := l.target.BitsetSeries()
	l.locker.RUnlock()

	destination.dstSrcLsIdsMapping = snapshot.CopyAddedSeries(bitsetSeries, destination.target)
}

// Input returns input lss.
func (l *LSS) Input() *cppbridge.LabelSetStorage {
	return l.input
}

// QueryLabelNames add to dedup all the unique label names present in lss in sorted order.
func (l *LSS) QueryLabelNames(
	shardID uint16,
	matchers []model.LabelMatcher,
	dedupAdd func(shardID uint16, snapshot *cppbridge.LabelSetSnapshot, values []string),
) error {
	l.locker.RLock()
	defer l.locker.RUnlock()

	queryLabelNamesResult := l.target.QueryLabelNames(matchers)

	if queryLabelNamesResult.Status() != cppbridge.LSSQueryStatusMatch {
		return fmt.Errorf("no matches on shard: %d", shardID)
	}

	dedupAdd(shardID, l.getSnapshot(), queryLabelNamesResult.Names())
	runtime.KeepAlive(queryLabelNamesResult)

	return nil
}

// QueryLabelValues query labels values to [LSS] and add values to
// the dedup-container that matches the given label matchers.
func (l *LSS) QueryLabelValues(
	shardID uint16,
	name string,
	matchers []model.LabelMatcher,
	dedupAdd func(shardID uint16, snapshot *cppbridge.LabelSetSnapshot, values []string),
) error {
	l.locker.RLock()
	defer l.locker.RUnlock()

	queryLabelValuesResult := l.target.QueryLabelValues(name, matchers)

	if queryLabelValuesResult.Status() != cppbridge.LSSQueryStatusMatch {
		return fmt.Errorf("no matches on shard: %d", shardID)
	}

	dedupAdd(shardID, l.getSnapshot(), queryLabelValuesResult.Values())
	runtime.KeepAlive(queryLabelValuesResult)

	return nil
}

// QuerySelector returns a created selector that matches the given label matchers.
func (l *LSS) QuerySelector(shardID uint16, matchers []model.LabelMatcher) (
	uintptr,
	*cppbridge.LabelSetSnapshot,
	error,
) {
	l.locker.RLock()
	defer l.locker.RUnlock()

	selector, status := l.target.QuerySelector(matchers)
	switch status {
	case cppbridge.LSSQueryStatusMatch:
		return selector, l.getSnapshot(), nil

	case cppbridge.LSSQueryStatusNoMatch:
		return 0, nil, nil

	default:
		return 0, nil, fmt.Errorf(
			"failed to query selector from shard: %d, query status: %d", shardID, status,
		)
	}
}

// QueryStatus get head status from [LSS].
func (l *LSS) QueryStatus(status *cppbridge.HeadStatus, limit int) {
	l.locker.RLock()
	status.FromLSS(l.target, limit)
	l.locker.RUnlock()
}

// ResetSnapshot resets the current snapshot. Use only WithLock.
func (l *LSS) ResetSnapshot() {
	l.snapshot = nil
	l.once = sync.Once{}
}

// Target returns main [LSS].
func (l *LSS) Target() *cppbridge.LabelSetStorage {
	return l.target
}

// WithLock calls fn on raws [cppbridge.LabelSetStorage] with write lock.
func (l *LSS) WithLock(fn func(target, input *cppbridge.LabelSetStorage) error) error {
	l.locker.Lock()
	err := fn(l.target, l.input)
	l.locker.Unlock()

	return err
}

// WithRLock calls fn on raws [cppbridge.LabelSetStorage] with read lock.
func (l *LSS) WithRLock(fn func(target, input *cppbridge.LabelSetStorage) error) error {
	l.locker.RLock()
	err := fn(l.target, l.input)
	l.locker.RUnlock()

	return err
}

// getSnapshot return the actual snapshot.
func (l *LSS) getSnapshot() *cppbridge.LabelSetSnapshot {
	l.once.Do(func() {
		l.snapshot = l.target.CreateLabelSetSnapshot()
	})

	return l.snapshot
}
