package querier

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

type Deduplicator interface {
	Add(shard uint16, values ...string)
	Values() []string
}

type DeduplicatorFactory interface {
	Deduplicator(numberOfShards uint16) Deduplicator
}

type Querier struct {
	mint                int64
	maxt                int64
	head                relabeler.Head
	deduplicatorFactory DeduplicatorFactory
	closer              func() error
	metrics             *Metrics
}

func NewQuerier(
	head relabeler.Head,
	deduplicatorFactory DeduplicatorFactory,
	mint, maxt int64,
	closer func() error,
	metrics *Metrics,
) *Querier {
	return &Querier{
		mint:                mint,
		maxt:                maxt,
		head:                head,
		deduplicatorFactory: deduplicatorFactory,
		closer:              closer,
		metrics:             metrics,
	}
}

func (q *Querier) LabelValues(
	ctx context.Context,
	name string,
	hints *storage.LabelHints,
	matchers ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return labelValues(
		ctx,
		name,
		q.head,
		q.deduplicatorFactory,
		q.metrics,
		relabeler.LSSLabelValuesQuerier,
		hints,
		matchers...,
	)
}

func labelValues(
	ctx context.Context,
	name string,
	head relabeler.Head,
	deduplicatorFactory DeduplicatorFactory,
	metrics *Metrics,
	taskName string,
	hints *storage.LabelHints,
	matchers ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	start := time.Now()
	defer func() {
		if metrics != nil {
			metrics.LabelValuesDuration.With(
				prometheus.Labels{"generation": fmt.Sprintf("%d", head.Generation())},
			).Observe(float64(time.Since(start).Microseconds()))
		}
	}()

	dedup := deduplicatorFactory.Deduplicator(head.NumberOfShards())
	convertedMatchers := convertPrometheusMatchersToOpcoreMatchers(matchers...)

	t := head.CreateTask(
		taskName,
		func(shard relabeler.Shard) error {
			queryLabelValuesResult := shard.LSS().QueryLabelValues(name, convertedMatchers)
			if queryLabelValuesResult.Status() != cppbridge.LSSQueryStatusMatch {
				return fmt.Errorf("no matches on shard: %d", shard.ShardID())
			}

			dedup.Add(shard.ShardID(), queryLabelValuesResult.Values()...)
			runtime.KeepAlive(queryLabelValuesResult)
			return nil
		},
		relabeler.ForLSSTask,
		relabeler.NonExclusiveTask,
	)
	head.Enqueue(t)

	anns := *annotations.New()
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

	if hints.Limit > 0 && hints.Limit < len(lvs) {
		return lvs[:hints.Limit], anns, nil
	}
	return lvs, anns, nil
}

func (q *Querier) LabelNames(
	ctx context.Context,
	hints *storage.LabelHints,
	matchers ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return labelNames(ctx, q.head, q.deduplicatorFactory, q.metrics, relabeler.LSSLabelNamesQuerier, hints, matchers...)
}

func labelNames(
	ctx context.Context,
	head relabeler.Head,
	deduplicatorFactory DeduplicatorFactory,
	metrics *Metrics,
	taskName string,
	hints *storage.LabelHints,
	matchers ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	start := time.Now()
	defer func() {
		if metrics != nil {
			metrics.LabelNamesDuration.With(
				prometheus.Labels{"generation": fmt.Sprintf("%d", head.Generation())},
			).Observe(float64(time.Since(start).Microseconds()))
		}
	}()

	dedup := deduplicatorFactory.Deduplicator(head.NumberOfShards())
	convertedMatchers := convertPrometheusMatchersToOpcoreMatchers(matchers...)

	t := head.CreateTask(
		taskName,
		func(shard relabeler.Shard) error {
			queryLabelNamesResult := shard.LSS().QueryLabelNames(convertedMatchers)
			if queryLabelNamesResult.Status() != cppbridge.LSSQueryStatusMatch {
				return fmt.Errorf("no matches on shard: %d", shard.ShardID())
			}

			dedup.Add(shard.ShardID(), queryLabelNamesResult.Names()...)
			runtime.KeepAlive(queryLabelNamesResult)
			return nil
		},
		relabeler.ForLSSTask,
		relabeler.NonExclusiveTask,
	)
	head.Enqueue(t)

	anns := *annotations.New()
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

// Close Querier if need.
func (q *Querier) Close() error {
	if q.closer != nil {
		return q.closer()
	}

	return nil
}

func (q *Querier) Select(
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

func (q *Querier) selectInstant(
	ctx context.Context,
	_ bool,
	_ *storage.SelectHints,
	matchers ...*labels.Matcher,
) storage.SeriesSet {
	start := time.Now()
	defer func() {
		if q.metrics != nil {
			q.metrics.SelectDuration.With(
				prometheus.Labels{
					"generation": strconv.FormatUint(q.head.Generation(), 10),
					"query_type": "instant",
				},
			).Observe(float64(time.Since(start).Microseconds()))
		}
	}()

	lssQueryResults := make([]*cppbridge.LSSQueryResult, q.head.NumberOfShards())
	snapshots := make([]*cppbridge.LabelSetSnapshot, q.head.NumberOfShards())

	convertedMatchers := convertPrometheusMatchersToOpcoreMatchers(matchers...)
	callerID := cppbridge.GetCaller(ctx)

	valueNotFoundTimestampValue := DefaultInstantQueryValueNotFoundTimestampValue
	if q.mint <= valueNotFoundTimestampValue {
		valueNotFoundTimestampValue = q.mint - 1
	}

	tLSSQuery := q.head.CreateTask(
		relabeler.LSSQueryInstantQuerier,
		func(shard relabeler.Shard) error {
			lssQueryResult := shard.LSS().Query(convertedMatchers, callerID)

			if lssQueryResult.Status() != cppbridge.LSSQueryStatusMatch {
				if lssQueryResult.Status() == cppbridge.LSSQueryStatusNoMatch {
					return nil
				}
				return fmt.Errorf(
					"failed to query from shard: %d, query status: %d",
					shard.ShardID(),
					lssQueryResult.Status(),
				)
			}

			lssQueryResults[shard.ShardID()] = lssQueryResult
			snapshots[shard.ShardID()] = shard.LSS().GetSnapshot()

			return nil
		},
		relabeler.ForLSSTask,
		relabeler.NonExclusiveTask,
	)
	q.head.Enqueue(tLSSQuery)
	if err := tLSSQuery.Wait(); err != nil {
		logger.Warnf("QUERIER: Select failed: %s", err)
		return storage.ErrSeriesSet(err)
	}

	seriesSets := make([]storage.SeriesSet, q.head.NumberOfShards())
	tDataStorageQuery := q.head.CreateTask(
		relabeler.DSQueryInstantQuerier,
		func(shard relabeler.Shard) error {
			lssQueryResult := lssQueryResults[shard.ShardID()]
			if lssQueryResult == nil {
				seriesSets[shard.ShardID()] = &SeriesSet{}
				return nil
			}

			seriesSets[shard.ShardID()] = NewInstantSeriesSet(
				lssQueryResult,
				snapshots[shard.ShardID()],
				valueNotFoundTimestampValue,
				shard.DataStorage().InstantQuery(q.maxt, valueNotFoundTimestampValue, lssQueryResult.IDs()),
			)

			return nil
		},
		relabeler.ForDataStorageTask,
		relabeler.NonExclusiveTask,
	)
	q.head.Enqueue(tDataStorageQuery)
	_ = tDataStorageQuery.Wait()

	return storage.NewMergeSeriesSet(seriesSets, storage.ChainedSeriesMerge)
}

func (q *Querier) selectRange(
	ctx context.Context,
	_ bool,
	_ *storage.SelectHints,
	matchers ...*labels.Matcher,
) storage.SeriesSet {
	start := time.Now()
	defer func() {
		if q.metrics != nil {
			q.metrics.SelectDuration.With(
				prometheus.Labels{
					"generation": strconv.FormatUint(q.head.Generation(), 10),
					"query_type": "range",
				},
			).Observe(float64(time.Since(start).Microseconds()))
		}
	}()

	lssQueryResults := make([]*cppbridge.LSSQueryResult, q.head.NumberOfShards())
	snapshots := make([]*cppbridge.LabelSetSnapshot, q.head.NumberOfShards())

	convertedMatchers := convertPrometheusMatchersToOpcoreMatchers(matchers...)
	callerID := cppbridge.GetCaller(ctx)

	tLSSQuery := q.head.CreateTask(
		relabeler.LSSQueryRangeQuerier,
		func(shard relabeler.Shard) error {
			lssQueryResult := shard.LSS().Query(convertedMatchers, callerID)

			if lssQueryResult.Status() != cppbridge.LSSQueryStatusMatch {
				if lssQueryResult.Status() == cppbridge.LSSQueryStatusNoMatch {
					return nil
				}
				return fmt.Errorf(
					"failed to query from shard: %d, query status: %d",
					shard.ShardID(),
					lssQueryResult.Status(),
				)
			}

			lssQueryResults[shard.ShardID()] = lssQueryResult
			snapshots[shard.ShardID()] = shard.LSS().GetSnapshot()

			return nil
		},
		relabeler.ForLSSTask,
		relabeler.NonExclusiveTask,
	)
	q.head.Enqueue(tLSSQuery)
	if err := tLSSQuery.Wait(); err != nil {
		logger.Warnf("QUERIER: Select failed: %s", err)
		return storage.ErrSeriesSet(err)
	}

	serializedChunksShards := make([]*cppbridge.HeadDataStorageSerializedChunks, q.head.NumberOfShards())
	tDataStorageQuery := q.head.CreateTask(
		relabeler.DSQueryRangeQuerier,
		func(shard relabeler.Shard) error {
			lssQueryResult := lssQueryResults[shard.ShardID()]
			if lssQueryResult == nil {
				return nil
			}

			serializedChunks := shard.DataStorage().Query(cppbridge.HeadDataStorageQuery{
				StartTimestampMs: q.mint,
				EndTimestampMs:   q.maxt,
				LabelSetIDs:      lssQueryResult.IDs(),
			})

			if serializedChunks.NumberOfChunks() == 0 {
				return nil
			}

			serializedChunksShards[shard.ShardID()] = serializedChunks

			return nil
		},
		relabeler.ForDataStorageTask,
		relabeler.NonExclusiveTask,
	)
	q.head.Enqueue(tDataStorageQuery)
	_ = tDataStorageQuery.Wait()

	seriesSets := make([]storage.SeriesSet, q.head.NumberOfShards())
	for shardID := range serializedChunksShards {
		if serializedChunksShards[shardID] == nil {
			seriesSets[shardID] = &SeriesSet{}
			continue
		}

		seriesSets[shardID] = &SeriesSet{
			mint:             q.mint,
			maxt:             q.maxt,
			deserializer:     cppbridge.NewHeadDataStorageDeserializer(serializedChunksShards[shardID]),
			chunksIndex:      serializedChunksShards[shardID].MakeIndex(),
			serializedChunks: serializedChunksShards[shardID],
			lssQueryResult:   lssQueryResults[shardID],
			labelSetSnapshot: snapshots[shardID],
		}
	}

	return storage.NewMergeSeriesSet(seriesSets, storage.ChainedSeriesMerge)
}

func convertPrometheusMatchersToOpcoreMatchers(matchers ...*labels.Matcher) []model.LabelMatcher {
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
