package querier_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/pp/go/util/locker"
	"github.com/prometheus/prometheus/storage"
)

type statsQuerier = querier.Querier[
	*statsTask, querier.DataStorage, querier.LSS, *statsShard, *statsHead,
]

type statsChunkQuerier = querier.ChunkQuerier[
	*statsTask, querier.DataStorage, querier.LSS, *statsShard, *statsHead,
]

type QuerierStatsSuite struct {
	suite.Suite
	registry *prometheus.Registry
	metrics  *querier.Metrics
	head     *statsHead
	shards   []*statsShard
	q        *statsQuerier
	cq       *statsChunkQuerier
	matchers []*labels.Matcher
	hints    *storage.SelectHints
}

func TestQuerierStatsSuite(t *testing.T) {
	suite.Run(t, new(QuerierStatsSuite))
}

func (s *QuerierStatsSuite) SetupTest() {
	previous := querier.QueryStageMetricsEnabled
	querier.QueryStageMetricsEnabled = true
	s.T().Cleanup(func() { querier.QueryStageMetricsEnabled = previous })

	s.registry = prometheus.NewRegistry()
	s.metrics = querier.NewMetrics(s.registry, querier.QueryableAppenderSource)
	raw := newStageStatsHead(s.T(), 4, numberOfShards)
	s.shards = make([]*statsShard, len(raw.Shards()))

	for i, sh := range raw.Shards() {
		s.shards[i] = newStatsShard(sh)
	}

	engine := head.NewHead("stats-test", s.shards, func(sh *statsShard, _ uint16) *statsShard {
		return sh
	}, nil, 0, nil)
	s.head = &statsHead{Head: engine}
	s.T().Cleanup(func() { s.Require().NoError(engine.Close()) })
	s.q = querier.NewQuerier(s.head, querier.NewNoOpShardedDeduplicator, 0, 10, 1, 0, s.metrics)
	s.cq = querier.NewChunkQuerier(s.head, querier.NewNoOpShardedDeduplicator, 0, 10, nil, s.metrics)
	s.matchers = []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "__name__", "stage_metric")}
	s.hints = &storage.SelectHints{Start: 0, End: 10, Step: 1}
}

func (s *QuerierStatsSuite) TestCancelledAcquire() {
	s.head.acquireErr = context.Canceled
	result := s.q.Select(context.Background(), false, s.hints, s.matchers...)
	s.Require().ErrorIs(result.Err(), context.Canceled)
	s.Equal(map[string]float64{"semaphore_wait": 1}, s.counts("select_range", false))
	s.Empty(s.counts("select_range", true))
	s.Zero(s.histogramCount("prompp_querier_query_select_duration", "range"))
}

func (s *QuerierStatsSuite) TestChunkLabelNames() {
	values, _, err := s.cq.LabelNames(context.Background(), &storage.LabelHints{}, s.matchers...)
	s.Require().NoError(err)
	s.Equal([]string{"__name__", "__name__", "series", "series"}, values)
	s.assertMetadata("chunk_label_names")
	s.Zero(s.histogramCount("prompp_querier_query_label_names_duration", ""))
}

func (s *QuerierStatsSuite) TestChunkLabelValues() {
	values, _, err := s.cq.LabelValues(context.Background(), "series", &storage.LabelHints{}, s.matchers...)
	s.Require().NoError(err)
	s.Len(values, 4)
	s.assertMetadata("chunk_label_values")
	s.Zero(s.histogramCount("prompp_querier_query_label_values_duration", ""))
}

func (s *QuerierStatsSuite) TestChunkSelect() {
	result := s.cq.Select(context.Background(), false, s.hints, s.matchers...)
	s.Require().NoError(result.Err())
	s.True(result.Next())
	s.assertSelect("select_chunks", false)
}

func (s *QuerierStatsSuite) TestChunkSelectLoadError() {
	s.enableLoad(errors.New("load failed"))
	result := s.cq.Select(context.Background(), false, s.hints, s.matchers...)
	s.Require().NoError(result.Err())
	s.False(result.Next())
	s.assertSelect("select_chunks", true)
}

func (s *QuerierStatsSuite) TestClosedAcquire() {
	s.head.acquireErr = locker.ErrSemaphoreClosed
	s.Require().NoError(s.cq.Select(context.Background(), false, s.hints, s.matchers...).Err())
	s.Equal(map[string]float64{"semaphore_wait": 1}, s.counts("select_chunks", false))
}

func (s *QuerierStatsSuite) TestConcurrentQueries() {
	var wg sync.WaitGroup

	for range 8 {
		wg.Go(func() {
			for range 20 {
				_ = s.q.Select(context.Background(), false, s.hints, s.matchers...)
			}
		})
	}

	wg.Wait()

	for _, count := range s.counts("select_range", false) {
		s.Equal(float64(160), count)
	}

	for _, count := range s.counts("select_range", true) {
		s.Equal(float64(160), count)
	}

	s.NotContains(s.counts("select_range", false), "data_load_wait")
}

func (s *QuerierStatsSuite) TestCrossSeriesRange() {
	s.Require().NoError(querier.SetSelectFuncOptimize("cross"))
	s.T().Cleanup(querier.SetDefaultOptimizeType)
	q := querier.NewQuerier(s.head, querier.NewNoOpShardedDeduplicator, 0, 10, 1, 0, s.metrics)
	q.IsPossibleToOptimize = func([]*cppbridge.LSSQueryResult, *storage.SelectHints, int64, int64) bool {
		return true
	}
	s.hints.Func = "sum"
	result := q.Select(context.Background(), false, s.hints, s.matchers...)

	s.Require().NoError(result.Err())
	s.assertSelect("select_range", false)
	s.Equal(uint64(1), s.histogramCount("prompp_querier_query_select_duration", "range"))
	s.Equal(float64(1), testutil.ToFloat64(s.metrics.OptimizationType.WithLabelValues("cross_series")))
	before := s.counts("select_range", false)
	iterateSeriesSet(result)

	s.Require().NoError(result.Err())
	s.Equal(before, s.counts("select_range", false))
}

func (s *QuerierStatsSuite) TestFailedAcquireAllOperations() {
	// Arrange
	ctx := context.Background()

	for _, tc := range []struct {
		operation string
		call      func() error
	}{
		{
			operation: "select_instant",
			call: func() error {
				return s.q.Select(ctx, false, &storage.SelectHints{}, s.matchers...).Err()
			},
		},
		{
			operation: "select_range",
			call: func() error {
				return s.q.Select(ctx, false, s.hints, s.matchers...).Err()
			},
		},
		{
			operation: "select_chunks",
			call: func() error {
				return s.cq.Select(ctx, false, s.hints, s.matchers...).Err()
			},
		},
		{
			operation: "label_names",
			call: func() error {
				_, _, err := s.q.LabelNames(ctx, &storage.LabelHints{}, s.matchers...)

				return err
			},
		},
		{
			operation: "label_values",
			call: func() error {
				_, _, err := s.q.LabelValues(ctx, "series", &storage.LabelHints{}, s.matchers...)

				return err
			},
		},
		{
			operation: "chunk_label_names",
			call: func() error {
				_, _, err := s.cq.LabelNames(ctx, &storage.LabelHints{}, s.matchers...)

				return err
			},
		},
		{
			operation: "chunk_label_values",
			call: func() error {
				_, _, err := s.cq.LabelValues(ctx, "series", &storage.LabelHints{}, s.matchers...)

				return err
			},
		},
	} {
		s.Run(tc.operation, func() {
			// Arrange
			s.head.acquireErr = context.Canceled

			// Act
			err := tc.call()

			// Assert
			s.Require().ErrorIs(err, context.Canceled)
			s.Equal(map[string]float64{"semaphore_wait": 1}, s.counts(tc.operation, false))
			s.Empty(s.counts(tc.operation, true))
		})
	}
}

func (s *QuerierStatsSuite) TestHistogramAfterRelease() {
	s.assertHistogramAfterRelease()
}

func (s *QuerierStatsSuite) TestInstant() {
	result := s.q.Select(context.Background(), false, &storage.SelectHints{}, s.matchers...)
	s.Require().NoError(result.Err())
	s.assertSelect("select_instant", false)
	s.Equal(uint64(1), s.histogramCount("prompp_querier_query_select_duration", "instant"))
}

func (s *QuerierStatsSuite) TestInstantLoadError() {
	err := errors.New("load failed")
	s.enableLoad(err)
	result := s.q.Select(context.Background(), false, &storage.SelectHints{}, s.matchers...)
	s.Require().ErrorIs(result.Err(), err)
	counts := s.counts("select_instant", false)
	s.Equal(float64(1), counts["data_load_wait"])
	s.NotContains(counts, "cleanup")
	s.NotContains(counts, "build_result")
}

func (s *QuerierStatsSuite) TestIteratorDoesNotRecord() {
	result := s.q.Select(context.Background(), false, s.hints, s.matchers...)
	before := s.counts("select_range", false)
	iterateSeriesSet(result)
	s.Equal(before, s.counts("select_range", false))
}

func (s *QuerierStatsSuite) TestLabelNames() {
	values, _, err := s.q.LabelNames(context.Background(), &storage.LabelHints{Limit: 1}, s.matchers...)
	s.Require().NoError(err)
	s.Equal([]string{"__name__"}, values)
	s.assertMetadata("label_names")
	s.Equal(uint64(1), s.histogramCount("prompp_querier_query_label_names_duration", ""))
}

func (s *QuerierStatsSuite) TestLabelValues() {
	values, _, err := s.q.LabelValues(context.Background(), "series", &storage.LabelHints{}, s.matchers...)
	s.Require().NoError(err)
	s.Equal([]string{"0", "1", "2", "3"}, values)
	s.assertMetadata("label_values")
}

func (s *QuerierStatsSuite) TestMetadataAnnotations() {
	s.shards[0].lss.metadataErr = errors.New("metadata failed")
	_, anns, err := s.q.LabelNames(context.Background(), &storage.LabelHints{}, s.matchers...)
	s.Require().NoError(err)
	s.Len(anns, 1)
	s.assertMetadata("label_names")
}

func (s *QuerierStatsSuite) TestMetadataCancelledAfterWait() {
	for _, names := range []bool{true, false} {
		ctx, cancel := context.WithCancel(context.Background())
		s.shards[0].lss.cancel = cancel
		operation := "label_values"
		var err error

		if names {
			operation = "label_names"
			_, _, err = s.q.LabelNames(ctx, &storage.LabelHints{}, s.matchers...)
		} else {
			_, _, err = s.q.LabelValues(ctx, "series", &storage.LabelHints{}, s.matchers...)
		}

		s.Require().ErrorIs(err, context.Canceled)
		s.Equal(map[string]float64{
			"semaphore_wait": 1, "prepare": 1, "lss_query": 1,
		}, s.counts(operation, false))
	}
}

func (s *QuerierStatsSuite) TestNilMetrics() {
	q := querier.NewQuerier(s.head, querier.NewNoOpShardedDeduplicator, 0, 10, 1, 0, nil)
	s.Require().NoError(q.Select(context.Background(), false, s.hints, s.matchers...).Err())
	s.Empty(s.counts("select_range", false))
	s.Empty(s.counts("select_range", true))
}

func (s *QuerierStatsSuite) TestNilRegisterer() {
	metrics := querier.NewMetrics(nil, querier.QueryableAppenderSource)
	q := querier.NewQuerier(s.head, querier.NewNoOpShardedDeduplicator, 0, 10, 1, 0, metrics)
	s.Require().NoError(q.Select(context.Background(), false, s.hints, s.matchers...).Err())
	s.Empty(s.counts("select_range", false))
}

func (s *QuerierStatsSuite) TestNoMatches() {
	matcher := labels.MustNewMatcher(labels.MatchEqual, "__name__", "absent")
	result := s.q.Select(context.Background(), false, s.hints, matcher)
	s.Require().NoError(result.Err())
	s.False(result.Next())
	s.Equal(map[string]float64{"lss_query_selector": 1}, s.counts("select_range", true))
	s.NotContains(s.counts("select_range", false), "data_load_wait")
}

func (s *QuerierStatsSuite) TestOptimizedRange() {
	s.Require().NoError(querier.SetSelectFuncOptimize("all"))
	s.T().Cleanup(querier.SetDefaultOptimizeType)
	q := querier.NewQuerier(s.head, querier.NewNoOpShardedDeduplicator, 0, 10, 1, 0, s.metrics)
	s.hints.Func = "max_over_time"
	s.hints.Range = 10
	s.hints.Step = 4
	s.Require().NoError(q.Select(context.Background(), false, s.hints, s.matchers...).Err())
	s.assertSelect("select_range", false)
	s.Equal(float64(1), testutil.ToFloat64(s.metrics.OptimizationType.WithLabelValues("aggregation")))
}

func (s *QuerierStatsSuite) TestRange() {
	s.Require().NoError(s.q.Select(context.Background(), false, s.hints, s.matchers...).Err())
	s.assertSelect("select_range", false)
}

func (s *QuerierStatsSuite) TestRangeLoadError() {
	s.enableLoad(errors.New("load failed"))
	result := s.q.Select(context.Background(), false, s.hints, s.matchers...)
	s.Require().NoError(result.Err())
	s.False(result.Next())
	s.assertSelect("select_range", true)
}

func (s *QuerierStatsSuite) TestRegistrationAndSources() {
	_ = s.q.Select(context.Background(), false, s.hints, s.matchers...)
	reused := querier.NewMetrics(s.registry, querier.QueryableAppenderSource)
	q := querier.NewQuerier(s.head, querier.NewNoOpShardedDeduplicator, 0, 10, 1, 0, reused)
	_ = q.Select(context.Background(), false, s.hints, s.matchers...)
	s.Equal(float64(2), s.counts("select_range", false)["build_result"])
	other := querier.NewMetrics(s.registry, querier.QueryableStorageSource)
	q = querier.NewQuerier(s.head, querier.NewNoOpShardedDeduplicator, 0, 10, 1, 0, other)
	_ = q.Select(context.Background(), false, s.hints, s.matchers...)
	s.Equal(float64(2), s.counts("select_range", false)["build_result"])
	families, err := s.registry.Gather()
	s.Require().NoError(err)
	var series int

	for _, family := range families {
		if !strings.HasPrefix(family.GetName(), "prompp_querier_query_stage_") &&
			!strings.HasPrefix(family.GetName(), "prompp_querier_query_shard_stage_") {
			continue
		}

		s.Contains(family.GetHelp(), "including unsuccessful attempts")
		series += len(family.GetMetric())

		for _, metric := range family.GetMetric() {
			s.Len(metric.GetLabel(), 3)

			if family.GetName() == "prompp_querier_query_stage_executions_total" &&
				metricLabel(metric, "source") == querier.QueryableStorageSource &&
				metricLabel(metric, "operation") == "select_range" && metricLabel(metric, "stage") == "build_result" {
				s.Equal(float64(1), metric.GetCounter().GetValue())
			}
		}
	}

	s.Equal(188, series)
}

func (s *QuerierStatsSuite) TestRotationWithDifferentShardCount() {
	for _, count := range []uint16{1, 3} {
		h := newStageStatsHead(s.T(), 6, count)
		q := querier.NewQuerier(h, querier.NewNoOpShardedDeduplicator, 0, 10, 1, 0, s.metrics)
		s.Require().NoError(q.Select(context.Background(), false, s.hints, s.matchers...).Err())
	}

	s.Equal(float64(2), s.counts("select_range", false)["build_result"])
	s.Equal(float64(2), s.counts("select_range", true)["data_storage_query"])
}

func (s *QuerierStatsSuite) TestSelectorError() {
	s.shards[0].lss.selectorErr = errors.New("selector failed")
	result := s.q.Select(context.Background(), false, s.hints, s.matchers...)
	s.Require().EqualError(result.Err(), "selector failed")
	s.Equal(map[string]float64{
		"semaphore_wait": 1, "prepare": 1, "lss_query_selector": 1,
	}, s.counts("select_range", false))
	s.Equal(map[string]float64{"lss_query_selector": 1}, s.counts("select_range", true))

	s.shards[0].lss.selectorErr = nil
	s.Require().NoError(s.cq.Select(context.Background(), false, s.hints, s.matchers...).Err())
	s.Require().NoError(s.q.Select(context.Background(), false, s.hints, s.matchers...).Err())
	s.Equal(float64(2), s.counts("select_range", false)["lss_query_selector"])
	s.Equal(float64(1), s.counts("select_range", true)["data_storage_query"])
}

func (s *QuerierStatsSuite) TestSelectorTaskCleanupOnEnqueuePanic() {
	for _, tc := range []struct {
		operation string
		call      func()
	}{
		{
			operation: "select_instant",
			call: func() {
				s.q.Select(context.Background(), false, &storage.SelectHints{}, s.matchers...)
			},
		},
		{
			operation: "select_range",
			call: func() {
				s.q.Select(context.Background(), false, s.hints, s.matchers...)
			},
		},
		{
			operation: "select_chunks",
			call: func() {
				s.cq.Select(context.Background(), false, s.hints, s.matchers...)
			},
		},
	} {
		s.Run(tc.operation, func() {
			// Arrange
			var enqueued *statsTask
			var returned int
			var released bool
			s.head.onEnqueue = func(t *statsTask) {
				enqueued = t
				panic("enqueue failed")
			}
			s.head.onPutTask = func(t *statsTask) {
				s.Same(enqueued, t)
				returned++
			}
			s.head.onRelease = func() {
				s.Equal(1, returned)
				released = true
			}

			// Act
			s.Require().PanicsWithValue("enqueue failed", tc.call)

			// Assert
			s.Require().NotNil(enqueued)
			s.Equal(1, returned)
			s.True(released)
		})
	}
}

func (s *QuerierStatsSuite) TestSharedLoadWaiters() {
	first := querier.NewLoadAndQueryWaiter[*statsTask, querier.DataStorage, querier.LSS](s.head)
	second := querier.NewLoadAndQueryWaiter[*statsTask, querier.DataStorage, querier.LSS](s.head)
	s.False(first.HasQueriesAfterPrimaryWait())
	s.False(second.HasQueriesAfterPrimaryWait())
	gate := make(chan struct{})

	for _, sh := range s.shards {
		sh.loadGate = gate
	}

	t := s.head.CreateTask("test_shared_load", func(sh *statsShard) error {
		first.Add(sh, 0)
		second.Add(sh, 0)

		return nil
	})
	s.head.Enqueue(t)
	err := t.Wait()
	close(gate)
	s.Require().NoError(err)
	s.True(first.HasQueriesAfterPrimaryWait())
	s.True(second.HasQueriesAfterPrimaryWait())
	s.Require().NoError(first.Wait())
	s.Require().NoError(second.Wait())
	s.True(first.HasQueriesAfterPrimaryWait())
	s.True(second.HasQueriesAfterPrimaryWait())
	s.head.PutTask(t)

	for _, sh := range s.shards {
		s.Equal(int64(1), sh.loadCalls.Load())
	}
}

func (s *QuerierStatsSuite) TestSlotsDoNotRetainPreviousQuery() {
	s.Require().NoError(s.q.Select(context.Background(), false, s.hints, s.matchers...).Err())
	matcher := labels.MustNewMatcher(labels.MatchEqual, "__name__", "absent")
	s.Require().NoError(s.q.Select(context.Background(), false, s.hints, matcher).Err())
	s.Equal(map[string]float64{
		"lss_query_selector": 2, "data_storage_query": 1,
	}, s.counts("select_range", true))
}

func (s *QuerierStatsSuite) TestStageMetricsDisabled() {
	querier.QueryStageMetricsEnabled = false
	s.registry = prometheus.NewRegistry()
	s.metrics = querier.NewMetrics(s.registry, querier.QueryableAppenderSource)
	s.q = querier.NewQuerier(s.head, querier.NewNoOpShardedDeduplicator, 0, 10, 1, 0, s.metrics)
	s.cq = querier.NewChunkQuerier(s.head, querier.NewNoOpShardedDeduplicator, 0, 10, nil, s.metrics)
	start := time.Now()

	s.assertHistogramAfterRelease()
	s.head.onRelease = nil
	s.Require().NoError(s.cq.Select(context.Background(), false, s.hints, s.matchers...).Err())
	_, _, err := s.cq.LabelNames(context.Background(), &storage.LabelHints{}, s.matchers...)
	s.Require().NoError(err)
	_, _, err = s.cq.LabelValues(context.Background(), "series", &storage.LabelHints{}, s.matchers...)
	s.Require().NoError(err)
	maxDuration := float64(time.Since(start).Nanoseconds()) / 1e3
	families, err := s.registry.Gather()
	s.Require().NoError(err)
	s.Len(families, 4)

	for _, family := range families {
		s.NotContains(family.GetName(), "_stage_")

		if family.GetType() != dto.MetricType_HISTOGRAM {
			s.Equal("prompp_querier_query_optimization_type_count", family.GetName())

			continue
		}

		for _, metric := range family.GetMetric() {
			histogram := metric.GetHistogram()
			s.Require().NotNil(histogram)
			s.Equal(uint64(1), histogram.GetSampleCount())
			s.Positive(histogram.GetSampleSum())
			s.LessOrEqual(histogram.GetSampleSum(), maxDuration)
		}
	}

	s.head.acquireErr = context.Canceled
	s.Require().ErrorIs(s.q.Select(context.Background(), false, s.hints, s.matchers...).Err(), context.Canceled)
	s.Equal(uint64(1), s.histogramCount("prompp_querier_query_select_duration", "range"))
}

func (s *QuerierStatsSuite) TestSuccessfulLoad() {
	s.enableLoad(nil)
	s.Require().NoError(s.q.Select(context.Background(), false, s.hints, s.matchers...).Err())
	s.assertSelect("select_range", true)
}

func (s *QuerierStatsSuite) TestUnexecutedStagesAreZero() {
	families, err := s.registry.Gather()
	s.Require().NoError(err)
	var series int

	for _, family := range families {
		if !strings.HasPrefix(family.GetName(), "prompp_querier_query_stage_") &&
			!strings.HasPrefix(family.GetName(), "prompp_querier_query_shard_stage_") {
			continue
		}

		for _, metric := range family.GetMetric() {
			s.Zero(metric.GetCounter().GetValue())
			series++
		}
	}

	s.Equal(94, series)
}

func (s *QuerierStatsSuite) assertHistogramAfterRelease() {
	s.T().Helper()

	for _, tc := range []struct {
		operation string
		histogram string
		queryType string
	}{
		{"select_instant", "prompp_querier_query_select_duration", "instant"},
		{"select_range", "prompp_querier_query_select_duration", "range"},
		{"label_names", "prompp_querier_query_label_names_duration", ""},
		{"label_values", "prompp_querier_query_label_values_duration", ""},
	} {
		s.Run(tc.operation, func() {
			s.head.onRelease = func() {
				s.Zero(s.histogramCount(tc.histogram, tc.queryType))
			}

			var err error

			switch tc.operation {
			case "select_instant":
				err = s.q.Select(context.Background(), false, &storage.SelectHints{}, s.matchers...).Err()
			case "select_range":
				err = s.q.Select(context.Background(), false, s.hints, s.matchers...).Err()
			case "label_names":
				_, _, err = s.q.LabelNames(context.Background(), &storage.LabelHints{}, s.matchers...)
			case "label_values":
				_, _, err = s.q.LabelValues(context.Background(), "series", &storage.LabelHints{}, s.matchers...)
			}

			s.Require().NoError(err)
			s.Equal(uint64(1), s.histogramCount(tc.histogram, tc.queryType))
			s.NotContains(s.counts(tc.operation, false), "cleanup")
		})
	}
}

func (s *QuerierStatsSuite) assertMetadata(operation string) {
	s.T().Helper()
	s.Equal(map[string]float64{
		"semaphore_wait": 1, "prepare": 1, "lss_query": 1, "build_result": 1,
	}, s.counts(operation, false))
	s.Equal(map[string]float64{"lss_query": 1}, s.counts(operation, true))
}

func (s *QuerierStatsSuite) assertSelect(operation string, load bool) {
	s.T().Helper()
	expected := map[string]float64{
		"semaphore_wait": 1, "prepare": 1, "lss_query_selector": 1, "lss_query": 1,
		"data_storage_query": 1, "build_result": 1,
	}

	if load {
		expected["data_load_wait"] = 1
	}

	s.Equal(expected, s.counts(operation, false))
	s.Equal(map[string]float64{"lss_query_selector": 1, "data_storage_query": 1}, s.counts(operation, true))
}

func (s *QuerierStatsSuite) counts(operation string, shards bool) map[string]float64 {
	s.T().Helper()
	name := "prompp_querier_query_stage_executions_total"

	if shards {
		name = "prompp_querier_query_shard_stage_executions_total"
	}

	families, err := s.registry.Gather()
	s.Require().NoError(err)
	counts := make(map[string]float64)

	for _, family := range families {
		if family.GetName() != name {
			continue
		}

		for _, metric := range family.GetMetric() {
			if metricLabel(metric, "operation") == operation &&
				metricLabel(metric, "source") == querier.QueryableAppenderSource && metric.GetCounter().GetValue() > 0 {
				counts[metricLabel(metric, "stage")] = metric.GetCounter().GetValue()
			}
		}
	}

	return counts
}

func (s *QuerierStatsSuite) enableLoad(err error) {
	for _, sh := range s.shards {
		sh.ds.needLoad = true
		sh.loadErr = err
	}
}

func (s *QuerierStatsSuite) histogramCount(name, queryType string) uint64 {
	families, err := s.registry.Gather()
	s.Require().NoError(err)

	for _, family := range families {
		if family.GetName() != name {
			continue
		}

		for _, metric := range family.GetMetric() {
			if metricLabel(metric, "query_type") == queryType {
				return metric.GetHistogram().GetSampleCount()
			}
		}
	}

	return 0
}

func metricLabel(metric *dto.Metric, name string) string {
	for _, label := range metric.GetLabel() {
		if label.GetName() == name {
			return label.GetValue()
		}
	}

	return ""
}
