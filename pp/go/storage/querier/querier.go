package querier

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
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
// Enabled by default; disable via PROMPP_FEATURES=disable_instant_query_feature.
var InstantQueryFeature = true

const (
	// lssQueryInstantQuerySelector name of task.
	lssQueryInstantQuerySelector = "lss_query_instant_query_selector"
	// lssQueryRangeQuerySelector name of task.
	lssQueryRangeQuerySelector = "lss_query_range_query_selector"
	// lssLabelValuesQuerier name of task.
	lssLabelValuesQuerier = "lss_label_values_querier"
	// lssLabelNamesQuerier name of task.
	lssLabelNamesQuerier = "lss_label_names_querier"

	// lssGroupSeriesByLabelNames name of task.
	lssGroupSeriesByLabelNames = "lss_group_series_by_label_names"

	// dsQueryInstantQuerier name of task.
	dsQueryInstantQuerier = "data_storage_query_instant_querier"
	// dsQueryRangeQuerier name of task.
	dsQueryRangeQuerier = "data_storage_query_range_querier"
	// dsQueryFirstTimestampsQuerier name of task.
	dsQueryFirstTimestampsQuerier = "data_storage_query_first_timestamps_querier"

	// DefaultInstantQueryValueNotFoundTimestampValue default value for not found timestamp value for instant query.
	DefaultInstantQueryValueNotFoundTimestampValue int64 = 0

	// DefaultNotFoundTimestampValue default value for not found timestamp value.
	DefaultNotFoundTimestampValue int64 = math.MinInt64
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

// DefaultCountOfSeriesToOptimize is the default count of series to optimize.
const DefaultCountOfSeriesToOptimize = 10

// optimizeAggregationRatio is the ratio to optimize the aggregation function.
const optimizeAggregationRatio = 3.3

// defaultOptimizeType is the default option for selecting functions optimization.
var defaultOptimizeType = noneOptimizeType

// SetSelectFuncOptimize replaces the optimization modes used by subsequently created [Querier] instances.
// Options are separated by | and may have surrounding whitespace. They are applied left to right:
// none clears the selection, all enables every mode, and other options add their mode to the selection.
// Invalid input leaves the previous selection unchanged.
func SetSelectFuncOptimize(opts string) error {
	optimization := noneOptimizeType

	for opt := range strings.SplitSeq(opts, "|") {
		opt = strings.TrimSpace(opt)

		switch opt {
		case "none":
			optimization = noneOptimizeType

		case "drop_point":
			optimization |= dropPointOptimizeType

		case "new_point":
			optimization |= newPointOptimizeType

		case "cross":
			optimization |= crossSeriesOptimizeType

		case "all":
			optimization = allOptimizeType

		default:
			return fmt.Errorf(
				"invalid select func optimization option: '%s', valid options are: "+
					"'none', 'drop_point', 'new_point', 'cross', 'all'", opt,
			)
		}
	}

	selectFuncOptimize = optimization

	return nil
}

// SetDefaultOptimizeType set the default option for selecting functions optimization.
func SetDefaultOptimizeType() {
	selectFuncOptimize = defaultOptimizeType
}

// selectFuncOptimize is the option for selecting functions optimization.
var selectFuncOptimize = defaultOptimizeType

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
	// IsPossibleToOptimize defaults to [DefaultIsPossibleToOptimize].
	// It is called only when the optimization mode and query shape permit optimization.
	// Set it before querying to override the optimization gate for this querier.
	IsPossibleToOptimize func(
		lssQueryResults []*cppbridge.LSSQueryResult,
		hints *storage.SelectHints,
		scrapeIntervalMS, headMinTSMS int64,
	) bool

	mint             int64
	maxt             int64
	scrapeIntervalMS int64
	headMinTSMS      int64
	head             THead
	deduplicatorCtor deduplicatorCtor
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
	mint, maxt, scrapeIntervalMS, headMinTSMS int64,
	metrics *Metrics,
) *Querier[TTask, TDataStorage, TLSS, TShard, THead] {
	return newQuerierWithSelectFuncOptimize(
		head,
		deduplicatorCtor,
		mint,
		maxt,
		scrapeIntervalMS,
		headMinTSMS,
		metrics,
		selectFuncOptimize,
	)
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
	mint, maxt, scrapeIntervalMS, headMinTSMS int64,
	metrics *Metrics,
) *Querier[TTask, TDataStorage, TLSS, TShard, THead] {
	return newQuerierWithSelectFuncOptimize(
		head,
		deduplicatorCtor,
		mint,
		maxt,
		scrapeIntervalMS,
		headMinTSMS,
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
	mint, maxt, scrapeIntervalMS, headMinTSMS int64,
	metrics *Metrics,
	queryOptimize queryOptimizeType,
) *Querier[TTask, TDataStorage, TLSS, TShard, THead] {
	return &Querier[TTask, TDataStorage, TLSS, TShard, THead]{
		IsPossibleToOptimize: DefaultIsPossibleToOptimize,
		mint:                 mint,
		maxt:                 maxt,
		scrapeIntervalMS:     scrapeIntervalMS,
		headMinTSMS:          headMinTSMS,
		head:                 head,
		deduplicatorCtor:     deduplicatorCtor,
		metrics:              metrics,
		queryOptimize:        queryOptimize,
	}
}

// Close [Querier] if need.
//
//revive:disable-next-line:confusing-naming // other type of querier.
func (*Querier[TTask, TDataStorage, TLSS, TShard, THead]) Close() error {
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
			return emptySeriesSet
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
	hints *storage.SelectHints,
	matchers ...*labels.Matcher,
) storage.SeriesSet {
	resources, err := beginSelectQuery(ctx, q.head, q.metrics, operationSelectRange)
	if err != nil {
		if errors.Is(err, locker.ErrSemaphoreClosed) {
			return emptySeriesSet
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

	hints = SwitchFuncOptimize(hints, q.queryOptimize)

	if hints != emptySelectHints &&
		!q.IsPossibleToOptimize(resources.lssQueryResults, hints, q.scrapeIntervalMS, q.headMinTSMS) {
		hints = emptySelectHints
	}

	resources.serializedData = poolProvider.GetSerializedData()
	t := queryDataStorage(
		dsQueryRangeQuerier, q.head, resources.lssQueryResults, resources.serializedData, q.mint, q.maxt, hints, stats,
	)
	resources.addTask(t)

	var result storage.SeriesSet
	optimizationType := "none"

	switch {
	case isCrossSeriesFunc(hints):
		optimizationType = "cross_series"
		result = q.makeCrossSeriesSet(
			resources.lssQueryResults, resources.snapshots, resources.serializedData, hints,
		)

	case isAggregationSeriesFunc(hints):
		optimizationType = "aggregation"
		resources.seriesSets = poolProvider.GetSeriesSet()
		result = q.makeAggrSeriesSet(
			resources.lssQueryResults, resources.snapshots, resources.serializedData, resources.seriesSets,
		)

	default:
		resources.seriesSets = poolProvider.GetSeriesSet()
		result = q.makeSeriesSet(
			resources.lssQueryResults, resources.snapshots, resources.serializedData, resources.seriesSets,
		)
	}

	if q.metrics != nil {
		q.metrics.OptimizationType.WithLabelValues(optimizationType).Inc()
	}

	stats.lap.Mark(stageBuildResult)

	return result
}

// makeAggrSeriesSet fills the caller-owned shard buffer and builds a merged set without retaining the buffer.
func (q *Querier[TTask, TDataStorage, TLSS, TShard, THead]) makeAggrSeriesSet(
	lssQueryResults []*cppbridge.LSSQueryResult,
	snapshots []*cppbridge.LabelSetSnapshot,
	shardedSerializedData []*cppbridge.DataStorageSerializedData,
	seriesSets []storage.SeriesSet,
) storage.SeriesSet {
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

// makeSeriesSet fills the caller-owned shard buffer and builds a merged set without retaining the buffer.
func (q *Querier[TTask, TDataStorage, TLSS, TShard, THead]) makeSeriesSet(
	lssQueryResults []*cppbridge.LSSQueryResult,
	snapshots []*cppbridge.LabelSetSnapshot,
	shardedSerializedData []*cppbridge.DataStorageSerializedData,
	seriesSets []storage.SeriesSet,
) storage.SeriesSet {
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

// makeCrossSeriesSet queries the cross series set.
func (q *Querier[TTask, TDataStorage, TLSS, TShard, THead]) makeCrossSeriesSet(
	lssQueryResults []*cppbridge.LSSQueryResult,
	snapshots []*cppbridge.LabelSetSnapshot,
	shardedSerializedData []*cppbridge.DataStorageSerializedData,
	hints *storage.SelectHints,
) storage.SeriesSet {
	staleNaNSeries := make([][]StaleNaNSeries, len(lssQueryResults))
	for i := range staleNaNSeries {
		if lssQueryResults[i] == nil {
			continue
		}

		staleNaNSeries[i] = NewStaleNaNSeriesSlice(lssQueryResults[i].Len(), DefaultNotFoundTimestampValue)
	}

	tds := q.head.CreateTask(
		dsQueryFirstTimestampsQuerier,
		func(shard TShard) error {
			shardID := shard.ShardID()
			res := lssQueryResults[shardID]
			if res == nil {
				return nil
			}

			shard.DataStorage().QueryStaleNaNSeries(
				res.IDs(),
				uintptr(unsafe.Pointer(unsafe.SliceData(staleNaNSeries[shardID]))), // #nosec G103 // it's safe to use
			)

			return nil
		},
	)
	q.head.Enqueue(tds)

	poolProvider := q.head.PoolProvider()
	seriesGroups := poolProvider.GetSeriesGroups()
	tlss := q.head.CreateTask(
		lssGroupSeriesByLabelNames,
		func(shard TShard) error {
			shardID := shard.ShardID()
			res := lssQueryResults[shardID]
			if res == nil {
				return nil
			}

			nameIDs := poolProvider.GetNameIDs(len(hints.Grouping))
			shard.LSS().LabelNameToIDs(hints.Grouping, nameIDs)
			seriesGroups[shardID] = shard.LSS().GroupSeriesByLabelNames(res.IDs(), nameIDs)
			poolProvider.PutNameIDs(nameIDs)

			return nil
		},
	)
	q.head.Enqueue(tlss)

	_ = tds.Wait()
	_ = tlss.Wait()
	q.head.PutTask(tds)
	q.head.PutTask(tlss)

	seriesSets := poolProvider.GetSeriesSet()
	sNaNSeriesSets := poolProvider.GetSeriesSet()
	for shardID, serializedData := range shardedSerializedData {
		if serializedData == nil {
			sNaNSeriesSets[shardID] = emptySeriesSet
			seriesSets[shardID] = emptySeriesSet
			continue
		}

		sNaNSeriesSets[shardID] = NewStaleNaNSeriesSet(
			staleNaNSeries[shardID],
			snapshots[shardID],
			DefaultNotFoundTimestampValue,
		)

		seriesSets[shardID] = NewCrossSeriesSet(
			serializedData,
			snapshots[shardID],
			seriesGroups[shardID],
			q.mint,
			q.maxt,
			hints.Grouping,
			q.head.ID(),
			uint16(shardID), // #nosec G115 // no overflow
		)
	}

	resultSeriesSets := NewMergeManyShardSeriesSets(seriesSets, sNaNSeriesSets)

	poolProvider.PutSeriesSet(sNaNSeriesSets)
	poolProvider.PutSeriesSet(seriesSets)
	poolProvider.PutSeriesGroups(seriesGroups)

	return resultSeriesSets
}

// SwitchFuncOptimize returns hints when the optimization mode and query shape permit optimization.
// The per-querier cost gate is evaluated separately by [Querier.Select].
func SwitchFuncOptimize(
	hints *storage.SelectHints,
	queryOptimize queryOptimizeType,
) *storage.SelectHints {
	if hints == nil {
		return emptySelectHints
	}

	if hints.IsSubquery {
		return emptySelectHints
	}

	if funcOptimizeMap[hints.Func]&queryOptimize != 0 &&
		isNotWithout(hints) &&
		isAllowedGroupingForCrossSeriesFunc(hints.Grouping) {
		return hints
	}

	return emptySelectHints
}

// DefaultIsPossibleToOptimize reports whether the series count and sampling bounds justify optimization.
// A non-positive scrape interval disables optimization.
func DefaultIsPossibleToOptimize(
	lssQueryResults []*cppbridge.LSSQueryResult,
	hints *storage.SelectHints,
	scrapeIntervalMS, headMinTSMS int64,
) bool {
	if scrapeIntervalMS <= 0 {
		return false
	}

	if isCrossSeriesFunc(hints) {
		countOfSeries := 0
		for _, lssQueryResult := range lssQueryResults {
			if lssQueryResult == nil {
				continue
			}

			countOfSeries += lssQueryResult.Len()
		}

		return isPossibleToOptimizeCrossSeriesFunc(hints, scrapeIntervalMS, countOfSeries)
	}

	if isAggregationSeriesFunc(hints) {
		return isPossibleToOptimizeAggregationFunc(hints, scrapeIntervalMS, headMinTSMS)
	}

	return false
}

// isPossibleToOptimizeCrossSeriesFunc checks if the cross series function is possible to optimize.
func isPossibleToOptimizeCrossSeriesFunc(
	hints *storage.SelectHints,
	scrapeIntervalMS int64,
	countOfSeries int,
) bool {
	// for cross series functions for instant query, we don't need to optimize the query
	if hints.Step == 0 {
		return false
	}

	if countOfSeries < DefaultCountOfSeriesToOptimize {
		return false
	}

	if len(hints.Grouping) == 0 {
		return true
	}

	// grouping by
	//revive:disable-next-line:add-constant // +2 is required to enable optimization
	if hints.Step == scrapeIntervalMS && countOfSeries > DefaultCountOfSeriesToOptimize+3 {
		return true
	}

	//revive:disable-next-line:add-constant // x2 scrape interval are required to enable optimization
	if hints.Step >= scrapeIntervalMS*2 && countOfSeries > DefaultCountOfSeriesToOptimize+2 {
		return true
	}

	return false
}

// isPossibleToOptimizeAggregationFunc checks if the aggregation function is possible to optimize.
func isPossibleToOptimizeAggregationFunc(
	hints *storage.SelectHints,
	scrapeIntervalMS, headMinTSMS int64,
) bool {
	if hints.Step == 0 {
		// instant query, shift is x5 scrape interval and range is x5 scrape interval
		//revive:disable-next-line:add-constant // x5 scrape interval
		return hints.Start-headMinTSMS >= scrapeIntervalMS*5 && hints.Range >= scrapeIntervalMS*5
	}

	if hints.Step <= scrapeIntervalMS {
		return false
	}

	if hints.Range < scrapeIntervalMS {
		return false
	}

	if hints.Step > hints.Range {
		return true
	}

	if float64(hints.Step)/float64(scrapeIntervalMS) >= optimizeAggregationRatio {
		return true
	}

	return false
}

// isNotWithout checks if the hints is not without by.
func isNotWithout(hints *storage.SelectHints) bool {
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

// isAllowedGroupingForCrossSeriesFunc checks if the series set is an cross series set.
func isAllowedGroupingForCrossSeriesFunc(grouping []string) bool {
	for _, group := range grouping {
		if group == labelHeadIDShardID {
			logger.Infof(
				"[QUERIER]: __head__shard_id is in the grouping, it will be ignored: %s",
				group,
			)
			return false
		}
	}

	return true
}

// isCrossSeriesFunc checks if the function is a cross series function.
func isCrossSeriesFunc(hints *storage.SelectHints) bool {
	return funcOptimizeMap[hints.Func]&crossSeriesOptimizeType == crossSeriesOptimizeType
}

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
			result := s.DataStorage().Query(
				cppbridge.DataStorageQuery{
					StartTimestampMs: mint,
					EndTimestampMs:   maxt,
					LabelSetIDs:      lssQueryResult.IDs(),
				},
				cppbridge.NoDownsampling,
				hints,
			)
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
