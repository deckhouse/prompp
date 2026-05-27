package querier_test

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
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
			expected: &prom_storage.SelectHints{},
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
		result := querier.SwitchFuncOptimize(test.hints, 7)
		s.Require().Equal(test.expected, result)
	}
}

//
// Constants
//

const (
	// defaultStartMs is the default start time.
	defaultStartMs = 1779290789000

	// defaultStep is the default step.
	defaultStep = 15 * time.Second

	// defaultCountOfSteps is the default count of steps.
	defaultCountOfSteps = 480
)

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
type offset struct {
	duration model.Duration
	negative bool
}

// newOffset creates a new [offset].
func newOffset(duration time.Duration) offset {
	if duration < 0 {
		return offset{duration: model.Duration(-duration), negative: true}
	}

	return offset{duration: model.Duration(duration)}
}

// String converts the offset to query string.
func (o offset) String() string {
	if o.duration == 0 {
		return ""
	}

	if o.negative {
		return fmt.Sprintf(" offset -%s", o.duration)
	}

	return fmt.Sprintf(" offset %s", o.duration)
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
	window      model.Duration
	step        model.Duration
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
// query
//

// queryResult is a result of a query.
type queryResult struct {
	qry promql.Query
	res *promql.Result
}

// queryRange executes a range query and returns the result.
func queryRange(
	ctx context.Context,
	optimization string,
	queryEngine *promql.Engine,
	q prom_storage.Queryable,
	opts promql.QueryOpts,
	query string,
	start, end time.Time,
	step time.Duration,
) (*queryResult, error) {
	if err := querier.SetSelectFuncOptimize(optimization); err != nil {
		return nil, err
	}

	qry, err := queryEngine.NewRangeQuery(ctx, q, opts, query, start, end, step)
	if err != nil {
		return nil, err
	}

	res := qry.Exec(ctx)
	if res.Err != nil {
		qry.Close()
		return nil, res.Err
	}

	return &queryResult{qry: qry, res: res}, nil
}

// queryInstant executes an instant query and returns the result.
func queryInstant(
	ctx context.Context,
	optimization string,
	queryEngine *promql.Engine,
	q prom_storage.Queryable,
	opts promql.QueryOpts,
	query string,
	ts time.Time,
) (*queryResult, error) {
	if err := querier.SetSelectFuncOptimize(optimization); err != nil {
		return nil, err
	}
	qry, err := queryEngine.NewInstantQuery(ctx, q, opts, query, ts)
	if err != nil {
		return nil, err
	}
	res := qry.Exec(ctx)
	if res.Err != nil {
		qry.Close()
		return nil, res.Err
	}

	return &queryResult{qry: qry, res: res}, nil
}

//
// QuerierOptimizeSuite
//

type QuerierOptimizeSuite struct {
	suite.Suite

	dataDir string
	head    *storage.Head
	start   time.Time
	end     time.Time
	step    time.Duration

	lookbackDelta time.Duration
	queryOpts     promql.QueryOpts
	metricNames   []string
	queryFuncs    []queryFunc
}

func TestQuerieOptimizeSuite(t *testing.T) {
	suite.Run(t, new(QuerierOptimizeSuite))
}

func (s *QuerierOptimizeSuite) SetupSuite() {
	s.start = time.UnixMilli(defaultStartMs)
	s.step = defaultStep
	s.end = s.start.Add(s.step * 480) // 480 steps

	s.dataDir = s.createDataDirectory()
	s.head = s.mustCreateHead(0)
	s.fillHead()

	s.lookbackDelta = 5 * time.Minute
	s.queryOpts = promql.NewPrometheusQueryOpts(false, s.lookbackDelta)

	s.queryFuncs = []queryFunc{
		// {name: "rate", needRange: true}, // -
		// {name: "irate", needRange: true}, // -
		// {name: "delta", needRange: true}, // -
		// {name: "idelta", needRange: true}, // -
		// {name: "increase", needRange: true}, // -
		{name: "min_over_time", needRange: true},  // +
		{name: "max_over_time", needRange: true},  // +
		{name: "last_over_time", needRange: true}, // +
		// {name: "sum_over_time", needRange: true}, // -
		// {name: "resets", needRange: true}, // -
		{name: "changes", needRange: true}, // +
	}

	q, err := s.Querier(s.start.UnixMilli(), s.end.UnixMilli())
	s.Require().NoError(err)

	names, _, err := q.LabelValues(s.T().Context(), "__name__", &prom_storage.LabelHints{})
	s.Require().NoError(err)

	s.metricNames = querier.DeduplicateAndSortStringSlices(names)
	s.Require().NoError(q.Close())
}

func (s *QuerierOptimizeSuite) TearDownSuite() {
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
		{
			Labels:  labels.FromStrings("__name__", "counter_metric", "value", "with_resets"),
			Samples: make([]cppbridge.Sample, 0, countOfSamples),
		},
	}

	resetsCounter := 1
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

		if resetsCounter%10 == 0 {
			resetsCounter = 1
		}
		timeSeries[6].Samples = append(timeSeries[6].Samples,
			cppbridge.Sample{Timestamp: tsMilli, Value: float64(resetsCounter)},
		)

		resetsCounter++
		valueCounter++
	}

	s.appendTimeSeries(timeSeries)
}

// Querier implements the [prom_storage.Queryable] interface.
func (s *QuerierOptimizeSuite) Querier(mint, maxt int64) (prom_storage.Querier, error) {
	return querier.NewQuerier(s.head, querier.NewNoOpShardedDeduplicator, mint, maxt, nil, nil), nil
}

//
// MatrixQuerierOptimizeSuiteSuite
//

type MatrixQuerierOptimizeSuiteSuite struct {
	QuerierOptimizeSuite

	queryEngine *promql.Engine
	steps       []time.Duration
	subQueries  []subQuery
	modifiers   []modifier
	offsets     []offset
}

func TestMatrixQuerierOptimizeSuiteSuite(t *testing.T) {
	suite.Run(t, new(MatrixQuerierOptimizeSuiteSuite))
}

func (s *MatrixQuerierOptimizeSuiteSuite) SetupSuite() {
	s.QuerierOptimizeSuite.SetupSuite()

	s.queryEngine = promql.NewEngine(promql.EngineOpts{
		Logger:                   log.NewNopLogger(),
		MaxSamples:               10000,
		Timeout:                  10 * time.Second,
		LookbackDelta:            s.lookbackDelta,
		NoStepSubqueryIntervalFn: func(int64) int64 { return s.lookbackDelta.Milliseconds() },
		EnableAtModifier:         true,
		EnableNegativeOffset:     true,
	})

	s.steps = []time.Duration{
		s.step - time.Second,
		s.step,
		(s.step - time.Second) * 2,
		s.step * 2,
		(s.step - time.Second) * 4,
		s.step * 4,
		(s.step - time.Second) * 5,
	}
	s.subQueries = []subQuery{
		{window: model.Duration(s.step), step: 0},                               // [15s]
		{window: model.Duration(s.step * 4), step: 0},                           // [60s]
		{window: model.Duration(s.step*4 - time.Second), step: 0},               // [59s]
		{window: model.Duration(s.step*4 + time.Second), step: 0},               // [61s]
		{window: model.Duration(s.step * 4), step: 0, defaultStep: true},        // [60s:]
		{window: model.Duration(s.step*16 - time.Second), step: 0},              // [239s]
		{window: model.Duration(s.step * 16), step: 0},                          // [240s]
		{window: model.Duration(s.step*16 + time.Second), step: 0},              // [241s]
		{window: model.Duration(s.step * 16), step: model.Duration(s.step * 4)}, // [240s:60s]
	}
	s.modifiers = []modifier{
		modifierNone,
		modifier(fmt.Sprintf(modifierAt, s.start.Unix()+(s.end.Unix()-s.start.Unix())/2)), // middle of the range
		modifierEnd,
		modifierStart,
	}
	s.offsets = []offset{
		newOffset(0),
		newOffset(s.end.Sub(s.start) / 2),
		newOffset(-s.end.Sub(s.start) / 2),
		newOffset(s.end.Sub(s.start)),
		newOffset(-s.end.Sub(s.start)),
	}

	q, err := s.Querier(s.start.UnixMilli(), s.end.UnixMilli())
	s.Require().NoError(err)

	names, _, err := q.LabelValues(s.T().Context(), "__name__", &prom_storage.LabelHints{})
	s.Require().NoError(err)

	s.metricNames = querier.DeduplicateAndSortStringSlices(names)
	s.Require().NoError(q.Close())
}

// rangeArgs runs the given function for all combinations of
// query functions, metric names, steps, subqueries, modifiers and offsets.
//
//revive:disable-next-line:cognitive-complexity // matrix test
func (s *MatrixQuerierOptimizeSuiteSuite) rangeArgs(fn func(
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

// rangeArgsWithStep runs the given function for all combinations of
// query functions, metric names, subqueries, modifiers and offsets.
//
//revive:disable-next-line:cognitive-complexity // matrix test
func (s *MatrixQuerierOptimizeSuiteSuite) rangeArgsWithStep(fn func(
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

func (s *MatrixQuerierOptimizeSuiteSuite) TestQueryRange() {
	ctx := s.T().Context()

	s.rangeArgs(func(qFunc queryFunc, metricName string, step time.Duration, subq subQuery, mod modifier, o offset) {
		query := qFunc.toQueryString(metricName, subq, mod, o)
		s.Run(fmt.Sprintf("%s_step_%s", query, step), func() {
			res, err := queryRange(ctx, "none", s.queryEngine, s, s.queryOpts, query, s.start, s.end, step)
			s.Require().NoError(err)
			defer res.qry.Close()

			resOpt, err := queryRange(ctx, "all", s.queryEngine, s, s.queryOpts, query, s.start, s.end, step)
			s.Require().NoError(err)
			defer resOpt.qry.Close()

			s.Require().Equal(res.res, resOpt.res)
		})
	})
}

func (s *MatrixQuerierOptimizeSuiteSuite) TestQueryRangeWithStep() {
	ctx := s.T().Context()
	w := 30 * time.Minute
	stepWindow := w / 2

	for st, end := s.start, s.start.Add(w); !end.After(s.end); st, end = st.Add(stepWindow), end.Add(stepWindow) {
		s.rangeArgs(func(qFunc queryFunc, mName string, step time.Duration, subq subQuery, mod modifier, o offset) {
			query := qFunc.toQueryString(mName, subq, mod, o)
			s.Run(fmt.Sprintf("%s_step_%s", query, step), func() {
				res, err := queryRange(ctx, "none", s.queryEngine, s, s.queryOpts, query, st, end, step)
				s.Require().NoError(err)
				defer res.qry.Close()

				resOpt, err := queryRange(ctx, "all", s.queryEngine, s, s.queryOpts, query, st, end, step)
				s.Require().NoError(err)
				defer resOpt.qry.Close()

				s.Require().Equal(res.res, resOpt.res)
			})
		})
	}
}

func (s *MatrixQuerierOptimizeSuiteSuite) TestQueryInstantStart() {
	ctx := s.T().Context()

	s.rangeArgsWithStep(func(qFunc queryFunc, metricName string, subq subQuery, mod modifier, o offset) {
		query := qFunc.toQueryString(metricName, subq, mod, o)
		s.Run(query, func() {
			res, err := queryInstant(ctx, "none", s.queryEngine, s, s.queryOpts, query, s.start)
			s.Require().NoError(err)
			defer res.qry.Close()

			resOpt, err := queryInstant(ctx, "all", s.queryEngine, s, s.queryOpts, query, s.start)
			s.Require().NoError(err)
			defer resOpt.qry.Close()

			s.Require().Equal(res.res, resOpt.res)
		})
	})
}

func (s *MatrixQuerierOptimizeSuiteSuite) TestQueryInstantMiddle() {
	ctx := s.T().Context()

	s.rangeArgsWithStep(func(qFunc queryFunc, metricName string, subq subQuery, mod modifier, o offset) {
		query := qFunc.toQueryString(metricName, subq, mod, o)
		s.Run(query, func() {
			res, err := queryInstant(ctx, "none", s.queryEngine, s, s.queryOpts, query, s.start.Add(s.step*90))
			s.Require().NoError(err)
			defer res.qry.Close()

			resOpt, err := queryInstant(ctx, "all", s.queryEngine, s, s.queryOpts, query, s.start.Add(s.step*90))
			s.Require().NoError(err)
			defer resOpt.qry.Close()

			s.Require().Equal(res.res, resOpt.res)
		})
	})
}

func (s *MatrixQuerierOptimizeSuiteSuite) TestQueryInstantEnd() {
	ctx := s.T().Context()

	s.rangeArgsWithStep(func(qFunc queryFunc, metricName string, subq subQuery, mod modifier, o offset) {
		query := qFunc.toQueryString(metricName, subq, mod, o)
		s.Run(query, func() {
			res, err := queryInstant(ctx, "none", s.queryEngine, s, s.queryOpts, query, s.end)
			s.Require().NoError(err)
			defer res.qry.Close()

			resOpt, err := queryInstant(ctx, "all", s.queryEngine, s, s.queryOpts, query, s.end)
			s.Require().NoError(err)
			defer resOpt.qry.Close()

			s.Require().Equal(res.res, resOpt.res)
		})
	})
}

func (s *MatrixQuerierOptimizeSuiteSuite) TestQueryRangeSingle() {
	ctx := s.T().Context()
	queryF := "max_over_time(((min_over_time(%s[60s])))[60s:])"
	start := s.start.Add(12 * time.Second)
	for _, metricName := range s.metricNames {
		query := fmt.Sprintf(queryF, metricName)
		s.Run(query, func() {
			res, err := queryRange(ctx, "none", s.queryEngine, s, s.queryOpts, query, start, s.end, s.step)
			s.Require().NoError(err)
			defer res.qry.Close()

			resOpt, err := queryRange(ctx, "all", s.queryEngine, s, s.queryOpts, query, start, s.end, s.step)
			s.Require().NoError(err)
			defer resOpt.qry.Close()

			s.Require().Equal(res.res, resOpt.res)
		})
	}
}
