package querier

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"unsafe"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/util/locker"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

// InstantQueryFeature is a feature flag for instant query when hints step is 0 and range is 0.
var InstantQueryFeature = false

const (
	// lssQueryInstantQuerySelector name of task.
	lssQueryInstantQuerySelector = "lss_query_instant_query_selector"
	// lssQueryRangeQuerySelector name of task.
	lssQueryRangeQuerySelector = "lss_query_range_query_selector"
	// lssLabelValuesQuerier name of task.
	lssLabelValuesQuerier = "lss_label_values_querier"
	// lssLabelNamesQuerier name of task.
	lssLabelNamesQuerier = "lss_label_names_querier"

	// dsQueryInstantQuerier name of task.
	dsQueryInstantQuerier = "data_storage_query_instant_querier"
	// dsQueryRangeQuerier name of task.
	dsQueryRangeQuerier = "data_storage_query_range_querier"

	// DefaultInstantQueryValueNotFoundTimestampValue default value for not found timestamp value.
	DefaultInstantQueryValueNotFoundTimestampValue int64 = 0
)

//
// Querier
//

// Querier provides querying access over time series data of a fixed time range.
type Querier[
	TTask Task,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
	THead Head[TTask, TDataStorage, TLSS, TShard],
] struct {
	mint             int64
	maxt             int64
	head             THead
	deduplicatorCtor deduplicatorCtor
	closer           func() error
	metrics          *Metrics
}

// NewQuerier init new [Querier].
func NewQuerier[
	TTask Task,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
	THead Head[TTask, TDataStorage, TLSS, TShard],
](
	head THead,
	deduplicatorCtor deduplicatorCtor,
	mint, maxt int64,
	closer func() error,
	metrics *Metrics,
) *Querier[TTask, TDataStorage, TLSS, TShard, THead] {
	return &Querier[TTask, TDataStorage, TLSS, TShard, THead]{
		mint:             mint,
		maxt:             maxt,
		head:             head,
		deduplicatorCtor: deduplicatorCtor,
		closer:           closer,
		metrics:          metrics,
	}
}

// Close [Querier] if need.
//
//revive:disable-next-line:confusing-naming // other type of querier.
func (q *Querier[TTask, TDataStorage, TLSS, TShard, THead]) Close() error {
	if q.closer != nil {
		return q.closer()
	}

	return nil
}

// LabelNames returns label values present in the head for the specific label name.
//
//revive:disable-next-line:confusing-naming // other type of querier.
func (q *Querier[TTask, TDataStorage, TLSS, TShard, THead]) LabelNames(
	ctx context.Context,
	hints *storage.LabelHints,
	matchers ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return queryLabelNames(
		ctx,
		q.head,
		q.deduplicatorCtor,
		q.metrics.startQuery(operationLabelNames),
		lssLabelNamesQuerier,
		hints,
		matchers...,
	)
}

// LabelValues returns label values present in the head for the specific label name
// that are within the time range mint to maxt. If matchers are specified the returned
// result set is reduced to label values of metrics matching the matchers.
//
//revive:disable-next-line:confusing-naming // other type of querier.
func (q *Querier[TTask, TDataStorage, TLSS, TShard, THead]) LabelValues(
	ctx context.Context,
	name string,
	hints *storage.LabelHints,
	matchers ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return queryLabelValues(
		ctx,
		name,
		q.head,
		q.deduplicatorCtor,
		q.metrics.startQuery(operationLabelValues),
		lssLabelValuesQuerier,
		hints,
		matchers...,
	)
}

// Select returns a set of series that matches the given label matchers.
//
//revive:disable-next-line:confusing-naming // other type of querier.
func (q *Querier[TTask, TDataStorage, TLSS, TShard, THead]) Select(
	ctx context.Context,
	sortSeries bool,
	hints *storage.SelectHints,
	matchers ...*labels.Matcher,
) storage.SeriesSet {
	if q.mint == q.maxt {
		return q.selectInstant(ctx, sortSeries, hints, matchers...)
	}

	if InstantQueryFeature && hints != nil && hints.Step == 0 && hints.Range == 0 {
		return q.selectInstant(ctx, sortSeries, hints, matchers...)
	}

	return q.selectRange(ctx, sortSeries, hints, matchers...)
}

// selectInstant returns a instant set of series that matches the given label matchers.
//
//revive:disable-next-line:function-length long but readable.
func (q *Querier[TTask, TDataStorage, TLSS, TShard, THead]) selectInstant(
	ctx context.Context,
	_ bool,
	_ *storage.SelectHints,
	matchers ...*labels.Matcher,
) storage.SeriesSet {
	resources, err := beginSelectQuery(ctx, q.head, q.metrics, operationSelectInstant)
	if err != nil {
		if errors.Is(err, locker.ErrSemaphoreClosed) {
			return &SeriesSet{}
		}

		logger.Warnf("[QUERIER]: select instant failed on the capture of the read lock query: %s", err)
		return storage.ErrSeriesSet(err)
	}

	defer resources.close()

	stats := &resources.stats
	poolProvider := resources.pool
	err = queryLss(lssQueryInstantQuerySelector, q.head, matchers, &resources)
	if err != nil {
		logger.Warnf("[QUERIER]: failed to instant: %s", err)
		return storage.ErrSeriesSet(err)
	}

	valueNotFoundTimestampValue := DefaultInstantQueryValueNotFoundTimestampValue
	if q.mint <= valueNotFoundTimestampValue {
		valueNotFoundTimestampValue = q.mint - 1
	}

	resources.seriesSets = poolProvider.GetSeriesSet()
	loadAndQueryWaiter := NewLoadAndQueryWaiter(q.head)
	// Capture slice headers so the shard closure does not move the mutable resource owner to the heap.
	snapshots := resources.snapshots
	lssQueryResults := resources.lssQueryResults
	seriesSets := resources.seriesSets
	slots := stats.slots
	tDataStorageQuery := q.head.CreateTask(
		dsQueryInstantQuerier,
		func(s TShard) error {
			shardID := s.ShardID()
			lssQueryResult := lssQueryResults[shardID]
			if lssQueryResult == nil {
				seriesSets[shardID] = &InstantSeriesSet{}
				return nil
			}

			instantSeries := NewInstantSeriesSlice(lssQueryResult.Len(), valueNotFoundTimestampValue)

			start := slots.Start()
			result := s.DataStorage().InstantQuery(
				q.maxt,
				lssQueryResult.IDs(),
				uintptr(unsafe.Pointer(unsafe.SliceData(instantSeries))), // #nosec G103 // it's meant to be that way
			)
			slots.Since(shardID, slotDataStorageQuery, start)

			if result.Status == cppbridge.DataStorageQueryStatusNeedDataLoad {
				loadAndQueryWaiter.Add(s, result.Querier)
			}

			seriesSets[shardID] = NewInstantSeriesSet(
				snapshots[shardID],
				valueNotFoundTimestampValue,
				instantSeries,
			)

			return nil
		},
	)
	resources.addTask(tDataStorageQuery)
	q.head.Enqueue(tDataStorageQuery)
	_ = tDataStorageQuery.Wait()
	stats.lap.Mark(stageDataStorageQuery)
	stats.shards.ObserveMax(shardStageDataStorageQuery, slots, slotDataStorageQuery)

	hasQueries := loadAndQueryWaiter.HasQueriesAfterPrimaryWait()
	err = loadAndQueryWaiter.Wait()
	if err != nil {
		SendUnrecoverableError(err)
		stats.lap.Mark(stageDataLoadWait)

		return storage.ErrSeriesSet(err)
	}

	if hasQueries {
		stats.lap.Mark(stageDataLoadWait)
	}

	result := NewMergeShardSeriesSet(resources.seriesSets)
	stats.lap.Mark(stageBuildResult)

	return result
}

// selectRange returns a range set of series that matches the given label matchers.
func (q *Querier[TTask, TDataStorage, TLSS, TShard, THead]) selectRange(
	ctx context.Context,
	_ bool,
	_ *storage.SelectHints,
	matchers ...*labels.Matcher,
) storage.SeriesSet {
	resources, err := beginSelectQuery(ctx, q.head, q.metrics, operationSelectRange)
	if err != nil {
		if errors.Is(err, locker.ErrSemaphoreClosed) {
			return &SeriesSet{}
		}

		logger.Warnf("[QUERIER]: select range failed on the capture of the read lock query: %s", err)
		return storage.ErrSeriesSet(err)
	}

	defer resources.close()

	stats := &resources.stats
	poolProvider := resources.pool
	err = queryLss(lssQueryRangeQuerySelector, q.head, matchers, &resources)
	if err != nil {
		logger.Warnf("[QUERIER]: failed to range: %s", err)
		return storage.ErrSeriesSet(err)
	}

	resources.serializedData = poolProvider.GetSerializedData()
	t := queryDataStorage(
		dsQueryRangeQuerier, q.head, resources.lssQueryResults, resources.serializedData, q.mint, q.maxt, stats,
	)
	resources.addTask(t)

	resources.seriesSets = poolProvider.GetSeriesSet()
	for shardID, serializedData := range resources.serializedData {
		if serializedData != nil {
			resources.seriesSets[shardID] = NewSeriesSet(
				q.mint,
				q.maxt,
				resources.lssQueryResults[shardID],
				resources.snapshots[shardID],
				serializedData,
			)
			continue
		}
		resources.seriesSets[shardID] = &SeriesSet{}
	}

	result := NewMergeShardSeriesSet(resources.seriesSets)
	stats.lap.Mark(stageBuildResult)

	return result
}

// convertPrometheusMatchersToPPMatchers converts prometheus matchers to pp matchers.
func convertPrometheusMatchersToPPMatchers(matchers ...*labels.Matcher) []model.LabelMatcher {
	promppMatchers := make([]model.LabelMatcher, len(matchers))
	for i := range matchers {
		promppMatchers[i].Name = matchers[i].Name
		promppMatchers[i].Value = matchers[i].Value
		promppMatchers[i].MatcherType = uint8(matchers[i].Type) // #nosec G115 // no overflow
	}

	return promppMatchers
}

// queryDataStorage returns serialized chunks from data storage for each shard.
func queryDataStorage[
	TTask Task,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
	THead Head[TTask, TDataStorage, TLSS, TShard],
](
	taskName string,
	head THead,
	lssQueryResults []*cppbridge.LSSQueryResult,
	shardedSerializedData []*cppbridge.DataStorageSerializedData,
	mint, maxt int64,
	stats *queryStats,
) TTask {
	loadAndQueryWaiter := NewLoadAndQueryWaiter(head)
	slots := stats.slots
	tDataStorageQuery := head.CreateTask(
		taskName,
		func(s TShard) error {
			shardID := s.ShardID()
			lssQueryResult := lssQueryResults[shardID]
			if lssQueryResult == nil {
				return nil
			}

			start := slots.Start()
			result := s.DataStorage().Query(cppbridge.DataStorageQuery{
				StartTimestampMs: mint,
				EndTimestampMs:   maxt,
				LabelSetIDs:      lssQueryResult.IDs(),
			})
			slots.Since(shardID, slotDataStorageQuery, start)

			if result.Status == cppbridge.DataStorageQueryStatusNeedDataLoad {
				loadAndQueryWaiter.Add(s, result.Querier)
			}
			shardedSerializedData[s.ShardID()] = result.SerializedData

			return nil
		},
	)
	head.Enqueue(tDataStorageQuery)
	_ = tDataStorageQuery.Wait()
	stats.lap.Mark(stageDataStorageQuery)
	stats.shards.ObserveMax(shardStageDataStorageQuery, slots, slotDataStorageQuery)

	hasQueries := loadAndQueryWaiter.HasQueriesAfterPrimaryWait()
	if err := loadAndQueryWaiter.Wait(); err != nil {
		clear(shardedSerializedData)
		SendUnrecoverableError(err)
	}

	if hasQueries {
		stats.lap.Mark(stageDataLoadWait)
	}

	return tDataStorageQuery
}

// queryLabelNames returns label names present in the head for the given matchers.
func queryLabelNames[
	TTask Task,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
	THead Head[TTask, TDataStorage, TLSS, TShard],
](
	ctx context.Context,
	head THead,
	deduplicatorCtor deduplicatorCtor,
	stats queryStats,
	taskName string,
	hints *storage.LabelHints,
	matchers ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	anns := *annotations.New()
	release, err := head.AcquireQuery(ctx)
	stats.lap.Mark(metadataStageSemaphoreWait)

	if err != nil {
		if errors.Is(err, locker.ErrSemaphoreClosed) {
			return nil, anns, nil
		}

		logger.Warnf("[QUERIER]: label names failed on the capture of the read lock query: %s", err)
		return nil, anns, err
	}

	poolProvider := head.PoolProvider()
	resources := newQueryResources(poolProvider, head, stats, release)
	defer resources.close()

	if resources.stats.shards.Enabled() {
		resources.slots = poolProvider.GetShardSlots()
		resources.stats.slots = resources.slots
	}

	dedup := deduplicatorCtor(head.NumberOfShards())
	convertedMatchers := convertPrometheusMatchersToPPMatchers(matchers...)
	resources.stats.lap.Mark(metadataStagePrepare)
	slots := resources.stats.slots

	t := head.CreateTask(
		taskName,
		func(shard TShard) error {
			start := slots.Start()
			err := shard.LSS().QueryLabelNames(shard.ShardID(), convertedMatchers, dedup.Add)
			slots.Since(shard.ShardID(), slotLSS, start)

			return err
		},
	)
	resources.addTask(t)
	head.Enqueue(t)

	if err := t.Wait(); err != nil {
		anns.Add(err)
	}
	resources.stats.shards.ObserveMax(metadataShardStageLSSQuery, slots, slotLSS)

	select {
	case <-ctx.Done():
		resources.stats.lap.Mark(metadataStageLSSQuery)

		return nil, anns, context.Cause(ctx)
	default:
	}
	resources.stats.lap.Mark(metadataStageLSSQuery)

	lns := dedup.Values()
	sort.Strings(lns)

	if hints.Limit > 0 && hints.Limit < len(lns) {
		lns = lns[:hints.Limit]
	}
	resources.stats.lap.Mark(metadataStageBuildResult)

	return lns, anns, nil
}

// queryLabelValues returns label values present in the head for the specific label name.
func queryLabelValues[
	TTask Task,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
	THead Head[TTask, TDataStorage, TLSS, TShard],
](
	ctx context.Context,
	name string,
	head THead,
	deduplicatorCtor deduplicatorCtor,
	stats queryStats,
	taskName string,
	_ *storage.LabelHints,
	matchers ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	anns := *annotations.New()
	release, err := head.AcquireQuery(ctx)
	stats.lap.Mark(metadataStageSemaphoreWait)

	if err != nil {
		if errors.Is(err, locker.ErrSemaphoreClosed) {
			return nil, anns, nil
		}

		logger.Warnf("[QUERIER]: label values failed on the capture of the read lock query: %s", err)
		return nil, anns, err
	}

	poolProvider := head.PoolProvider()
	resources := newQueryResources(poolProvider, head, stats, release)
	defer resources.close()

	if resources.stats.shards.Enabled() {
		resources.slots = poolProvider.GetShardSlots()
		resources.stats.slots = resources.slots
	}

	dedup := deduplicatorCtor(head.NumberOfShards())
	convertedMatchers := convertPrometheusMatchersToPPMatchers(matchers...)
	resources.stats.lap.Mark(metadataStagePrepare)
	slots := resources.stats.slots

	t := head.CreateTask(
		taskName,
		func(shard TShard) error {
			start := slots.Start()
			err := shard.LSS().QueryLabelValues(shard.ShardID(), name, convertedMatchers, dedup.Add)
			slots.Since(shard.ShardID(), slotLSS, start)

			return err
		},
	)
	resources.addTask(t)
	head.Enqueue(t)

	if err := t.Wait(); err != nil {
		anns.Add(err)
	}
	resources.stats.shards.ObserveMax(metadataShardStageLSSQuery, slots, slotLSS)

	select {
	case <-ctx.Done():
		resources.stats.lap.Mark(metadataStageLSSQuery)

		return nil, anns, context.Cause(ctx)
	default:
	}
	resources.stats.lap.Mark(metadataStageLSSQuery)

	lvs := dedup.Values()
	sort.Strings(lvs)
	resources.stats.lap.Mark(metadataStageBuildResult)

	return lvs, anns, nil
}

// closeLSSQueryResults releases the C-allocated buffers held by the query results.
// It is safe to call once every series set built from these results has copied out
// (or no longer needs) the data it references. Nil entries are skipped and Close is
// idempotent, so the finalizer stays a harmless fallback.
func closeLSSQueryResults(lssQueryResults []*cppbridge.LSSQueryResult) {
	for _, lssQueryResult := range lssQueryResults {
		if lssQueryResult != nil {
			lssQueryResult.Close()
		}
	}
}

// queryLss fills snapshots and LSS results and registers every buffer and task before using them.
//
//revive:disable-next-line:cyclomatic but readable.
//revive:disable-next-line:function-length long but readable.
func queryLss[
	TTask Task,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
	THead Head[TTask, TDataStorage, TLSS, TShard],
](
	taskName string,
	head THead,
	matchers []*labels.Matcher,
	resources *queryResources[TTask, TShard],
) error {
	stats := &resources.stats

	if stats.shards.Enabled() {
		resources.slots = resources.pool.GetShardSlots()
		stats.slots = resources.slots
	}

	resources.snapshots = resources.pool.GetSnapshots()
	resources.lssQueryResults = resources.pool.GetLSSQueryResults()
	resources.selectors = resources.pool.GetSelectors()
	resources.errors = resources.pool.GetErrors()
	convertedMatchers := convertPrometheusMatchersToPPMatchers(matchers...)
	stats.lap.Mark(stagePrepare)

	// Capture slice headers so shard tasks do not retain the mutable resource owner.
	selectors := resources.selectors
	snapshots := resources.snapshots
	slots := stats.slots
	tLSSQuerySelector := head.CreateTask(
		taskName,
		func(shard TShard) (err error) {
			shardID := shard.ShardID()
			start := slots.Start()
			selectors[shardID], snapshots[shardID], err = shard.LSS().QuerySelector(shardID, convertedMatchers)
			slots.Since(shardID, slotLSS, start)

			return err
		},
	)
	resources.addTask(tLSSQuerySelector)
	head.Enqueue(tLSSQuerySelector)
	err := tLSSQuerySelector.Wait()
	stats.lap.Mark(stageLSSQuerySelector)
	stats.shards.ObserveMax(shardStageLSSQuerySelector, slots, slotLSS)

	if err != nil {
		return err
	}

	for shardID, selector := range selectors {
		if selector == 0 {
			continue
		}

		lssQueryResult := snapshots[shardID].Query(selector)
		switch lssQueryResult.Status() {
		case cppbridge.LSSQueryStatusMatch:
			resources.lssQueryResults[shardID] = lssQueryResult
		case cppbridge.LSSQueryStatusNoMatch:
		default:
			resources.errors[shardID] = fmt.Errorf(
				"failed to query from shard: %d, query status: %d", shardID, lssQueryResult.Status(),
			)
		}
	}

	err = errors.Join(resources.errors...)
	stats.lap.Mark(stageLSSQuery)

	return err
}

// UnrecoverableErrorChan channel singal for [UnrecoverableError].
var UnrecoverableErrorChan = make(chan error)

// SendUnrecoverableError send to terminate on [UnrecoverableError].
func SendUnrecoverableError(err error) {
	if err != nil {
		logger.Warnf("Unrecoverable error: %v", err)
	}

	select {
	case UnrecoverableErrorChan <- UnrecoverableError{err}:
	default:
	}
}

// UnrecoverableError error if Head get unrecoverable error.
type UnrecoverableError struct {
	err error
}

// Error implements error.
func (err UnrecoverableError) Error() string {
	return fmt.Sprintf("Unrecoverable error: %v", err.err)
}

// Is implements errors.Is interface.
func (UnrecoverableError) Is(target error) bool {
	_, ok := target.(UnrecoverableError)
	return ok
}
