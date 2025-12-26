package querier

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"
	"unsafe"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/util/locker"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

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
	mint               int64
	maxt               int64
	longtermIntervalMs int64
	head               THead
	deduplicatorCtor   deduplicatorCtor
	closer             func() error
	metrics            *Metrics
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
	mint, maxt, longtermIntervalMs int64,
	closer func() error,
	metrics *Metrics,
) *Querier[TTask, TDataStorage, TLSS, TShard, THead] {
	return &Querier[TTask, TDataStorage, TLSS, TShard, THead]{
		mint:               mint,
		maxt:               maxt,
		longtermIntervalMs: longtermIntervalMs,
		head:               head,
		deduplicatorCtor:   deduplicatorCtor,
		closer:             closer,
		metrics:            metrics,
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
		q.metrics,
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
		q.metrics,
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
	start := time.Now()

	release, err := q.head.AcquireQuery(ctx)
	if err != nil {
		if errors.Is(err, locker.ErrSemaphoreClosed) {
			return &SeriesSet{}
		}

		logger.Warnf("[QUERIER]: select instant failed on the capture of the read lock query: %s", err)
		return storage.ErrSeriesSet(err)
	}
	defer release()

	defer func() {
		if q.metrics != nil {
			q.metrics.SelectDuration.With(
				prometheus.Labels{"query_type": "instant"},
			).Observe(float64(time.Since(start).Microseconds()))
		}
	}()

	lssQueryResults, snapshots, err := queryLss(lssQueryInstantQuerySelector, q.head, matchers)
	if err != nil {
		logger.Warnf("[QUERIER]: failed to instant: %s", err)
		return storage.ErrSeriesSet(err)
	}

	valueNotFoundTimestampValue := DefaultInstantQueryValueNotFoundTimestampValue
	if q.mint <= valueNotFoundTimestampValue {
		valueNotFoundTimestampValue = q.mint - 1
	}

	numberOfShards := q.head.NumberOfShards()
	seriesSets := make([]storage.SeriesSet, numberOfShards)
	loadAndQueryWaiter := NewLoadAndQueryWaiter[TTask, TDataStorage, TLSS, TShard, THead](q.head)
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

			result := s.DataStorage().InstantQuery(q.maxt, lssQueryResult.IDs(), uintptr(unsafe.Pointer(unsafe.SliceData(instantSeries))))
			if result.Status == cppbridge.DataStorageQueryStatusNeedDataLoad {
				loadAndQueryWaiter.Add(s, result.Querier)
			}

			seriesSets[shardID] = NewInstantSeriesSet(
				lssQueryResult,
				snapshots[shardID],
				valueNotFoundTimestampValue,
				instantSeries,
			)

			return nil
		},
	)
	q.head.Enqueue(tDataStorageQuery)
	_ = tDataStorageQuery.Wait()

	if err = loadAndQueryWaiter.Wait(); err != nil {
		SendUnrecoverableError(err)
		return storage.ErrSeriesSet(err)
	}

	return storage.NewMergeSeriesSet(seriesSets, storage.ChainedSeriesMerge)
}

// selectRange returns a range set of series that matches the given label matchers.
func (q *Querier[TTask, TDataStorage, TLSS, TShard, THead]) selectRange(
	ctx context.Context,
	_ bool,
	_ *storage.SelectHints,
	matchers ...*labels.Matcher,
) storage.SeriesSet {
	start := time.Now()

	release, err := q.head.AcquireQuery(ctx)
	if err != nil {
		if errors.Is(err, locker.ErrSemaphoreClosed) {
			return &SeriesSet{}
		}

		logger.Warnf("[QUERIER]: select range failed on the capture of the read lock query: %s", err)
		return storage.ErrSeriesSet(err)
	}
	defer release()

	defer func() {
		if q.metrics != nil {
			q.metrics.SelectDuration.With(
				prometheus.Labels{"query_type": "range"},
			).Observe(float64(time.Since(start).Microseconds()))
		}
	}()

	lssQueryResults, snapshots, err := queryLss(lssQueryRangeQuerySelector, q.head, matchers)
	if err != nil {
		logger.Warnf("[QUERIER]: failed to range: %s", err)
		return storage.ErrSeriesSet(err)
	}

	shardedSerializedData := queryDataStorage(
		dsQueryRangeQuerier,
		q.head,
		lssQueryResults,
		q.mint,
		q.maxt,
		q.longtermIntervalMs,
	)
	seriesSets := make([]storage.SeriesSet, q.head.NumberOfShards())
	for shardID, serializedData := range shardedSerializedData {
		if serializedData != nil {
			seriesSets[shardID] = NewSeriesSet(q.mint, q.maxt, lssQueryResults[shardID], snapshots[shardID], serializedData)
			continue
		}
		seriesSets[shardID] = &SeriesSet{}
	}

	return storage.NewMergeSeriesSet(seriesSets, storage.ChainedSeriesMerge)
}

// convertPrometheusMatchersToPPMatchers converts prometheus matchers to pp matchers.
func convertPrometheusMatchersToPPMatchers(matchers ...*labels.Matcher) []model.LabelMatcher {
	promppMatchers := make([]model.LabelMatcher, 0, len(matchers))
	for _, matcher := range matchers {
		promppMatchers = append(promppMatchers, model.LabelMatcher{
			Name:        matcher.Name,
			Value:       matcher.Value,
			MatcherType: uint8(matcher.Type), // #nosec G115 // no overflow
		})
	}

	return promppMatchers
}

// queryDataStorageV2 returns serialized chunks from data storage for each shard.
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
	mint, maxt, longtermIntervalMs int64,
) []*cppbridge.DataStorageSerializedData {
	shardedSerializedData := make([]*cppbridge.DataStorageSerializedData, head.NumberOfShards())
	loadAndQueryWaiter := NewLoadAndQueryWaiter[TTask, TDataStorage, TLSS, TShard, THead](head)
	tDataStorageQuery := head.CreateTask(
		taskName,
		func(s TShard) error {
			shardID := s.ShardID()
			lssQueryResult := lssQueryResults[shardID]
			if lssQueryResult == nil {
				return nil
			}

			var result cppbridge.DataStorageQueryResult
			result = s.DataStorage().Query(
				cppbridge.DataStorageQuery{
					StartTimestampMs: mint,
					EndTimestampMs:   maxt,
					LabelSetIDs:      lssQueryResult.IDs(),
				},
				longtermIntervalMs,
			)
			if result.Status == cppbridge.DataStorageQueryStatusNeedDataLoad {
				loadAndQueryWaiter.Add(s, result.Querier)
			}
			shardedSerializedData[s.ShardID()] = result.SerializedData

			return nil
		},
	)
	head.Enqueue(tDataStorageQuery)
	_ = tDataStorageQuery.Wait()

	if err := loadAndQueryWaiter.Wait(); err != nil {
		SendUnrecoverableError(err)
		return make([]*cppbridge.DataStorageSerializedData, head.NumberOfShards())
	}

	return shardedSerializedData
}

// queryLabelValues returns label values present in the head for the specific label name.
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
	metrics *Metrics,
	taskName string,
	hints *storage.LabelHints,
	matchers ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	start := time.Now()

	anns := *annotations.New()
	release, err := head.AcquireQuery(ctx)
	if err != nil {
		if errors.Is(err, locker.ErrSemaphoreClosed) {
			return nil, anns, nil
		}

		logger.Warnf("[QUERIER]: label names failed on the capture of the read lock query: %s", err)
		return nil, anns, err
	}
	defer release()

	defer func() {
		if metrics != nil {
			metrics.LabelNamesDuration.Observe(float64(time.Since(start).Microseconds()))
		}
	}()

	dedup := deduplicatorCtor(head.NumberOfShards())
	convertedMatchers := convertPrometheusMatchersToPPMatchers(matchers...)

	t := head.CreateTask(
		taskName,
		func(shard TShard) error {
			return shard.LSS().QueryLabelNames(shard.ShardID(), convertedMatchers, dedup.Add)
		},
	)
	head.Enqueue(t)

	if err := t.Wait(); err != nil {
		anns.Add(err)
	}

	select {
	case <-ctx.Done():
		return nil, anns, context.Cause(ctx)
	default:
	}

	lns := dedup.Values()
	sort.Strings(lns)

	if hints.Limit > 0 && hints.Limit < len(lns) {
		return lns[:hints.Limit], anns, nil
	}
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
	metrics *Metrics,
	taskName string,
	_ *storage.LabelHints,
	matchers ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	start := time.Now()

	anns := *annotations.New()
	release, err := head.AcquireQuery(ctx)
	if err != nil {
		if errors.Is(err, locker.ErrSemaphoreClosed) {
			return nil, anns, nil
		}

		logger.Warnf("[QUERIER]: label values failed on the capture of the read lock query: %s", err)
		return nil, anns, err
	}
	defer release()

	defer func() {
		if metrics != nil {
			metrics.LabelValuesDuration.Observe(float64(time.Since(start).Microseconds()))
		}
	}()

	dedup := deduplicatorCtor(head.NumberOfShards())
	convertedMatchers := convertPrometheusMatchersToPPMatchers(matchers...)

	t := head.CreateTask(
		taskName,
		func(shard TShard) error {
			return shard.LSS().QueryLabelValues(shard.ShardID(), name, convertedMatchers, dedup.Add)
		},
	)
	head.Enqueue(t)

	if err := t.Wait(); err != nil {
		anns.Add(err)
	}

	select {
	case <-ctx.Done():
		return nil, anns, context.Cause(ctx)
	default:
	}

	lvs := dedup.Values()
	sort.Strings(lvs)

	return lvs, anns, nil
}

// lssQuery returns query results and snapshots.
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
) (
	[]*cppbridge.LSSQueryResult,
	[]*cppbridge.LabelSetSnapshot,
	error,
) {
	numberOfShards := head.NumberOfShards()
	selectors := make([]uintptr, numberOfShards)
	snapshots := make([]*cppbridge.LabelSetSnapshot, numberOfShards)
	convertedMatchers := convertPrometheusMatchersToPPMatchers(matchers...)

	tLSSQuerySelector := head.CreateTask(
		taskName,
		func(shard TShard) (err error) {
			shardID := shard.ShardID()
			selectors[shardID], snapshots[shardID], err = shard.LSS().QuerySelector(shardID, convertedMatchers)
			return err
		},
	)
	head.Enqueue(tLSSQuerySelector)
	if err := tLSSQuerySelector.Wait(); err != nil {
		return nil, nil, err
	}

	lssQueryResults := make([]*cppbridge.LSSQueryResult, numberOfShards)
	errs := make([]error, numberOfShards)
	for shardID, selector := range selectors {
		if selector == 0 {
			continue
		}

		lssQueryResult := snapshots[shardID].Query(selector)
		switch lssQueryResult.Status() {
		case cppbridge.LSSQueryStatusMatch:
			lssQueryResults[shardID] = lssQueryResult
		case cppbridge.LSSQueryStatusNoMatch:
		default:
			errs[shardID] = fmt.Errorf(
				"failed to query from shard: %d, query status: %d", shardID, lssQueryResult.Status(),
			)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, nil, err
	}

	return lssQueryResults, snapshots, nil
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
