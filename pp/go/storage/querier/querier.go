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
// queryOptimizeType
//

// queryOptimizeType is the type for query optimization.
type queryOptimizeType uint8

const (
	// dropPointOptimizeType is the option for drop point functions optimization.
	dropPointOptimizeType queryOptimizeType = 1 << iota

	// newPointOptimizeType is the option for new point functions optimization.
	// Optimization creates a new point at the end of the window or step.
	newPointOptimizeType

	// crossSeriesOptimizeType is the option for cross-series functions optimization.
	// A new series is created.
	crossSeriesOptimizeType
)

const (
	// noneOptimizeType is the option without any optimization.
	noneOptimizeType queryOptimizeType = 0

	// allOptimizeType is the option for all functions optimization.
	allOptimizeType queryOptimizeType = dropPointOptimizeType | newPointOptimizeType | crossSeriesOptimizeType
)

// SetSelectFuncOptimize sets the select func optimization option by name.
func SetSelectFuncOptimize(opt string) error {
	switch opt {
	case "none":
		selectFuncOptimize = noneOptimizeType
		return nil

	case "drop_point":
		selectFuncOptimize = dropPointOptimizeType
		return nil

	case "new_point":
		selectFuncOptimize = newPointOptimizeType
		return nil

	case "cross":
		selectFuncOptimize = crossSeriesOptimizeType
		return nil

	case "all":
		selectFuncOptimize = allOptimizeType
		return nil

	default:
		return fmt.Errorf(
			"invalid select func optimization option: '%s', valid options are: "+
				"'none', 'drop_point', 'new_point', 'cross', 'all'", opt,
		)
	}
}

// selectFuncOptimize is the option for selecting functions optimization.
var selectFuncOptimize = noneOptimizeType

// emptySelectHints is an empty select hints, it's used when no optimization is needed.
var emptySelectHints = &storage.SelectHints{}

// emptySeriesSet is an empty series set.
var emptySeriesSet = &SeriesSet{}

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
	queryOptimize    queryOptimizeType
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
	return newQuerierWithSelectFuncOptimize(head, deduplicatorCtor, mint, maxt, closer, metrics, selectFuncOptimize)
}

// NewQuerierWithOutSelectFuncOptimize init new [Querier] without select func optimization.
func NewQuerierWithOutSelectFuncOptimize[
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
	return newQuerierWithSelectFuncOptimize(
		head,
		deduplicatorCtor,
		mint,
		maxt,
		closer,
		metrics,
		selectFuncOptimize&dropPointOptimizeType,
	)
}

// newQuerierWithSelectFuncOptimize init new [Querier] with select func optimization.
func newQuerierWithSelectFuncOptimize[
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
	queryOptimize queryOptimizeType,
) *Querier[TTask, TDataStorage, TLSS, TShard, THead] {
	return &Querier[TTask, TDataStorage, TLSS, TShard, THead]{
		mint:             mint,
		maxt:             maxt,
		head:             head,
		deduplicatorCtor: deduplicatorCtor,
		closer:           closer,
		metrics:          metrics,
		queryOptimize:    queryOptimize,
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

	poolProvider := q.head.PoolProvider()
	snapshots := poolProvider.GetSnapshots()
	defer poolProvider.PutSnapshots(snapshots)
	lssQueryResults := poolProvider.GetLSSQueryResults()
	defer poolProvider.PutLSSQueryResults(lssQueryResults)

	if err = queryLss(lssQueryInstantQuerySelector, q.head, matchers, snapshots, lssQueryResults); err != nil {
		logger.Warnf("[QUERIER]: failed to instant: %s", err)
		return storage.ErrSeriesSet(err)
	}

	valueNotFoundTimestampValue := DefaultInstantQueryValueNotFoundTimestampValue
	if q.mint <= valueNotFoundTimestampValue {
		valueNotFoundTimestampValue = q.mint - 1
	}

	seriesSets := poolProvider.GetSeriesSet()
	defer poolProvider.PutSeriesSet(seriesSets)
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

			result := s.DataStorage().InstantQuery(
				q.maxt,
				lssQueryResult.IDs(),
				uintptr(unsafe.Pointer(unsafe.SliceData(instantSeries))), // #nosec G103 // it's meant to be that way
			)
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
	defer q.head.PutTask(tDataStorageQuery)
	q.head.Enqueue(tDataStorageQuery)
	_ = tDataStorageQuery.Wait()

	if err = loadAndQueryWaiter.Wait(); err != nil {
		SendUnrecoverableError(err)
		return storage.ErrSeriesSet(err)
	}

	return NewMergeShardSeriesSet(seriesSets)
}

// selectRange returns a range set of series that matches the given label matchers.
func (q *Querier[TTask, TDataStorage, TLSS, TShard, THead]) selectRange(
	ctx context.Context,
	_ bool,
	hints *storage.SelectHints,
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

	poolProvider := q.head.PoolProvider()
	snapshots := poolProvider.GetSnapshots()
	defer poolProvider.PutSnapshots(snapshots)
	lssQueryResults := poolProvider.GetLSSQueryResults()
	defer poolProvider.PutLSSQueryResults(lssQueryResults)

	if err = queryLss(lssQueryRangeQuerySelector, q.head, matchers, snapshots, lssQueryResults); err != nil {
		logger.Warnf("[QUERIER]: failed to range: %s", err)
		return storage.ErrSeriesSet(err)
	}

	hints = SwitchFuncOptimize(hints, q.queryOptimize)
	shardedSerializedData := poolProvider.GetSerializedData()
	defer poolProvider.PutSerializedData(shardedSerializedData)
	queryDataStorage(dsQueryRangeQuerier, q.head, lssQueryResults, shardedSerializedData, q.mint, q.maxt, hints)

	if isAggregationSeriesFunc(hints) {
		return q.makeAggrSeriesSet(lssQueryResults, snapshots, shardedSerializedData)
	}

	return q.makeSeriesSet(lssQueryResults, snapshots, shardedSerializedData)
}

// makeAggrSeriesSet makes the aggregated series set.
func (q *Querier[TTask, TDataStorage, TLSS, TShard, THead]) makeAggrSeriesSet(
	lssQueryResults []*cppbridge.LSSQueryResult,
	snapshots []*cppbridge.LabelSetSnapshot,
	shardedSerializedData []*cppbridge.DataStorageSerializedData,
) storage.SeriesSet {
	poolProvider := q.head.PoolProvider()

	seriesSets := poolProvider.GetSeriesSet()
	defer poolProvider.PutSeriesSet(seriesSets)
	for shardID, serializedData := range shardedSerializedData {
		if serializedData != nil {
			seriesSets[shardID] = NewAggrSeriesSet(
				snapshots[shardID],
				serializedData,
				lssQueryResults[shardID],
				q.mint,
				q.maxt,
			)
			continue
		}

		seriesSets[shardID] = emptySeriesSet
	}

	return NewMergeShardSeriesSet(seriesSets)
}

// makeSeriesSet makes the series set.
func (q *Querier[TTask, TDataStorage, TLSS, TShard, THead]) makeSeriesSet(
	lssQueryResults []*cppbridge.LSSQueryResult,
	snapshots []*cppbridge.LabelSetSnapshot,
	shardedSerializedData []*cppbridge.DataStorageSerializedData,
) storage.SeriesSet {
	poolProvider := q.head.PoolProvider()

	seriesSets := poolProvider.GetSeriesSet()
	defer poolProvider.PutSeriesSet(seriesSets)
	for shardID, serializedData := range shardedSerializedData {
		if serializedData != nil {
			seriesSets[shardID] = NewSeriesSet(
				q.mint,
				q.maxt,
				lssQueryResults[shardID],
				snapshots[shardID],
				serializedData,
			)
			continue
		}

		seriesSets[shardID] = emptySeriesSet
	}

	return NewMergeShardSeriesSet(seriesSets)
}

// SwitchFuncOptimize switch the function optimization hints.
func SwitchFuncOptimize(hints *storage.SelectHints, queryOptimize queryOptimizeType) *storage.SelectHints {
	if hints == nil {
		return emptySelectHints
	}

	if hints.IsSubquery {
		return emptySelectHints
	}

	if funcOptimizeMap[hints.Func]&queryOptimize != 0 && isNotWithpout(hints) {
		return hints
	}

	return emptySelectHints
}

// isNotWithpout checks if the hints is not without by.
func isNotWithpout(hints *storage.SelectHints) bool {
	return hints.By || len(hints.Grouping) == 0
}

// funcOptimizeMap is the map of the function to the query optimization type.
var funcOptimizeMap = func() map[string]queryOptimizeType {
	optimizeType := func(Type cppbridge.PromqlCppFunctionType) queryOptimizeType {
		switch Type {
		case cppbridge.PromqlCppThinningFunction:
			return dropPointOptimizeType
		case cppbridge.PromqlCppSynthesizingFunction:
			return newPointOptimizeType
		case cppbridge.PromqlCppCrossSeriesSynthesizingFunction:
			return crossSeriesOptimizeType

		default:
			return noneOptimizeType
		}
	}

	cppFunctions := cppbridge.GetPromqlCppFunctions()
	functions := make(map[string]queryOptimizeType, len(cppFunctions))
	for _, function := range cppFunctions {
		if oType := optimizeType(function.Type); oType != noneOptimizeType {
			functions[function.Name] = oType
		}
	}

	return functions
}()

// isAggregationSeriesFunc checks if the function is an aggregation series function.
func isAggregationSeriesFunc(hints *storage.SelectHints) bool {
	return funcOptimizeMap[hints.Func]&dropPointOptimizeType == dropPointOptimizeType
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
	hints *storage.SelectHints,
) {
	loadAndQueryWaiter := NewLoadAndQueryWaiter[TTask, TDataStorage, TLSS, TShard, THead](head)
	tDataStorageQuery := head.CreateTask(
		taskName,
		func(s TShard) error {
			shardID := s.ShardID()
			lssQueryResult := lssQueryResults[shardID]
			if lssQueryResult == nil {
				return nil
			}

			result := s.DataStorage().Query(
				cppbridge.DataStorageQuery{
					StartTimestampMs: mint,
					EndTimestampMs:   maxt,
					LabelSetIDs:      lssQueryResult.IDs(),
				},
				cppbridge.NoDownsampling,
				hints,
			)
			if result.Status == cppbridge.DataStorageQueryStatusNeedDataLoad {
				loadAndQueryWaiter.Add(s, result.Querier)
			}
			shardedSerializedData[s.ShardID()] = result.SerializedData

			return nil
		},
	)
	defer head.PutTask(tDataStorageQuery)
	head.Enqueue(tDataStorageQuery)
	_ = tDataStorageQuery.Wait()

	if err := loadAndQueryWaiter.Wait(); err != nil {
		clear(shardedSerializedData)
		SendUnrecoverableError(err)
	}
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
	defer head.PutTask(t)
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
	defer head.PutTask(t)
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

// queryLss returns query results and snapshots.
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
	snapshots []*cppbridge.LabelSetSnapshot,
	lssQueryResults []*cppbridge.LSSQueryResult,
) error {
	poolProvider := head.PoolProvider()
	selectors := poolProvider.GetSelectors()
	defer poolProvider.PutSelectors(selectors)
	convertedMatchers := convertPrometheusMatchersToPPMatchers(matchers...)

	tLSSQuerySelector := head.CreateTask(
		taskName,
		func(shard TShard) (err error) {
			shardID := shard.ShardID()
			selectors[shardID], snapshots[shardID], err = shard.LSS().QuerySelector(shardID, convertedMatchers)
			return err
		},
	)
	defer head.PutTask(tLSSQuerySelector)
	head.Enqueue(tLSSQuerySelector)
	if err := tLSSQuerySelector.Wait(); err != nil {
		return err
	}

	errs := poolProvider.GetErrors()
	defer poolProvider.PutErrors(errs)
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

	return errors.Join(errs...)
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
