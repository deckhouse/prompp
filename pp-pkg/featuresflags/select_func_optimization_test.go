package featuresflags

import (
	"bytes"
	"testing"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	pp_storage "github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/head/poolprovider"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
	"github.com/prometheus/prometheus/pp/go/storage/head/transactionhead"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
	"github.com/prometheus/prometheus/storage"
)

// SelectFuncOptimizationSuite checks feature settings through optimization metrics of real queries.
type SelectFuncOptimizationSuite struct {
	suite.Suite

	output  bytes.Buffer
	logger  log.Logger
	head    *pp_storage.TransactionHead
	metrics *querier.Metrics
}

func TestSelectFuncOptimizationSuite(t *testing.T) {
	suite.Run(t, new(SelectFuncOptimizationSuite))
}

func (s *SelectFuncOptimizationSuite) SetupTest() {
	s.output.Reset()
	s.logger = log.NewLogfmtLogger(&s.output)
	s.metrics = querier.NewMetrics(prometheus.NewRegistry(), querier.QueryableStorageSource)
	querier.SetDefaultOptimizeType()

	lss := shard.NewLSS()
	ds := shard.NewDataStorage(false, false)
	storagetest.MustAppendTimeSeriesToLSSAndDataStorage(lss, ds, storagetest.TimeSeries{
		Labels: labels.FromStrings("__name__", "metric"),
		Samples: []cppbridge.Sample{
			{Timestamp: 1, Value: 2},
			{Timestamp: 5, Value: 3},
		},
	})
	storageShard := shard.NewShard(lss, ds, nil, nil, wal.NewNoopWal(), 0)
	s.head = transactionhead.NewHead[*shard.Shard, *shard.PerGoroutineShard](
		"feature-test",
		storageShard,
		shard.NewPerGoroutineShard[wal.NoopWal](storageShard, 1),
		poolprovider.NewHeadPool[*shard.PerGoroutineShard](1),
	)
}

func (s *SelectFuncOptimizationSuite) TearDownTest() {
	querier.SetDefaultOptimizeType()
}

func (s *SelectFuncOptimizationSuite) TestCombinationWithWhitespace() {
	setSelectFuncOptimization(s.logger, "drop_point | cross")

	s.selectAggregationAndCrossQueries()

	s.Equal(float64(1), testutil.ToFloat64(s.metrics.OptimizationType.WithLabelValues("aggregation")))
	s.Equal(float64(1), testutil.ToFloat64(s.metrics.OptimizationType.WithLabelValues("cross_series")))
	s.Equal(float64(0), testutil.ToFloat64(s.metrics.OptimizationType.WithLabelValues("none")))
	s.Contains(s.output.String(), "Select function optimization is set.")
	s.NotContains(s.output.String(), "level=error")
}

func (s *SelectFuncOptimizationSuite) TestInvalidOptionResetsDefault() {
	setSelectFuncOptimization(s.logger, "all")
	s.output.Reset()

	setSelectFuncOptimization(s.logger, "drop_point|unknown")
	s.selectAggregationAndCrossQueries()

	s.Equal(float64(2), testutil.ToFloat64(s.metrics.OptimizationType.WithLabelValues("none")))
	s.Contains(s.output.String(), "level=error")
	s.Contains(s.output.String(), "reset to default")
	s.NotContains(s.output.String(), "Select function optimization is set.")
}

func (s *SelectFuncOptimizationSuite) TestSuccessiveValuesReplaceSelection() {
	setSelectFuncOptimization(s.logger, "all")
	setSelectFuncOptimization(s.logger, "drop_point")

	s.selectAggregationAndCrossQueries()

	s.Equal(float64(1), testutil.ToFloat64(s.metrics.OptimizationType.WithLabelValues("aggregation")))
	s.Equal(float64(0), testutil.ToFloat64(s.metrics.OptimizationType.WithLabelValues("cross_series")))
	s.Equal(float64(1), testutil.ToFloat64(s.metrics.OptimizationType.WithLabelValues("none")))
}

func (s *SelectFuncOptimizationSuite) selectAggregationAndCrossQueries() {
	s.selectWithHints(&storage.SelectHints{Start: 0, End: 8, Step: 4, Range: 4, Func: "max_over_time"})
	s.selectWithHints(&storage.SelectHints{Start: 0, End: 8, Step: 4, LookbackDelta: 4, Func: "sum"})
}

func (s *SelectFuncOptimizationSuite) selectWithHints(hints *storage.SelectHints) {
	q := querier.NewQuerier(s.head, querier.NewNoOpShardedDeduplicator, 0, 8, 1, 0, s.metrics)
	q.IsPossibleToOptimize = func(
		[]*cppbridge.LSSQueryResult,
		*storage.SelectHints,
		int64, int64,
	) bool {
		return true
	}
	seriesSet := q.Select(s.T().Context(), false, hints, labels.MustNewMatcher(labels.MatchEqual, "__name__", "metric"))
	s.NotEmpty(storagetest.TimeSeriesFromSeriesSet(seriesSet, true))
	s.Require().NoError(seriesSet.Err())
	s.Require().NoError(q.Close())
}
