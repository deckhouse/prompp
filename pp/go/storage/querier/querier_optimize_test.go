package querier_test

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
	"github.com/prometheus/prometheus/promql"
	prom_storage "github.com/prometheus/prometheus/storage"
)

//
// SwitchFuncOptimizeSuite
//

type SwitchFuncOptimizeSuite struct {
	suite.Suite
}

func TestSwitchFuncOptimizeSuite(t *testing.T) {
	suite.Run(t, new(SwitchFuncOptimizeSuite))
}

func (s *SwitchFuncOptimizeSuite) TestNone() {
	tests := []struct {
		hints    *prom_storage.SelectHints
		expected *prom_storage.SelectHints
	}{
		{
			hints:    &prom_storage.SelectHints{},
			expected: &prom_storage.SelectHints{},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "sum"},
			expected: &prom_storage.SelectHints{},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "sum", By: true},
			expected: &prom_storage.SelectHints{},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "sum", By: true, Grouping: []string{"label"}},
			expected: &prom_storage.SelectHints{},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "sum", By: false, Grouping: []string{"label"}},
			expected: &prom_storage.SelectHints{},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "sum_over_time"},
			expected: &prom_storage.SelectHints{},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "max_over_time"},
			expected: &prom_storage.SelectHints{},
		},
	}

	for _, test := range tests {
		result := querier.SwitchFuncOptimize(test.hints, 0)
		s.Require().Equal(test.expected, result)
	}
}

func (s *SwitchFuncOptimizeSuite) TestDropPoint() {
	tests := []struct {
		hints    *prom_storage.SelectHints
		expected *prom_storage.SelectHints
	}{
		{
			hints:    &prom_storage.SelectHints{},
			expected: &prom_storage.SelectHints{},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "sum"},
			expected: &prom_storage.SelectHints{},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "sum", By: true},
			expected: &prom_storage.SelectHints{},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "sum", By: true, Grouping: []string{"label"}},
			expected: &prom_storage.SelectHints{},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "sum", By: false, Grouping: []string{"label"}},
			expected: &prom_storage.SelectHints{},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "sum_over_time"},
			expected: &prom_storage.SelectHints{},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "max_over_time"},
			expected: &prom_storage.SelectHints{Func: "max_over_time"},
		},
	}

	for _, test := range tests {
		result := querier.SwitchFuncOptimize(test.hints, 1)
		s.Require().Equal(test.expected, result)
	}
}

func (s *SwitchFuncOptimizeSuite) TestNewPoint() {
	tests := []struct {
		hints    *prom_storage.SelectHints
		expected *prom_storage.SelectHints
	}{
		{
			hints:    &prom_storage.SelectHints{},
			expected: &prom_storage.SelectHints{},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "sum"},
			expected: &prom_storage.SelectHints{},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "sum", By: true},
			expected: &prom_storage.SelectHints{},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "sum", By: true, Grouping: []string{"label"}},
			expected: &prom_storage.SelectHints{},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "sum", By: false, Grouping: []string{"label"}},
			expected: &prom_storage.SelectHints{},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "sum_over_time"},
			expected: &prom_storage.SelectHints{Func: "sum_over_time"},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "max_over_time"},
			expected: &prom_storage.SelectHints{},
		},
	}

	for _, test := range tests {
		result := querier.SwitchFuncOptimize(test.hints, 2)
		s.Require().Equal(test.expected, result)
	}
}

func (s *SwitchFuncOptimizeSuite) TestCrossSeries() {
	tests := []struct {
		hints    *prom_storage.SelectHints
		expected *prom_storage.SelectHints
	}{
		{
			hints:    &prom_storage.SelectHints{},
			expected: &prom_storage.SelectHints{},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "sum"},
			expected: &prom_storage.SelectHints{Func: "sum"},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "sum", By: true},
			expected: &prom_storage.SelectHints{Func: "sum", By: true},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "sum", By: true, Grouping: []string{"label"}},
			expected: &prom_storage.SelectHints{Func: "sum", By: true, Grouping: []string{"label"}},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "sum", By: false, Grouping: []string{"label"}},
			expected: &prom_storage.SelectHints{},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "sum_over_time"},
			expected: &prom_storage.SelectHints{},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "max_over_time"},
			expected: &prom_storage.SelectHints{},
		},
	}

	for _, test := range tests {
		result := querier.SwitchFuncOptimize(test.hints, 4)
		s.Require().Equal(test.expected, result)
	}
}

func (s *SwitchFuncOptimizeSuite) TestAll() {
	tests := []struct {
		hints    *prom_storage.SelectHints
		expected *prom_storage.SelectHints
	}{
		{
			hints:    &prom_storage.SelectHints{},
			expected: &prom_storage.SelectHints{},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "sum"},
			expected: &prom_storage.SelectHints{Func: "sum"},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "sum", By: true},
			expected: &prom_storage.SelectHints{Func: "sum", By: true},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "sum", By: true, Grouping: []string{"label"}},
			expected: &prom_storage.SelectHints{Func: "sum", By: true, Grouping: []string{"label"}},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "sum", By: false, Grouping: []string{"label"}},
			expected: &prom_storage.SelectHints{},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "sum_over_time"},
			expected: &prom_storage.SelectHints{Func: "sum_over_time"},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "max_over_time"},
			expected: &prom_storage.SelectHints{Func: "max_over_time"},
		},
	}

	for _, test := range tests {
		result := querier.SwitchFuncOptimize(test.hints, 7)
		s.Require().Equal(test.expected, result)
	}
}

//
// modifier
//

// modifier is the modifier for query string.
type modifier string

const (
	// modifierNone is the modifier for query string empty.
	modifierNone modifier = ""

	// modifierAt is the modifier for query string At.
	modifierAt string = " @ %d"

	// modifierEnd is the modifier for query string End.
	modifierEnd modifier = " @ end()"

	// modifierStart is the modifier for query string Start.
	modifierStart modifier = " @ start()"
)

//
// offset
//

// offset is the offset for query string.
type offset time.Duration

// String converts the offset to query string.
func (o offset) String() string {
	if o == 0 {
		return ""
	}

	return fmt.Sprintf(" offset %s", time.Duration(o))
}

//
// queryFunc
//

// queryFunc is the struct for query function.
type queryFunc struct {
	name      string
	needRange bool
}

// toQueryString converts the query function to query string.
func (q *queryFunc) toQueryString(metricName string, sq subQuery, mod modifier, o offset) string {
	if q.needRange {
		return fmt.Sprintf("%s(%s%s%s%s)", q.name, metricName, sq.toQueryString(), mod, o)
	}

	return fmt.Sprintf("%s(%s%s%s)", q.name, metricName, mod, o)
}

//
// subQuery
//

// subQuery is the struct for subquery.
type subQuery struct {
	window      time.Duration
	step        time.Duration
	defaultStep bool
}

// toQueryString converts the subquery to query string.
func (s *subQuery) toQueryString() string {
	if s.step == 0 {
		if s.defaultStep {
			return fmt.Sprintf("[%s:]", s.window)
		}

		return fmt.Sprintf("[%s]", s.window)
	}

	return fmt.Sprintf("[%s:%s]", s.window, s.step)
}

//
// QuerierOptimizeSuite
//

type QuerierOptimizeSuite struct {
	suite.Suite

	dataDir     string
	head        *storage.Head
	start       time.Time
	end         time.Time
	step        time.Duration
	queryEngine *promql.Engine
	queryOpts   promql.QueryOpts
	metricNames []string
	queryFuncs  []queryFunc
	steps       []time.Duration
	subQueries  []subQuery
	modifiers   []modifier
	offsets     []offset
}

func TestQuerieOptimizeSuite(t *testing.T) {
	suite.Run(t, new(QuerierOptimizeSuite))
}

func (s *QuerierOptimizeSuite) SetupSuite() {
	s.start = time.UnixMilli(1779290789000)
	s.end = time.UnixMilli(1779297989000)
	s.step = 15 * time.Second

	s.queryFuncs = []queryFunc{
		// {name: "rate", needRange: true},
		{name: "irate", needRange: true},
		// {name: "delta", needRange: true},
		// // {name: "idelta", needRange: true},
		// {name: "increase", needRange: true},
		// {name: "min_over_time", needRange: true},
		// {name: "max_over_time", needRange: true},
		// {name: "last_over_time", needRange: true},
		// {name: "sum_over_time", needRange: true},
		// {name: "count_over_time", needRange: true},
		// {name: "resets", needRange: true},
		// {name: "changes", needRange: true},
	}
	s.steps = []time.Duration{
		// s.step - time.Second,
		s.step,
		// (s.step - time.Second) * 2,
		// s.step * 2,
		// (s.step - time.Second) * 3,
		// s.step * 3,
		// (s.step - time.Second) * 4,
		// s.step * 4,
		// (s.step - time.Second) * 5,
	}
	s.subQueries = []subQuery{
		{window: s.step, step: 0},                           // [15s]
		{window: s.step * 4, step: 0},                       // [60s]
		{window: s.step*4 - time.Second, step: 0},           // [59s]
		{window: s.step*4 + time.Second, step: 0},           // [61s]
		{window: s.step * 4, step: 0, defaultStep: true},    // [60s:]
		{window: s.step * 4, step: s.step},                  // [60s:15s]
		{window: s.step * 4, step: s.step - time.Second},    // [60s:14s]
		{window: s.step * 4, step: s.step + time.Second},    // [60s:16s]
		{window: s.step * 16, step: s.step * 4},             // [240s:60s]
		{window: s.step * 16, step: s.step*4 - time.Second}, // [240s:59s]
		{window: s.step * 16, step: s.step*4 + time.Second}, // [240s:61s]
		{window: s.step * 4, step: s.step * 8},              // [60s:120s]
	}
	s.modifiers = []modifier{
		modifierNone,
		// modifier(fmt.Sprintf(modifierAt, s.start.Unix()+(s.end.Unix()-s.start.Unix())/2)), // middle of the range
		// modifierEnd,
		// modifierStart,
	}
	s.offsets = []offset{
		offset(0),
		// offset(5 * time.Minute),
		// offset(-5 * time.Minute),
		// offset(10 * time.Minute),
		// offset(-10 * time.Minute),
		// offset(20 * time.Minute),
		// offset(-20 * time.Minute),
		// offset(30 * time.Minute),
		// offset(-30 * time.Minute),
		// offset(60 * time.Minute),
		// offset(-60 * time.Minute),
	}

	s.dataDir = s.createDataDirectory()
	s.head = s.mustCreateHead(1)
	s.fillHead()

	lookbackDelta := 5 * time.Minute
	s.queryEngine = promql.NewEngine(promql.EngineOpts{
		Logger:                   log.NewNopLogger(),
		MaxSamples:               10000,
		Timeout:                  10 * time.Second,
		LookbackDelta:            lookbackDelta,
		NoStepSubqueryIntervalFn: func(int64) int64 { return lookbackDelta.Milliseconds() },
		EnableAtModifier:         true,
		EnableNegativeOffset:     true,
	})
	s.queryOpts = promql.NewPrometheusQueryOpts(false, lookbackDelta)

	q, err := s.Querier(s.start.UnixMilli(), s.end.UnixMilli())
	s.Require().NoError(err)

	names, _, err := q.LabelValues(s.T().Context(), "__name__", &prom_storage.LabelHints{})
	s.Require().NoError(err)

	s.metricNames = querier.DeduplicateAndSortStringSlices(names)
	s.Require().NoError(q.Close())
}

func (s *QuerierOptimizeSuite) TearDownSuite() {
	s.Require().NoError(s.queryEngine.Close())
	s.Require().NoError(s.head.Close())
}

func (s *QuerierOptimizeSuite) createDataDirectory() string {
	dataDir := filepath.Join(s.T().TempDir(), "data")
	s.Require().NoError(os.MkdirAll(dataDir, os.ModeDir))
	return dataDir
}

func (s *QuerierOptimizeSuite) mustCreateCatalog() *catalog.Catalog {
	l, err := catalog.NewFileLogV2(filepath.Join(s.dataDir, "catalog.log"))
	s.Require().NoError(err)

	c, err := catalog.New(
		clockwork.NewFakeClock(),
		l,
		&catalog.DefaultIDGenerator{},
		catalog.DefaultMaxLogFileSize,
		nil,
	)
	s.Require().NoError(err)

	return c
}

func (s *QuerierOptimizeSuite) mustCreateHead(unloadDataStorageInterval time.Duration) *storage.Head {
	h, err := storage.NewBuilder(
		s.mustCreateCatalog(),
		s.dataDir,
		maxSegmentSize,
		prometheus.DefaultRegisterer,
		unloadDataStorageInterval,
	).Build(0, numberOfShards)
	s.Require().NoError(err)
	return h
}

func (s *QuerierOptimizeSuite) appendTimeSeries(timeSeries []storagetest.TimeSeries) {
	storagetest.MustAppendTimeSeries(&s.Suite, s.head, timeSeries)
}

func (s *QuerierOptimizeSuite) fillHead() {
	countOfSamples := (s.end.UnixMilli()-s.start.UnixMilli())/s.step.Milliseconds() + 1
	timeSeries := []storagetest.TimeSeries{
		{
			Labels:  labels.FromStrings("__name__", "sin_cos_metric", "value", "sin", "inc", "tick_usecond"),
			Samples: make([]cppbridge.Sample, 0, countOfSamples),
		},
		{
			Labels:  labels.FromStrings("__name__", "sin_cos_metric", "value", "sin", "inc", "tick_second"),
			Samples: make([]cppbridge.Sample, 0, countOfSamples),
		},
		{
			Labels:  labels.FromStrings("__name__", "sin_cos_metric", "value", "cos", "inc", "tick_usecond"),
			Samples: make([]cppbridge.Sample, 0, countOfSamples),
		},
		{
			Labels:  labels.FromStrings("__name__", "sin_cos_metric", "value", "cos", "inc", "tick_second"),
			Samples: make([]cppbridge.Sample, 0, countOfSamples),
		},
		{
			Labels:  labels.FromStrings("__name__", "counter_metric", "value", "inc"),
			Samples: make([]cppbridge.Sample, 0, countOfSamples),
		},
		{
			Labels:  labels.FromStrings("__name__", "counter_metric", "value", "with_stalenan"),
			Samples: make([]cppbridge.Sample, 0, countOfSamples),
		},
	}

	valueCounter := 1
	for ts := s.start; !ts.After(s.end); ts = ts.Add(s.step) {
		tsMilli := ts.UnixMilli()
		timeSeries[0].Samples = append(timeSeries[0].Samples,
			cppbridge.Sample{Timestamp: tsMilli, Value: math.Sin(float64(ts.UnixMilli())) * 10},
		)
		timeSeries[1].Samples = append(timeSeries[1].Samples,
			cppbridge.Sample{Timestamp: tsMilli, Value: math.Sin(float64(ts.Second())) * 10},
		)
		timeSeries[2].Samples = append(timeSeries[2].Samples,
			cppbridge.Sample{Timestamp: tsMilli, Value: math.Cos(float64(ts.UnixMilli())) * 10},
		)
		timeSeries[3].Samples = append(timeSeries[3].Samples,
			cppbridge.Sample{Timestamp: tsMilli, Value: math.Cos(float64(ts.Second())) * 10},
		)
		timeSeries[4].Samples = append(timeSeries[4].Samples,
			cppbridge.Sample{Timestamp: tsMilli, Value: float64(valueCounter)},
		)

		if valueCounter%5 == 0 {
			timeSeries[5].Samples = append(timeSeries[5].Samples,
				cppbridge.Sample{Timestamp: tsMilli, Value: math.Float64frombits(value.StaleNaN)},
			)
		} else {
			timeSeries[5].Samples = append(timeSeries[5].Samples,
				cppbridge.Sample{Timestamp: tsMilli, Value: float64(valueCounter + 1)},
			)
		}

		valueCounter++
	}

	s.appendTimeSeries(timeSeries)
}

//revive:disable-next-line:cognitive-complexity // matrix test
func (s *QuerierOptimizeSuite) rangeArgs(fn func(
	qFunc queryFunc,
	metricName string,
	step time.Duration,
	subq subQuery,
	mod modifier,
	o offset,
),
) {
	for _, qFunc := range s.queryFuncs {
		for _, metricName := range s.metricNames {
			for _, step := range s.steps {
				for _, subq := range s.subQueries {
					for _, mod := range s.modifiers {
						for _, o := range s.offsets {
							fn(qFunc, metricName, step, subq, mod, o)
						}
					}
				}
			}
		}
	}
}

func (s *QuerierOptimizeSuite) rangeArgsWithStep(fn func(
	qFunc queryFunc,
	metricName string,
	subq subQuery,
	mod modifier,
	o offset,
),
) {
	for _, qFunc := range s.queryFuncs {
		for _, metricName := range s.metricNames {
			for _, subq := range s.subQueries {
				for _, mod := range s.modifiers {
					for _, o := range s.offsets {
						fn(qFunc, metricName, subq, mod, o)
					}
				}
			}
		}
	}
}

func (s *QuerierOptimizeSuite) Querier(mint, maxt int64) (prom_storage.Querier, error) {
	return querier.NewQuerier(s.head, querier.NewNoOpShardedDeduplicator, mint, maxt, nil, nil), nil
}

func (s *QuerierOptimizeSuite) TestQuerierOptimizeRange() {
	ctx := s.T().Context()

	s.rangeArgs(func(qFunc queryFunc, metricName string, step time.Duration, subq subQuery, mod modifier, o offset) {
		query := qFunc.toQueryString(metricName, subq, mod, o)
		s.Run(fmt.Sprintf("%s_step_%s", query, step), func() {
			s.Require().NoError(querier.SetSelectFuncOptimize("none"))
			qry, err := s.queryEngine.NewRangeQuery(ctx, s, s.queryOpts, query, s.start, s.end, step)
			s.Require().NoError(err)
			defer qry.Close()
			res := qry.Exec(ctx)
			s.Require().NoError(res.Err)

			s.Require().NoError(querier.SetSelectFuncOptimize("all"))
			qryOpt, err := s.queryEngine.NewRangeQuery(ctx, s, s.queryOpts, query, s.start, s.end, step)
			s.Require().NoError(err)
			resOpt := qryOpt.Exec(ctx)
			s.Require().NoError(resOpt.Err)
			defer qryOpt.Close()

			s.Require().Equal(res.Value, resOpt.Value)
		})
	})
}

func (s *QuerierOptimizeSuite) TestQuerierOptimizeRangeWithStep() {
	ctx := s.T().Context()
	w := 20 * time.Minute
	step := w / 2

	for st, end := s.start, s.start.Add(w); !end.After(s.end); st, end = st.Add(step), end.Add(step) {
		s.rangeArgs(func(qFunc queryFunc, metricName string, step time.Duration, subq subQuery, mod modifier, o offset) {
			query := qFunc.toQueryString(metricName, subq, mod, o)
			s.Run(fmt.Sprintf("%s_step_%s", query, step), func() {
				s.Require().NoError(querier.SetSelectFuncOptimize("none"))
				qry, err := s.queryEngine.NewRangeQuery(ctx, s, s.queryOpts, query, st, end, step)
				s.Require().NoError(err)
				defer qry.Close()
				res := qry.Exec(ctx)
				s.Require().NoError(res.Err)

				s.Require().NoError(querier.SetSelectFuncOptimize("all"))
				qryOpt, err := s.queryEngine.NewRangeQuery(ctx, s, s.queryOpts, query, st, end, step)
				s.Require().NoError(err)
				defer qryOpt.Close()
				resOpt := qryOpt.Exec(ctx)
				s.Require().NoError(resOpt.Err)

				s.Require().Equal(res.Value, resOpt.Value)
			})
		})
	}
}

func (s *QuerierOptimizeSuite) TestQuerierOptimizeInstantStart() {
	baseCtx := s.T().Context()

	s.rangeArgsWithStep(func(qFunc queryFunc, metricName string, subq subQuery, mod modifier, o offset) {
		query := qFunc.toQueryString(metricName, subq, mod, o)
		s.Run(query, func() {
			s.Require().NoError(querier.SetSelectFuncOptimize("none"))
			qry, err := s.queryEngine.NewInstantQuery(baseCtx, s, s.queryOpts, query, s.start)
			s.Require().NoError(err)
			defer qry.Close()
			res := qry.Exec(baseCtx)
			s.Require().NoError(res.Err)

			s.Require().NoError(querier.SetSelectFuncOptimize("all"))
			qryOpt, err := s.queryEngine.NewInstantQuery(baseCtx, s, s.queryOpts, query, s.start)
			s.Require().NoError(err)
			defer qryOpt.Close()
			resOpt := qryOpt.Exec(baseCtx)
			s.Require().NoError(resOpt.Err)

			s.Require().Equal(res.Value, resOpt.Value)
		})
	})
}

func (s *QuerierOptimizeSuite) TestQuerierOptimizeInstantMiddle() {
	ctx := s.T().Context()

	s.rangeArgsWithStep(func(qFunc queryFunc, metricName string, subq subQuery, mod modifier, o offset) {
		query := qFunc.toQueryString(metricName, subq, mod, o)
		s.Run(query, func() {
			s.Require().NoError(querier.SetSelectFuncOptimize("none"))
			// qry, err := s.queryEngine.NewInstantQuery(ctx, s, s.queryOpts, query, s.start.Add(s.step*90))
			qry, err := s.queryEngine.NewInstantQuery(ctx, s, s.queryOpts, query, s.start.Add(s.step*3))
			s.Require().NoError(err)
			defer qry.Close()
			res := qry.Exec(ctx)
			s.Require().NoError(res.Err)

			s.Require().NoError(querier.SetSelectFuncOptimize("all"))
			qryOpt, err := s.queryEngine.NewInstantQuery(ctx, s, s.queryOpts, query, s.start.Add(s.step*3))
			s.Require().NoError(err)
			defer qryOpt.Close()
			resOpt := qryOpt.Exec(ctx)
			s.Require().NoError(resOpt.Err)

			s.T().Log(res.Value)
			s.T().Log(resOpt.Value)
			s.Require().Equal(res.Value, resOpt.Value)
		})
	})
}

func (s *QuerierOptimizeSuite) TestQuerierOptimizeInstantEnd() {
	baseCtx := s.T().Context()

	s.rangeArgsWithStep(func(qFunc queryFunc, metricName string, subq subQuery, mod modifier, o offset) {
		query := qFunc.toQueryString(metricName, subq, mod, o)
		s.Run(query, func() {
			s.Require().NoError(querier.SetSelectFuncOptimize("none"))
			qry, err := s.queryEngine.NewInstantQuery(baseCtx, s, s.queryOpts, query, s.end)
			s.Require().NoError(err)
			defer qry.Close()
			res := qry.Exec(baseCtx)
			s.Require().NoError(res.Err)

			s.Require().NoError(querier.SetSelectFuncOptimize("all"))
			qryOpt, err := s.queryEngine.NewInstantQuery(baseCtx, s, s.queryOpts, query, s.end)
			s.Require().NoError(err)
			defer qryOpt.Close()
			resOpt := qryOpt.Exec(baseCtx)
			s.Require().NoError(resOpt.Err)

			s.Require().Equal(res.Value, resOpt.Value)
		})
	})
}
