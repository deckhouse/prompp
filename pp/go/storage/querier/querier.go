package querier

import (
	"context"
	"sort"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/logger"
	"github.com/prometheus/prometheus/util/annotations"
)

//
// Deduplicator
//

// Deduplicator accumulates and deduplicates incoming values.
type Deduplicator interface {
	Add(shard uint16, snapshot *cppbridge.LabelSetSnapshot, values []string)
	Values() []string
}

// deduplicatorCtor constructor [Deduplicator].
type deduplicatorCtor func(numberOfShards uint16) Deduplicator

//
// GenericTask
//

// GenericTask the minimum required GenericTask implementation.
type GenericTask interface {
	Wait() error
}

//
// Shard
//

// Shard the minimum required head Shard implementation.
type Shard interface {
	QueryLabelValues(
		name string,
		matchers []model.LabelMatcher,
		dedupAdd func(shardID uint16, snapshot *cppbridge.LabelSetSnapshot, values []string),
	) error
}

//
// Head
//

// Head the minimum required Head implementation.
type Head[
	TGenericTask GenericTask,
	TShard Shard,
] interface {
	CreateTask(taskName string, fn func(shard TShard) error, isLss bool) TGenericTask
	Enqueue(t TGenericTask)
	NumberOfShards() uint16
	RLockQuery(ctx context.Context) (runlock func(), err error)
}

//
// Querier
//

// Querier provides querying access over time series data of a fixed time range.
type Querier[
	TGenericTask GenericTask,
	TShard Shard,
	THead Head[TGenericTask, TShard],
] struct {
	mint             int64
	maxt             int64
	head             THead
	deduplicatorCtor deduplicatorCtor
	closer           func() error
	metrics          *Metrics
}

// Close Querier if need.
func (q *Querier[TGenericTask, TShard, THead]) Close() error {
	if q.closer != nil {
		return q.closer()
	}

	return nil
}

// LabelValues returns label values present in the head for the specific label name
// that are within the time range mint to maxt. If matchers are specified the returned
// result set is reduced to label values of metrics matching the matchers.
func (q *Querier[TGenericTask, TShard, THead]) LabelValues(
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
		storage.LSSLabelValuesQuerier,
		matchers...,
	)
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

// queryLabelValues returns label values present in the head for the specific label name.
func queryLabelValues[
	TGenericTask GenericTask,
	TShard Shard,
	THead Head[TGenericTask, TShard],
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
	runlock, err := head.RLockQuery(ctx)
	if err != nil {
		logger.Warnf("[QUERIER]: label values failed on the capture of the read lock query: %s", err)
		return nil, anns, err
	}
	defer runlock()

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
			return shard.QueryLabelValues(name, convertedMatchers, dedup.Add)
		},
		storage.ForLSSTask,
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
