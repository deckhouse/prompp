package shard

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/model"
)

const (
	// rotateDuration duration of cache rotation.
	rotateDuration = 5 * time.Minute
)

// LSS labelset storage for [shard].
type LSS struct {
	input              *cppbridge.LabelSetStorage
	target             *cppbridge.LSSWithSnapshot
	dstSrcLsIdsMapping *cppbridge.IdsMapping
	lsCache            *model.CacheWithBitset
	locker             sync.RWMutex
	stopc              chan struct{}
	stopOnce           sync.Once
}

// NewLSS init new [LSS].
func NewLSS() *LSS {
	l := &LSS{
		input:    cppbridge.NewLssStorage(),
		target:   cppbridge.NewLSSWithSnapshotWithoutBitset(cppbridge.NewQueryableLssStorage()),
		lsCache:  model.NewCacheWithBitset(),
		stopc:    make(chan struct{}),
		stopOnce: sync.Once{},
	}

	go l.rotateCache(l.stopc)

	return l
}

// AllocatedMemory return size of allocated memory for labelset storages.
func (l *LSS) AllocatedMemory() uint64 {
	l.locker.RLock()
	am := l.input.AllocatedMemory() + l.target.LSS().AllocatedMemory()
	l.locker.RUnlock()

	return am
}

// CacheStats returns current bitset count and cache size.
func (l *LSS) CacheStats() (cacheSize uint64, cacheBitsetCount uint32) {
	return l.lsCache.Stats()
}

// CopyAddedSeriesTo copy the label sets from the source lss to the destination lss that were added source lss.
func (l *LSS) CopyAddedSeriesTo(destination *LSS) {
	l.locker.RLock()
	snapshot := l.target.Snapshot()
	bitsetSeries := l.target.LSS().BitsetSeries()
	l.locker.RUnlock()

	destination.dstSrcLsIdsMapping = snapshot.CopyAddedSeries(bitsetSeries, destination.target.LSS())
}

// FindByHash label set by hash in cache.
func (l *LSS) FindByHash(
	hash uint64,
	builderSortedAdd []cppbridge.Label,
	builderSortedDel []string,
	builderSnapshot *cppbridge.LabelSetSnapshot,
	builderLSID uint32,
) (labels.Labels, bool) {
	lsID, length, ok := l.lsCache.Load(hash)
	if !ok {
		return labels.EmptyLabels(), false
	}

	l.locker.RLock()
	snapshot := l.target.Snapshot()
	l.locker.RUnlock()
	if ok := snapshot.LabelSetEqualWithBuilder(
		builderSortedAdd,
		builderSortedDel,
		builderSnapshot,
		builderLSID,
		lsID,
	); !ok {
		logger.Warnf("cache collision on hash: %d, lsID: %d, length: %d", hash, lsID, length)
		return labels.EmptyLabels(), false
	}

	return labels.NewLabelsWithLSS(
		snapshot,
		lsID,
		length,
	), true
}

// FindFromBuilder label set from builder in lss, return length ls, lsid and bool ok.
//
//revive:disable-next-line:flag-parameter this is not a flag, but a parameter
func (l *LSS) FindFromBuilder(
	sortedAdd []cppbridge.Label,
	sortedDel []string,
	snapshot *cppbridge.LabelSetSnapshot,
	hash uint64,
	lsID uint32,
	skipCache bool,
) (labels.Labels, bool) {
	l.locker.RLock()
	newlsID, length, find := l.target.LSS().FindFromBuilder(sortedAdd, sortedDel, snapshot, lsID)
	if !find {
		l.locker.RUnlock()
		return labels.EmptyLabels(), false
	}

	newSnapshot := l.target.Snapshot()
	l.locker.RUnlock()

	if !skipCache {
		l.lsCache.Store(hash, newlsID, length)
	}

	return labels.NewLabelsWithLSS(
		newSnapshot,
		newlsID,
		length,
	), true
}

// Input returns input lss.
func (l *LSS) Input() *cppbridge.LabelSetStorage {
	return l.input
}

// Outdate marked *LabelSetStorage is outdated.
func (l *LSS) Outdate() {
	l.target.Outdate()
}

// QueryLabelNames add to dedup all the unique label names present in lss in sorted order.
func (l *LSS) QueryLabelNames(
	shardID uint16,
	matchers []model.LabelMatcher,
	dedupAdd func(shardID uint16, snapshot *cppbridge.LabelSetSnapshot, values []string),
) error {
	l.locker.RLock()
	defer l.locker.RUnlock()

	queryLabelNamesResult := l.target.LSS().QueryLabelNames(matchers)

	if queryLabelNamesResult.Status() != cppbridge.LSSQueryStatusMatch {
		return fmt.Errorf("no matches on shard: %d", shardID)
	}

	dedupAdd(shardID, l.target.Snapshot(), queryLabelNamesResult.Names())
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

	queryLabelValuesResult := l.target.LSS().QueryLabelValues(name, matchers)

	if queryLabelValuesResult.Status() != cppbridge.LSSQueryStatusMatch {
		return fmt.Errorf("no matches on shard: %d", shardID)
	}

	dedupAdd(shardID, l.target.Snapshot(), queryLabelValuesResult.Values())
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

	selector, status := l.target.LSS().QuerySelector(matchers)
	switch status {
	case cppbridge.LSSQueryStatusMatch:
		return selector, l.target.Snapshot(), nil

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
	status.FromLSS(l.target.LSS(), limit)
	l.locker.RUnlock()
}

// ResetSnapshot resets the current snapshot. Use only WithLock.
func (l *LSS) ResetSnapshot() {
	l.target.ResetSnapshot()
}

// Stop [LSS] rotation cache.
func (l *LSS) Stop() {
	l.stopOnce.Do(func() {
		close(l.stopc)
	})
}

// Target returns main [LSS].
func (l *LSS) Target() *cppbridge.LabelSetStorage {
	return l.target.LSS()
}

// WithLock calls fn on raws [cppbridge.LabelSetStorage] with write lock.
func (l *LSS) WithLock(fn func(target, input *cppbridge.LabelSetStorage) error) error {
	l.locker.Lock()
	err := fn(l.target.LSS(), l.input)
	l.locker.Unlock()

	return err
}

// WithRLock calls fn on raws [cppbridge.LabelSetStorage] with read lock.
func (l *LSS) WithRLock(fn func(target, input *cppbridge.LabelSetStorage) error) error {
	l.locker.RLock()
	err := fn(l.target.LSS(), l.input)
	l.locker.RUnlock()

	return err
}

// rotateCache rotate head cache.
func (l *LSS) rotateCache(stopc chan struct{}) {
	rotateTimer := time.NewTimer(rotateDuration)

	for {
		select {
		case <-stopc:
			l.lsCache.Reset()
			return

		case <-rotateTimer.C:
			cacheSize, cacheBitsetCount := l.lsCache.StatsWithClearBitset()
			if uint64(cacheBitsetCount) <= cacheSize/2 { //revive:disable-line:add-constant // half of cache size
				l.lsCache.Reset()
			}

			rotateTimer.Reset(rotateDuration)
		}
	}
}
