package querier

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/storage/logger"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

const (
	// LSSQueryInstantQuerySelector name of task.
	LSSQueryInstantQuerySelector = "lss_query_instant_query_selector"
	// LSSQueryRangeQuerySelector name of task.
	LSSQueryRangeQuerySelector = "lss_query_range_query_selector"
	// LSSLabelValuesQuerier name of task.
	LSSLabelValuesQuerier = "lss_label_values_querier"
	// LSSLabelNamesQuerier name of task.
	LSSLabelNamesQuerier = "lss_label_names_querier"

	// DSQueryInstantQuerier name of task.
	DSQueryInstantQuerier = "data_storage_query_instant_querier"
	// DSQueryRangeQuerier name of task.
	DSQueryRangeQuerier = "data_storage_query_range_querier"

	// DefaultInstantQueryValueNotFoundTimestampValue default value for not found timestamp value.
	DefaultInstantQueryValueNotFoundTimestampValue int64 = 0
)

//
// Querier
//

// Querier provides querying access over time series data of a fixed time range.
type Querier[
	TGenericTask GenericTask,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
	THead Head[TGenericTask, TDataStorage, TLSS, TShard],
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
	TGenericTask GenericTask,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
	THead Head[TGenericTask, TDataStorage, TLSS, TShard],
](
	head THead,
	deduplicatorCtor deduplicatorCtor,
	mint, maxt int64,
	closer func() error,
	metrics *Metrics,
) *Querier[TGenericTask, TDataStorage, TLSS, TShard, THead] {
	return &Querier[TGenericTask, TDataStorage, TLSS, TShard, THead]{
		mint:             mint,
		maxt:             maxt,
		head:             head,
		deduplicatorCtor: deduplicatorCtor,
		closer:           closer,
		metrics:          metrics,
	}
}

// Close [Querier] if need.
func (q *Querier[TGenericTask, TDataStorage, TLSS, TShard, THead]) Close() error {
	if q.closer != nil {
		return q.closer()
	}

	return nil
}

// LabelNames returns label values present in the head for the specific label name.
func (q *Querier[TGenericTask, TDataStorage, TLSS, TShard, THead]) LabelNames(
	ctx context.Context,
	hints *storage.LabelHints,
	matchers ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return queryLabelNames(
		ctx,
		q.head,
		q.deduplicatorCtor,
		q.metrics,
		LSSLabelNamesQuerier,
		hints,
		matchers...,
	)
}

// LabelValues returns label values present in the head for the specific label name
// that are within the time range mint to maxt. If matchers are specified the returned
// result set is reduced to label values of metrics matching the matchers.
func (q *Querier[TGenericTask, TDataStorage, TLSS, TShard, THead]) LabelValues(
	ctx context.Context,
	name string,
	matchers ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return queryLabelValues(
		ctx,
		name,
		q.head,
		q.deduplicatorCtor,
		q.metrics,
		LSSLabelValuesQuerier,
		matchers...,
	)
}

// Select returns a set of series that matches the given label matchers.
func (q *Querier[TGenericTask, TDataStorage, TLSS, TShard, THead]) Select(
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
func (q *Querier[TGenericTask, TDataStorage, TLSS, TShard, THead]) selectInstant(
	ctx context.Context,
	_ bool,
	_ *storage.SelectHints,
	matchers ...*labels.Matcher,
) storage.SeriesSet {
	start := time.Now()

	release, err := q.head.AcquireQuery(ctx)
	if err != nil {
		logger.Warnf("[QUERIER]: select instant failed on the capture of the read lock query: %s", err)
		return storage.ErrSeriesSet(err)
	}
	defer release()

	defer func() {
		if q.metrics == nil {
			q.metrics.SelectDuration.With(
				prometheus.Labels{"query_type": "instant"},
			).Observe(float64(time.Since(start).Microseconds()))
		}
	}()

	lssQueryResults, snapshots, err := queryLss(LSSQueryInstantQuerySelector, q.head, matchers)
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
	tDataStorageQuery := q.head.CreateTask(
		DSQueryInstantQuerier,
		func(shard TShard) error {
			shardID := shard.ShardID()
			lssQueryResult := lssQueryResults[shardID]
			if lssQueryResult == nil {
				seriesSets[shardID] = &SeriesSet{}
				return nil
			}

			seriesSets[shardID] = NewInstantSeriesSet(
				lssQueryResult,
				snapshots[shardID],
				valueNotFoundTimestampValue,
				shard.DataStorage().InstantQuery(q.maxt, valueNotFoundTimestampValue, lssQueryResult.IDs()),
			)

			return nil
		},
	)
	q.head.Enqueue(tDataStorageQuery)
	_ = tDataStorageQuery.Wait()

	return storage.NewMergeSeriesSet(seriesSets, storage.ChainedSeriesMerge)
}

// selectRange returns a range set of series that matches the given label matchers.
func (q *Querier[TGenericTask, TDataStorage, TLSS, TShard, THead]) selectRange(
	ctx context.Context,
	_ bool,
	_ *storage.SelectHints,
	matchers ...*labels.Matcher,
) storage.SeriesSet {
	start := time.Now()

	release, err := q.head.AcquireQuery(ctx)
	if err != nil {
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

	lssQueryResults, snapshots, err := queryLss(LSSQueryRangeQuerySelector, q.head, matchers)
	if err != nil {
		logger.Warnf("[QUERIER]: failed to range: %s", err)
		return storage.ErrSeriesSet(err)
	}

	serializedChunksShards := queryDataStorage(DSQueryRangeQuerier, q.head, lssQueryResults, q.mint, q.maxt)
	seriesSets := make([]storage.SeriesSet, q.head.NumberOfShards())
	for shardID, serializedChunksShard := range serializedChunksShards {
		if serializedChunksShard == nil {
			seriesSets[shardID] = &SeriesSet{}
			continue
		}

		seriesSets[shardID] = &SeriesSet{
			mint:             q.mint,
			maxt:             q.maxt,
			deserializer:     cppbridge.NewHeadDataStorageDeserializer(serializedChunksShard),
			chunksIndex:      serializedChunksShard.MakeIndex(),
			serializedChunks: serializedChunksShard,
			lssQueryResult:   lssQueryResults[shardID],
			labelSetSnapshot: snapshots[shardID],
		}
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

// queryDataStorage returns serialized chunks from data storage for each shard.
func queryDataStorage[
	TGenericTask GenericTask,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
	THead Head[TGenericTask, TDataStorage, TLSS, TShard],
](
	taskName string,
	head THead,
	lssQueryResults []*cppbridge.LSSQueryResult,
	mint, maxt int64,
) []*cppbridge.HeadDataStorageSerializedChunks {
	serializedChunksShards := make([]*cppbridge.HeadDataStorageSerializedChunks, head.NumberOfShards())
	tDataStorageQuery := head.CreateTask(
		taskName,
		func(shard TShard) error {
			shardID := shard.ShardID()
			lssQueryResult := lssQueryResults[shardID]
			if lssQueryResult == nil {
				return nil
			}

			serializedChunks := shard.DataStorage().Query(cppbridge.HeadDataStorageQuery{
				StartTimestampMs: mint,
				EndTimestampMs:   maxt,
				LabelSetIDs:      lssQueryResult.IDs(),
			})

			if serializedChunks.NumberOfChunks() == 0 {
				return nil
			}

			serializedChunksShards[shardID] = serializedChunks

			return nil
		},
	)
	head.Enqueue(tDataStorageQuery)
	_ = tDataStorageQuery.Wait()

	return serializedChunksShards
}

// queryLabelValues returns label values present in the head for the specific label name.
func queryLabelNames[
	TGenericTask GenericTask,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
	THead Head[TGenericTask, TDataStorage, TLSS, TShard],
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
	TGenericTask GenericTask,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
	THead Head[TGenericTask, TDataStorage, TLSS, TShard],
](
	ctx context.Context,
	name string,
	head THead,
	deduplicatorCtor deduplicatorCtor,
	metrics *Metrics,
	taskName string,
	matchers ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	start := time.Now()

	anns := *annotations.New()
	release, err := head.AcquireQuery(ctx)
	if err != nil {
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
	TGenericTask GenericTask,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
	THead Head[TGenericTask, TDataStorage, TLSS, TShard],
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
