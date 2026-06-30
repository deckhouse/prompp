package querier_test

import (
	"context"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	prom_storage "github.com/prometheus/prometheus/storage"
)

//
// SwitchFuncOptimizeSuite
//

type SwitchFuncOptimizeSuite struct {
	suite.Suite

	isPossibleToOptimize func() bool
}

func TestSwitchFuncOptimizeSuite(t *testing.T) {
	suite.Run(t, new(SwitchFuncOptimizeSuite))
}

func (s *SwitchFuncOptimizeSuite) SetupSuite() {
	s.isPossibleToOptimize = func() bool { return true }
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
		{
			hints:    &prom_storage.SelectHints{Func: "max_over_time", IsSubquery: true},
			expected: &prom_storage.SelectHints{},
		},
	}

	for _, test := range tests {
		result := querier.SwitchFuncOptimize(test.hints, s.isPossibleToOptimize, 0)
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
		{
			hints:    &prom_storage.SelectHints{Func: "max_over_time", IsSubquery: true},
			expected: &prom_storage.SelectHints{},
		},
	}

	for _, test := range tests {
		result := querier.SwitchFuncOptimize(test.hints, s.isPossibleToOptimize, 1)
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
		{
			hints:    &prom_storage.SelectHints{Func: "max_over_time", IsSubquery: true},
			expected: &prom_storage.SelectHints{},
		},
	}

	for _, test := range tests {
		result := querier.SwitchFuncOptimize(test.hints, s.isPossibleToOptimize, 2)
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
		{
			hints:    &prom_storage.SelectHints{Func: "max_over_time", IsSubquery: true},
			expected: &prom_storage.SelectHints{},
		},
	}

	for _, test := range tests {
		result := querier.SwitchFuncOptimize(test.hints, s.isPossibleToOptimize, 4)
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
			expected: &prom_storage.SelectHints{},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "max_over_time"},
			expected: &prom_storage.SelectHints{Func: "max_over_time"},
		},
		{
			hints:    &prom_storage.SelectHints{Func: "max_over_time", IsSubquery: true},
			expected: &prom_storage.SelectHints{},
		},
	}

	for _, test := range tests {
		result := querier.SwitchFuncOptimize(test.hints, s.isPossibleToOptimize, 7)
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
	defaultStep = 30 * time.Second
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
// querierOptimize
//

// querierOptimize is the querier optimizer for testing.
type querierOptimize struct {
	noErrorFunc storagetest.NoErrorFunc

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

// setup sets up the querier optimizer.
func (s *querierOptimize) setup(
	ctx context.Context,
	baseDir string,
	noErrorFunc storagetest.NoErrorFunc,
	start time.Time,
	step time.Duration,
	countOfSteps int,
) {
	s.noErrorFunc = noErrorFunc
	s.start = start
	s.step = step
	s.end = s.start.Add(s.step * time.Duration(countOfSteps))

	s.dataDir = filepath.Join(baseDir, "data")
	s.noErrorFunc(os.MkdirAll(s.dataDir, os.ModeDir))

	s.head = s.mustCreateHead(0)
	s.fillHead(ctx)

	s.lookbackDelta = 5 * time.Minute
	s.queryOpts = promql.NewPrometheusQueryOpts(false, s.lookbackDelta)

	s.queryFuncs = []queryFunc{
		{name: "min_over_time", needRange: true},        // +
		{name: "max_over_time", needRange: true},        // +
		{name: "last_over_time", needRange: true},       // +
		{name: "changes", needRange: true},              // +
		{name: "min", needRange: false},                 // +
		{name: "min by(value) ", needRange: false},      // +
		{name: "max", needRange: false},                 // +
		{name: "max by(value) ", needRange: false},      // +
		{name: "sum", needRange: false},                 // +
		{name: "sum by(value, inc) ", needRange: false}, // +

		// {name: "rate", needRange: true}, // -
		// {name: "irate", needRange: true}, // -
		// {name: "delta", needRange: true}, // -
		// {name: "idelta", needRange: true}, // -
		// {name: "increase", needRange: true}, // -
		// {name: "sum_over_time", needRange: true}, // -
		// {name: "resets", needRange: true}, // -
	}

	q, err := s.Querier(s.start.UnixMilli(), s.end.UnixMilli())
	s.noErrorFunc(err)

	names, _, err := q.LabelValues(ctx, "__name__", &prom_storage.LabelHints{})
	s.noErrorFunc(err)

	s.metricNames = querier.DeduplicateAndSortStringSlices(names)
	s.noErrorFunc(q.Close())
}

// close closes the querier optimizer.
func (s *querierOptimize) close() error {
	return s.head.Close()
}

// mustCreateHead creates a new head.
func (s *querierOptimize) mustCreateHead(unloadDataStorageInterval time.Duration) *storage.Head {
	l, err := catalog.NewFileLogV2(filepath.Join(s.dataDir, "catalog.log"))
	s.noErrorFunc(err)

	c, err := catalog.New(
		clockwork.NewFakeClock(),
		l,
		&catalog.DefaultIDGenerator{},
		catalog.DefaultMaxLogFileSize,
		nil,
	)
	s.noErrorFunc(err)

	h, err := storage.NewBuilder(
		c,
		s.dataDir,
		maxSegmentSize,
		prometheus.DefaultRegisterer,
		unloadDataStorageInterval,
	).Build(0, numberOfShards)
	s.noErrorFunc(err)
	return h
}

// fillHead fills the head with the given time series.
func (s *querierOptimize) fillHead(ctx context.Context) {
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
			Labels:  labels.FromStrings("__name__", "sin_cos_metric", "value", "sin_stalenan", "inc", "tick_second"),
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

	floatStaleNaN := math.Float64frombits(value.StaleNaN)
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

		if valueCounter%5 == 0 {
			timeSeries[2].Samples = append(timeSeries[2].Samples,
				cppbridge.Sample{Timestamp: tsMilli, Value: floatStaleNaN},
			)
		} else {
			timeSeries[2].Samples = append(timeSeries[2].Samples,
				cppbridge.Sample{Timestamp: tsMilli, Value: math.Sin(float64(ts.Second())) * 10},
			)
		}

		timeSeries[3].Samples = append(timeSeries[3].Samples,
			cppbridge.Sample{Timestamp: tsMilli, Value: math.Cos(float64(ts.UnixMilli())) * 10},
		)
		timeSeries[4].Samples = append(timeSeries[4].Samples,
			cppbridge.Sample{Timestamp: tsMilli, Value: math.Cos(float64(ts.Second())) * 10},
		)
		timeSeries[5].Samples = append(timeSeries[5].Samples,
			cppbridge.Sample{Timestamp: tsMilli, Value: float64(valueCounter)},
		)

		if valueCounter%5 == 0 {
			timeSeries[6].Samples = append(timeSeries[6].Samples,
				cppbridge.Sample{Timestamp: tsMilli, Value: floatStaleNaN},
			)
		} else {
			timeSeries[6].Samples = append(timeSeries[6].Samples,
				cppbridge.Sample{Timestamp: tsMilli, Value: float64(valueCounter + 1)},
			)
		}

		if resetsCounter%10 == 0 {
			resetsCounter = 1
		}
		timeSeries[7].Samples = append(timeSeries[7].Samples,
			cppbridge.Sample{Timestamp: tsMilli, Value: float64(resetsCounter)},
		)

		resetsCounter++
		valueCounter++
	}

	storagetest.MustAppendTimeSeries(ctx, s.noErrorFunc, s.head, timeSeries)
}

// fillHeadWithCounter fills the head with the given number of counter metrics.
func (s *querierOptimize) fillHeadWithCounter(ctx context.Context, start, counter int) {
	countOfSamples := (s.end.UnixMilli()-s.start.UnixMilli())/s.step.Milliseconds() + 1
	timeSeries := make([]storagetest.TimeSeries, 0, counter*2)
	for i := start; i < start+counter; i++ {
		timeSeries = append(timeSeries, storagetest.TimeSeries{
			Labels:  labels.FromStrings("__name__", "counter_metric", "value", "inc", "counter", strconv.Itoa(i)),
			Samples: make([]cppbridge.Sample, 0, countOfSamples),
		})
	}

	for i := start; i < start+counter; i++ {
		timeSeries = append(timeSeries, storagetest.TimeSeries{
			Labels:  labels.FromStrings("__name__", "sin_cos_metric", "value", "sin_cos", "zvallue", strconv.Itoa(i)),
			Samples: make([]cppbridge.Sample, 0, countOfSamples),
		})
	}

	valueCounter := 1
	for ts := s.start; !ts.After(s.end); ts = ts.Add(s.step) {
		tsMilli := ts.UnixMilli()
		for i := range counter {
			timeSeries[i].Samples = append(timeSeries[i].Samples,
				cppbridge.Sample{Timestamp: tsMilli, Value: float64(valueCounter)},
			)

			timeSeries[i+counter].Samples = append(timeSeries[i+counter].Samples,
				cppbridge.Sample{Timestamp: tsMilli, Value: math.Sin(float64(i))*10 + math.Cos(float64(i))*10},
			)
		}

		valueCounter++
	}

	storagetest.MustAppendTimeSeries(ctx, s.noErrorFunc, s.head, timeSeries)
}

// Querier implements the [prom_storage.Queryable] interface.
func (s *querierOptimize) Querier(mint, maxt int64) (prom_storage.Querier, error) {
	return querier.NewQuerier(
		s.head,
		querier.NewNoOpShardedDeduplicator,
		mint,
		maxt,
		s.step.Milliseconds(),
		s.start.UnixMilli(),
		nil,
	), nil
}

//
// MatrixQuerierOptimizeSuiteSuite
//

type MatrixQuerierOptimizeSuiteSuite struct {
	suite.Suite
	querierOptimize

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
	s.querierOptimize.setup(
		s.T().Context(),
		s.T().TempDir(),
		s.Require().NoError,
		time.UnixMilli(defaultStartMs),
		defaultStep,
		defaultCountOfSteps,
	)

	querier.IsPossibleToOptimize = func(
		[]*cppbridge.LSSQueryResult,
		*prom_storage.SelectHints,
		int64, int64,
	) func() bool {
		return func() bool {
			return true
		}
	}

	s.queryEngine = promql.NewEngine(promql.EngineOpts{
		Logger:                   log.NewNopLogger(),
		MaxSamples:               10000,
		Timeout:                  10 * time.Second,
		LookbackDelta:            s.lookbackDelta,
		NoStepSubqueryIntervalFn: func(int64) int64 { return s.lookbackDelta.Milliseconds() },
		EnableAtModifier:         true,
		EnableNegativeOffset:     true,
	})

	s.steps = defaultSteps
	s.subQueries = defaultSubQueries
	s.modifiers = defaultModifiers
	s.offsets = defaultOffsets
}

func (s *MatrixQuerierOptimizeSuiteSuite) TearDownSuite() {
	s.Suite.Require().NoError(s.querierOptimize.close())
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

					if !qFunc.needRange {
						break // skip subQuery
					}
				}
			}
		}
	}
}

// rangeArgsWithoutStep runs the given function for all combinations of
// query functions, metric names, subqueries, modifiers and offsets.
//
//revive:disable-next-line:cognitive-complexity // matrix test
func (s *MatrixQuerierOptimizeSuiteSuite) rangeArgsWithoutStep(fn func(
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

				if !qFunc.needRange {
					break // skip subQuery
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

			s.Require().True(resultEqual(res.res, resOpt.res, query))
		})
	})
}

func (s *MatrixQuerierOptimizeSuiteSuite) TestQueryInstantStart() {
	ctx := s.T().Context()

	s.rangeArgsWithoutStep(func(qFunc queryFunc, metricName string, subq subQuery, mod modifier, o offset) {
		query := qFunc.toQueryString(metricName, subq, mod, o)
		s.Run(query, func() {
			res, err := queryInstant(ctx, "none", s.queryEngine, s, s.queryOpts, query, s.start)
			s.Require().NoError(err)
			defer res.qry.Close()

			resOpt, err := queryInstant(ctx, "all", s.queryEngine, s, s.queryOpts, query, s.start)
			s.Require().NoError(err)
			defer resOpt.qry.Close()

			s.Require().True(resultEqual(res.res, resOpt.res, query))
		})
	})
}

func (s *MatrixQuerierOptimizeSuiteSuite) TestQueryInstantMiddle() {
	ctx := s.T().Context()

	s.rangeArgsWithoutStep(func(qFunc queryFunc, metricName string, subq subQuery, mod modifier, o offset) {
		query := qFunc.toQueryString(metricName, subq, mod, o)
		s.Run(query, func() {
			res, err := queryInstant(ctx, "none", s.queryEngine, s, s.queryOpts, query, s.start.Add(s.step*90))
			s.Require().NoError(err)
			defer res.qry.Close()

			resOpt, err := queryInstant(ctx, "all", s.queryEngine, s, s.queryOpts, query, s.start.Add(s.step*90))
			s.Require().NoError(err)
			defer resOpt.qry.Close()

			s.Require().True(resultEqual(res.res, resOpt.res, query))
		})
	})
}

func (s *MatrixQuerierOptimizeSuiteSuite) TestQueryInstantEnd() {
	ctx := s.T().Context()

	s.rangeArgsWithoutStep(func(qFunc queryFunc, metricName string, subq subQuery, mod modifier, o offset) {
		query := qFunc.toQueryString(metricName, subq, mod, o)
		s.Run(query, func() {
			res, err := queryInstant(ctx, "none", s.queryEngine, s, s.queryOpts, query, s.end)
			s.Require().NoError(err)
			defer res.qry.Close()

			resOpt, err := queryInstant(ctx, "all", s.queryEngine, s, s.queryOpts, query, s.end)
			s.Require().NoError(err)
			defer resOpt.qry.Close()

			s.Require().True(resultEqual(res.res, resOpt.res, query))
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

			s.Require().True(resultEqual(res.res, resOpt.res, query))
		})
	}
}

//
// resultEqual
//

// defaultEpsilon is the default epsilon for comparing two values.
var defaultEpsilon = 0.0000000000001

// resultEqual compares two results.
//
//nolint:gocritic // unnamedResult // comporator
func resultEqual(exp, act *promql.Result, query string) (bool, string) {
	if exp == nil && act == nil {
		return true, ""
	}

	if exp == nil || act == nil {
		return false, fmt.Sprintf("query: %s\none of the results is nil", query)
	}

	if exp.Err != act.Err {
		return false, fmt.Sprintf("query: %s\nerror: %v, got %v", query, exp.Err, act.Err)
	}

	if !maps.Equal(exp.Warnings, act.Warnings) {
		return false, fmt.Sprintf("query: %s\nwarnings: %v, got %v", query, exp.Warnings, act.Warnings)
	}

	if eq, result := valueEqual(exp.Value, act.Value); !eq {
		return false, fmt.Sprintf("query: %s\n%s", query, result)
	}

	return true, ""
}

// valueEqual compares two values.
//
//nolint:gocritic // unnamedResult // comporator
func valueEqual(exp, act parser.Value) (bool, string) {
	if exp == nil && act == nil {
		return true, ""
	}

	if exp == nil || act == nil {
		return false, "one of the values is nil"
	}

	if exp.Type() != act.Type() {
		return false, fmt.Sprintf("value type: expected %s, got %s", exp.Type(), act.Type())
	}

	switch expType := exp.(type) {
	case promql.Scalar:
		return scalarEqual(expType, act.(promql.Scalar))

	case promql.Vector:
		return vectorEqual(expType, act.(promql.Vector))

	case promql.Matrix:
		return matrixEqual(expType, act.(promql.Matrix))

	default:
		return false, fmt.Sprintf("expected scalar, vector or matrix, got %T", exp)
	}
}

// scalarEqual compares two scalars.
//
//nolint:gocritic // unnamedResult // comporator
func scalarEqual(exp, act promql.Scalar) (bool, string) {
	if exp.T != act.T || !inEpsilon(exp.V, act.V, defaultEpsilon) {
		return false, fmt.Sprintf("scalar: %s != %s", exp, act)
	}

	return true, ""
}

// vectorEqual compares two vectors.
//
//nolint:gocritic // unnamedResult // comporator
func vectorEqual(exp, act promql.Vector) (bool, string) {
	if len(exp) != len(act) {
		return false, fmt.Sprintf("vector: length: %d != %d", len(exp), len(act))
	}

	msg := strings.Builder{}
	_, _ = msg.WriteString("vector:\n")
	isEqual := true

	// we are sorting, because the sorting is broken on instant requests
	// there are no guarantees for the order by groups in cross series set
	slices.SortFunc(exp, func(a, b promql.Sample) int {
		return labels.Compare(a.Metric, b.Metric)
	})
	slices.SortFunc(act, func(a, b promql.Sample) int {
		return labels.Compare(a.Metric, b.Metric)
	})

	for i, v := range exp {
		if eq, result := sampleEqual(v, act[i]); !eq {
			_, _ = msg.WriteString(result)
			isEqual = false
		}
	}

	if isEqual {
		msg.Reset()
	}

	return isEqual, msg.String()
}

// sampleEqual compares two samples.
//
//nolint:gocritic // unnamedResult // comporator
func sampleEqual(exp, act promql.Sample) (bool, string) {
	if !labels.Equal(exp.Metric, act.Metric) {
		return false, fmt.Sprintf("labels: %s != %s\n", exp.Metric, act.Metric)
	}

	msg := strings.Builder{}
	_, _ = fmt.Fprintf(&msg, "labels: %s\n", exp.Metric)
	isEqual := true

	if exp.T != act.T || !inEpsilon(exp.F, act.F, defaultEpsilon) {
		_, _ = fmt.Fprintf(
			&msg,
			"floats:\n %s != %s\n",
			promql.FPoint{T: exp.T, F: exp.F},
			promql.FPoint{T: act.T, F: act.F},
		)
		isEqual = false
	}

	if isEqual {
		msg.Reset()
	}

	return isEqual, msg.String()
}

// matrixEqual compares two matrices.
//
//nolint:gocritic // unnamedResult // comporator
func matrixEqual(exp, act promql.Matrix) (bool, string) {
	if len(exp) != len(act) {
		return false, fmt.Sprintf("matrix: length: %d != %d", len(exp), len(act))
	}

	msg := strings.Builder{}
	_, _ = msg.WriteString("matrix:\n")
	isEqual := true

	for i, v := range exp {
		if eq, result := seriesEqual(v, act[i]); !eq {
			_, _ = msg.WriteString(result)
			isEqual = false
		}
	}

	if isEqual {
		msg.Reset()
	}

	return isEqual, msg.String()
}

// seriesEqual compares two series.
//
//nolint:gocritic // unnamedResult // comporator
func seriesEqual(exp, act promql.Series) (bool, string) {
	if !labels.Equal(exp.Metric, act.Metric) {
		return false, fmt.Sprintf("labels: %s != %s\n", exp.Metric, act.Metric)
	}

	msg := strings.Builder{}
	isEqual := true
	_, _ = fmt.Fprintf(&msg, "labels: %s\n", exp.Metric)

	if len(exp.Floats) != len(act.Floats) {
		_, _ = fmt.Fprintf(&msg, "floats: length: %d != %d\n", len(exp.Floats), len(act.Floats))
		_, _ = fmt.Fprintf(&msg, "    exp: %s\n", exp.Floats)
		_, _ = fmt.Fprintf(&msg, "    act: %s\n", act.Floats)
		return false, msg.String()
	}

	_, _ = msg.WriteString("floats:\n")

	for i, v := range exp.Floats {
		if v.T != act.Floats[i].T || !inEpsilon(v.F, act.Floats[i].F, defaultEpsilon) {
			_, _ = fmt.Fprintf(&msg, "    %s != %s\n", v, act.Floats[i])
			isEqual = false
		}
	}

	if isEqual {
		msg.Reset()
	}

	return isEqual, msg.String()
}

// inEpsilon checks if two values are within epsilon.
func inEpsilon(expected, actual, epsilon float64) bool {
	if math.IsNaN(expected) && math.IsNaN(actual) {
		return true
	}

	if math.IsNaN(expected) || math.IsNaN(actual) {
		return false
	}

	if expected == 0 && actual == 0 {
		return true
	}

	if expected == 0 || actual == 0 {
		return false
	}

	return calcRelative(expected, actual) <= epsilon
}

// calcRelative calculates the relative between two values.
func calcRelative(expected, actual float64) float64 {
	return math.Abs(expected-actual) / math.Abs(expected)
}

//
// Benchmark
//

func BenchmarkRangeQuerySum(b *testing.B) {
	ctx := b.Context()
	qo := &querierOptimize{}
	qo.setup(
		ctx,
		b.TempDir(),
		func(err error, msgAndArgs ...any) { require.NoError(b, err, msgAndArgs) },
		time.UnixMilli(defaultStartMs),
		defaultStep,
		defaultCountOfSteps,
	)
	defer qo.close()
	querier.IsPossibleToOptimize = func(
		[]*cppbridge.LSSQueryResult,
		*prom_storage.SelectHints,
		int64, int64,
	) func() bool {
		return func() bool {
			return true
		}
	}

	queryEngine := promql.NewEngine(promql.EngineOpts{
		Logger:                   log.NewNopLogger(),
		MaxSamples:               100000,
		Timeout:                  10 * time.Second,
		LookbackDelta:            qo.lookbackDelta,
		NoStepSubqueryIntervalFn: func(int64) int64 { return qo.lookbackDelta.Milliseconds() },
		EnableAtModifier:         true,
		EnableNegativeOffset:     true,
	})

	query := "sum(counter_metric)"
	steps := []time.Duration{
		qo.step,
		qo.step * 2,
		qo.step * 3,
		qo.step * 4,
	}

	series := 5
	qo.fillHeadWithCounter(ctx, 0, series)

	// 3 - default series for counter_metric
	for i := series + 3; i < 14; i++ {
		qo.fillHeadWithCounter(ctx, i, 1)
		for _, step := range steps {
			b.Run(fmt.Sprintf("opt_none_series_%d_step_%s_range_0s", i, step), func(b *testing.B) {
				b.ResetTimer()
				for b.Loop() {
					res, err := queryRange(ctx, "none", queryEngine, qo, qo.queryOpts, query, qo.start, qo.end, step)
					require.NoError(b, err)
					res.qry.Close()
				}
			})

			b.Run(fmt.Sprintf("opt_all_series_%d_step_%s_range_0s", i, step), func(b *testing.B) {
				b.ResetTimer()
				for b.Loop() {
					res, err := queryRange(ctx, "all", queryEngine, qo, qo.queryOpts, query, qo.start, qo.end, step)
					require.NoError(b, err)
					res.qry.Close()
				}
			})
		}
	}
}

func BenchmarkRangeQuerySumBy(b *testing.B) {
	ctx := b.Context()
	qo := &querierOptimize{}
	qo.setup(
		ctx,
		b.TempDir(),
		func(err error, msgAndArgs ...any) { require.NoError(b, err, msgAndArgs) },
		time.UnixMilli(defaultStartMs),
		defaultStep,
		defaultCountOfSteps,
	)
	defer qo.close()
	querier.IsPossibleToOptimize = func(
		[]*cppbridge.LSSQueryResult,
		*prom_storage.SelectHints,
		int64, int64,
	) func() bool {
		return func() bool {
			return true
		}
	}

	queryEngine := promql.NewEngine(promql.EngineOpts{
		Logger:                   log.NewNopLogger(),
		MaxSamples:               100000,
		Timeout:                  10 * time.Second,
		LookbackDelta:            qo.lookbackDelta,
		NoStepSubqueryIntervalFn: func(int64) int64 { return qo.lookbackDelta.Milliseconds() },
		EnableAtModifier:         true,
		EnableNegativeOffset:     true,
	})

	query := "sum by(value) (counter_metric)"
	steps := []time.Duration{
		qo.step,
		qo.step * 2,
		qo.step * 3,
		qo.step * 4,
	}

	series := 7
	qo.fillHeadWithCounter(ctx, 0, series)

	// 3 - default series for counter_metric
	for i := series + 3; i < 20; i++ {
		qo.fillHeadWithCounter(ctx, i, 1)
		for _, step := range steps {
			b.Run(fmt.Sprintf("opt_none_series_%d_step_%s_range_0s", i, step), func(b *testing.B) {
				b.ResetTimer()
				for b.Loop() {
					res, err := queryRange(ctx, "none", queryEngine, qo, qo.queryOpts, query, qo.start, qo.end, step)
					require.NoError(b, err)
					res.qry.Close()
				}
			})

			b.Run(fmt.Sprintf("opt_all_series_%d_step_%s_range_0s", i, step), func(b *testing.B) {
				b.ResetTimer()
				for b.Loop() {
					res, err := queryRange(ctx, "all", queryEngine, qo, qo.queryOpts, query, qo.start, qo.end, step)
					require.NoError(b, err)
					res.qry.Close()
				}
			})
		}
	}
}

//revive:disable-next-line:cognitive-complexity // this is a benchmark
func BenchmarkRangeQueryOverTime(b *testing.B) {
	ctx := b.Context()
	qo := &querierOptimize{}
	qo.setup(
		ctx,
		b.TempDir(),
		func(err error, msgAndArgs ...any) { require.NoError(b, err, msgAndArgs) },
		time.UnixMilli(defaultStartMs),
		defaultStep,
		defaultCountOfSteps,
	)
	defer qo.close()
	querier.IsPossibleToOptimize = func(
		[]*cppbridge.LSSQueryResult,
		*prom_storage.SelectHints,
		int64, int64,
	) func() bool {
		return func() bool {
			return true
		}
	}

	queryEngine := promql.NewEngine(promql.EngineOpts{
		Logger:                   log.NewNopLogger(),
		MaxSamples:               100000,
		Timeout:                  10 * time.Second,
		LookbackDelta:            qo.lookbackDelta,
		NoStepSubqueryIntervalFn: func(int64) int64 { return qo.lookbackDelta.Milliseconds() },
		EnableAtModifier:         true,
		EnableNegativeOffset:     true,
	})

	queryf := "max_over_time(counter_metric[%s])"
	steps := []time.Duration{
		qo.step * 25 / 10,
		qo.step * 26 / 10,
		qo.step * 27 / 10,
		qo.step * 28 / 10,
		qo.step * 29 / 10,
		qo.step * 30 / 10,
		qo.step * 31 / 10,
		qo.step * 32 / 10,
		qo.step * 33 / 10,
		qo.step * 34 / 10,
		qo.step * 35 / 10,
	}
	ranges := []time.Duration{
		qo.step * 27 / 10,
		qo.step * 28 / 10,
		qo.step * 29 / 10,
		qo.step * 30 / 10,
		qo.step * 31 / 10,
		qo.step * 32 / 10,
		qo.step * 33 / 10,
		qo.step * 34 / 10,
		qo.step * 35 / 10,
		qo.step * 36 / 10,
		qo.step * 37 / 10,
		qo.step * 38 / 10,
		qo.step * 39 / 10,
		qo.step * 40 / 10,
		qo.step * 41 / 10,
		qo.step * 42 / 10,
		qo.step * 43 / 10,
		qo.step * 44 / 10,
		qo.step * 45 / 10,
		qo.step * 46 / 10,
		qo.step * 47 / 10,
		qo.step * 48 / 10,
		qo.step * 49 / 10,
		qo.step * 50 / 10,
		qo.step * 51 / 10,
		qo.step * 52 / 10,
		qo.step * 53 / 10,
		qo.step * 54 / 10,
		qo.step * 55 / 10,
		qo.step * 56 / 10,
		qo.step * 57 / 10,
		qo.step * 58 / 10,
		qo.step * 59 / 10,
		qo.step * 60 / 10,
		qo.step * 61 / 10,
		qo.step * 62 / 10,
		qo.step * 63 / 10,
		qo.step * 64 / 10,
		qo.step * 65 / 10,
		qo.step * 66 / 10,
		qo.step * 67 / 10,
		qo.step * 68 / 10,
		qo.step * 69 / 10,
		qo.step * 70 / 10,
	}

	series := 6
	qo.fillHeadWithCounter(ctx, 0, series)

	// 3 - default series for counter_metric
	for i := series + 3; i < 10; i++ {
		qo.fillHeadWithCounter(ctx, i, 1)
		for _, step := range steps {
			for _, r := range ranges {
				query := fmt.Sprintf(queryf, r)
				b.Run(fmt.Sprintf("opt_none_series_%d_step_%s_range_%s", i, step, r), func(b *testing.B) {
					b.ResetTimer()
					for b.Loop() {
						res, err := queryRange(ctx, "none", queryEngine, qo, qo.queryOpts, query, qo.start, qo.end, step)
						require.NoError(b, err)
						res.qry.Close()
					}
				})

				b.Run(fmt.Sprintf("opt_all_series_%d_step_%s_range_%s", i, step, r), func(b *testing.B) {
					b.ResetTimer()
					for b.Loop() {
						res, err := queryRange(ctx, "all", queryEngine, qo, qo.queryOpts, query, qo.start, qo.end, step)
						require.NoError(b, err)
						res.qry.Close()
					}
				})
			}
		}
	}
}

func BenchmarkInstantQuerySum(b *testing.B) {
	ctx := b.Context()
	qo := &querierOptimize{}
	qo.setup(
		ctx,
		b.TempDir(),
		func(err error, msgAndArgs ...any) { require.NoError(b, err, msgAndArgs) },
		time.UnixMilli(defaultStartMs),
		defaultStep,
		defaultCountOfSteps,
	)
	defer qo.close()
	querier.IsPossibleToOptimize = func(
		[]*cppbridge.LSSQueryResult,
		*prom_storage.SelectHints,
		int64, int64,
	) func() bool {
		return func() bool {
			return true
		}
	}

	queryEngine := promql.NewEngine(promql.EngineOpts{
		Logger:                   log.NewNopLogger(),
		MaxSamples:               100000,
		Timeout:                  10 * time.Second,
		LookbackDelta:            qo.lookbackDelta,
		NoStepSubqueryIntervalFn: func(int64) int64 { return qo.lookbackDelta.Milliseconds() },
		EnableAtModifier:         true,
		EnableNegativeOffset:     true,
	})

	query := "sum(counter_metric)"
	shifts := []time.Duration{
		qo.step * 4,   // 240s
		qo.step * 60,  // 1800s
		qo.step * 125, // 3750s
	}

	series := 10
	qo.fillHeadWithCounter(ctx, 0, series)

	// 3 - default series for counter_metric
	for i := series + 3; i < 18; i++ {
		qo.fillHeadWithCounter(ctx, i, 1)
		for _, shift := range shifts {
			b.Run(fmt.Sprintf("opt_none_series_%d_shift_%s_range_0s", i, shift), func(b *testing.B) {
				b.ResetTimer()
				for b.Loop() {
					res, err := queryInstant(ctx, "none", queryEngine, qo, qo.queryOpts, query, qo.start.Add(shift))
					require.NoError(b, err)
					res.qry.Close()
				}
			})

			b.Run(fmt.Sprintf("opt_all_series_%d_shift_%s_range_0s", i, shift), func(b *testing.B) {
				b.ResetTimer()
				for b.Loop() {
					res, err := queryInstant(ctx, "all", queryEngine, qo, qo.queryOpts, query, qo.start.Add(shift))
					require.NoError(b, err)
					res.qry.Close()
				}
			})
		}
	}
}

func BenchmarkInstantQuerySumBy(b *testing.B) {
	ctx := b.Context()
	qo := &querierOptimize{}
	qo.setup(
		ctx,
		b.TempDir(),
		func(err error, msgAndArgs ...any) { require.NoError(b, err, msgAndArgs) },
		time.UnixMilli(defaultStartMs),
		defaultStep,
		defaultCountOfSteps,
	)
	defer qo.close()
	querier.IsPossibleToOptimize = func(
		[]*cppbridge.LSSQueryResult,
		*prom_storage.SelectHints,
		int64, int64,
	) func() bool {
		return func() bool {
			return true
		}
	}

	queryEngine := promql.NewEngine(promql.EngineOpts{
		Logger:                   log.NewNopLogger(),
		MaxSamples:               100000,
		Timeout:                  10 * time.Second,
		LookbackDelta:            qo.lookbackDelta,
		NoStepSubqueryIntervalFn: func(int64) int64 { return qo.lookbackDelta.Milliseconds() },
		EnableAtModifier:         true,
		EnableNegativeOffset:     true,
	})

	// query := "sum(counter_metric)"
	query := "sum by(value) (counter_metric)"
	shifts := []time.Duration{
		qo.step * 4,   // 240s
		qo.step * 60,  // 1800s
		qo.step * 125, // 3750s
	}

	series := 6
	qo.fillHeadWithCounter(ctx, 0, series)

	// 3 - default series for counter_metric
	for i := series + 3; i < 14; i++ {
		qo.fillHeadWithCounter(ctx, i, 1)
		for _, shift := range shifts {
			b.Run(fmt.Sprintf("opt_none_series_%d_shift_%s_range_0s", i, shift), func(b *testing.B) {
				b.ResetTimer()
				for b.Loop() {
					res, err := queryInstant(ctx, "none", queryEngine, qo, qo.queryOpts, query, qo.start.Add(shift))
					require.NoError(b, err)
					res.qry.Close()
				}
			})

			b.Run(fmt.Sprintf("opt_all_series_%d_shift_%s_range_0s", i, shift), func(b *testing.B) {
				b.ResetTimer()
				for b.Loop() {
					res, err := queryInstant(ctx, "all", queryEngine, qo, qo.queryOpts, query, qo.start.Add(shift))
					require.NoError(b, err)
					res.qry.Close()
				}
			})
		}
	}
}

//revive:disable-next-line:cognitive-complexity // this is a benchmark
func BenchmarkInstantQueryOverTime(b *testing.B) {
	ctx := b.Context()
	qo := &querierOptimize{}
	qo.setup(
		ctx,
		b.TempDir(),
		func(err error, msgAndArgs ...any) { require.NoError(b, err, msgAndArgs) },
		time.UnixMilli(defaultStartMs),
		defaultStep,
		defaultCountOfSteps,
	)
	defer qo.close()
	querier.IsPossibleToOptimize = func(
		[]*cppbridge.LSSQueryResult,
		*prom_storage.SelectHints,
		int64, int64,
	) func() bool {
		return func() bool {
			return true
		}
	}

	queryEngine := promql.NewEngine(promql.EngineOpts{
		Logger:                   log.NewNopLogger(),
		MaxSamples:               100000,
		Timeout:                  10 * time.Second,
		LookbackDelta:            qo.lookbackDelta,
		NoStepSubqueryIntervalFn: func(int64) int64 { return qo.lookbackDelta.Milliseconds() },
		EnableAtModifier:         true,
		EnableNegativeOffset:     true,
	})

	queryf := "max_over_time(counter_metric[%s])"
	shifts := []time.Duration{
		qo.step,
		qo.step * 2,
		qo.step * 3,
		qo.step * 4,
		qo.step * 5,
		qo.step * 6,
		qo.step * 7,
		qo.step * 8,
		qo.step * 9,
		qo.step * 10,
		qo.step * 11,
		qo.step * 12,
		qo.step * 62,
		// qo.step * 125,
	}
	ranges := []time.Duration{
		qo.step / 2,
		qo.step,
		qo.step * 15 / 10,
		qo.step * 2,
		qo.step * 25 / 10,
		qo.step * 3,
		qo.step * 35 / 10,
		qo.step * 4,
		qo.step * 45 / 10,
		qo.step * 5,
		qo.step * 55 / 10,
		qo.step * 6,
		qo.step * 65 / 10,
		qo.step * 7,
		qo.step * 75 / 10,
		qo.step * 8,
		qo.step * 85 / 10,
		qo.step * 9,
		qo.step * 95 / 10,
		qo.step * 10,
		qo.step * 20,
		qo.step * 30,
		qo.step * 40,
		qo.step * 60,
	}

	series := 6
	qo.fillHeadWithCounter(ctx, 0, series)

	// 3 - default series for counter_metric
	for i := series + 3; i < 11; i++ {
		qo.fillHeadWithCounter(ctx, i, 1)
		for _, shift := range shifts {
			for _, r := range ranges {
				query := fmt.Sprintf(queryf, r)

				b.Run(fmt.Sprintf("opt_none_series_%d_shift_%s_range_%s", i, shift, r), func(b *testing.B) {
					b.ResetTimer()
					for b.Loop() {
						res, err := queryInstant(ctx, "none", queryEngine, qo, qo.queryOpts, query, qo.start.Add(shift))
						require.NoError(b, err)
						res.qry.Close()
					}
				})

				b.Run(fmt.Sprintf("opt_all_series_%d_shift_%s_range_%s", i, shift, r), func(b *testing.B) {
					b.ResetTimer()
					for b.Loop() {
						res, err := queryInstant(ctx, "all", queryEngine, qo, qo.queryOpts, query, qo.start.Add(shift))
						require.NoError(b, err)
						res.qry.Close()
					}
				})
			}
		}
	}
}
