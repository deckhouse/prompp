package promql_test

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/util/teststorage"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// OP_FUNCTIONS

type BaseSuite struct {
	suite.Suite

	engine  *promql.Engine
	storage *teststorage.TestStorage
}

func (s *BaseSuite) SetupSuite() {
	opts := promql.EngineOpts{
		Logger:        nil,
		Reg:           nil,
		MaxSamples:    10000,
		Timeout:       10 * time.Second,
		LookbackDelta: 59 * time.Second,
	}
	s.engine = promql.NewEngine(opts)
}

func (s *BaseSuite) SetupTest() {
	s.storage = teststorage.New(s.T())
}

func (s *BaseSuite) TearDownTest() {
	_ = s.storage.Close()
}

func (s *BaseSuite) fillStorageByStrings(records ...string) {
	a := s.storage.Appender(context.Background())
	var (
		ts       int64
		value    float64
		labelSet string
	)
	for i := range records {
		_, err := fmt.Sscanf(records[i], "%d %f %s", &ts, &value, &labelSet)
		s.Require().NoError(err)
		var labelPairs []string
		for _, pair := range strings.Split(labelSet, ",") {
			labelPairs = append(labelPairs, strings.Split(pair, "=")...)
		}
		_, _ = a.Append(0, labels.FromStrings(labelPairs...), ts*60*1000, value)
	}
	s.Require().NoError(a.Commit())
}

func (s *BaseSuite) time(duration string) time.Time {
	d, err := time.ParseDuration(duration)
	s.Require().NoError(err)
	return timestamp.Time(int64(d / time.Millisecond))
}

func (s *BaseSuite) checkMatrix(res *promql.Result, expected map[string][]float64) {
	actual, err := res.Matrix()
	s.Require().NoError(err)
	for _, series := range actual {
		values := make([]float64, len(series.Floats))
		for i := range values {
			values[i] = series.Floats[i].F
		}
		key := series.Metric.String()
		s.Equalf(expected[key], values, "series %s mismatch", key)
		delete(expected, key)
	}
	s.Empty(expected)
}

func TestDefined(t *testing.T) {
	suite.Run(t, new(DefinedSuite))
}

type DefinedSuite struct {
	BaseSuite
}

func (s *DefinedSuite) SetupSuite() {
	promql.RegisterOPDefined()
	s.BaseSuite.SetupSuite()
}

func (s *DefinedSuite) TestSingle() {
	ctx := context.Background()

	s.fillStorageByStrings(
		"1 5 __name__=foo,server=foo",
	)
	expected := `{server="foo"} => 0 @[180000]`

	query, err := s.engine.NewInstantQuery(
		ctx,
		s.storage,
		nil,
		"op_defined(foo[3m])",
		s.time("3m"),
	)
	s.Require().NoError(err)

	result := query.Exec(context.Background())
	s.Require().NoError(result.Err)
	s.Equal(expected, result.String())
}

func (s *DefinedSuite) TestMultiple() {
	ctx := context.Background()

	s.fillStorageByStrings(
		"0 5 __name__=foo,server=apple",
		"0 5 __name__=foo,server=orange",
		// "1 5 __name__=foo,server=apple",
		"1 5 __name__=foo,server=orange",
		// "2 5 __name__=foo,server=apple",
		"2 5 __name__=foo,server=orange",
		// "3 5 __name__=foo,server=apple",
		"3 5 __name__=foo,server=orange",
		"4 5 __name__=foo,server=apple",
	)
	query, err := s.engine.NewRangeQuery(
		ctx,
		s.storage,
		nil,
		"op_defined(foo[10m])",
		s.time("2m"),
		s.time("6m"),
		time.Minute,
	)
	s.Require().NoError(err)

	result := query.Exec(context.Background())
	s.Require().NoError(result.Err)
	s.checkMatrix(result, map[string][]float64{
		// Minutest           2  3  4  5  6
		`{server="apple"}`:  {0, 0, 1, 1, 0},
		`{server="orange"}`: {1, 1, 1, 0, 0},
	})
}

func (s *DefinedSuite) TestBackwardCompatibility() {
	ctx := context.Background()

	s.fillStorageByStrings(
		"0 5 __name__=foo,server=apple",
		"0 5 __name__=foo,server=orange",
		"1 5 __name__=foo,server=orange",
		"2 5 __name__=foo,server=orange",
		"3 5 __name__=foo,server=orange",
		"4 5 __name__=foo,server=apple",
	)

	opQuery, opErr := s.engine.NewRangeQuery(
		ctx,
		s.storage,
		nil,
		"op_defined(foo[10m])",
		s.time("2m"),
		s.time("6m"),
		time.Minute,
	)
	opResult := opQuery.Exec(context.Background())
	s.Require().NoError(opErr)
	s.Require().NoError(opResult.Err)
}

func TestReplaceNaN(t *testing.T) {
	suite.Run(t, new(ReplaceNaNSuite))
}

type ReplaceNaNSuite struct {
	BaseSuite
}

func (s *ReplaceNaNSuite) SetupSuite() {
	promql.RegisterOPReplaceNaN()
	s.BaseSuite.SetupSuite()
}

func (s *ReplaceNaNSuite) TestSingle() {
	ctx := context.Background()

	s.fillStorageByStrings(
		"1 5 __name__=foo,server=foo",
	)
	expected := `{server="foo"} => 17 @[180000]`

	query, err := s.engine.NewInstantQuery(
		ctx,
		s.storage,
		nil,
		"op_replace_nan(foo[3m], 17)",
		s.time("3m"),
	)
	s.Require().NoError(err)

	result := query.Exec(context.Background())
	s.Require().NoError(result.Err)
	s.Equal(expected, result.String())
}

func (s *ReplaceNaNSuite) TestMinimal() {
	ctx := context.Background()

	s.fillStorageByStrings(
		"1 5 __name__=foo,server=foo",
	)

	query, err := s.engine.NewInstantQuery(
		ctx,
		s.storage,
		nil,
		"op_replace_nan(foo[3m], 17, 60001)",
		s.time("3m"),
	)
	s.Require().NoError(err)

	result := query.Exec(context.Background())
	s.Require().NoError(result.Err)
	s.Zero(result.String())
}

func (s *ReplaceNaNSuite) TestMultiple() {
	ctx := context.Background()

	s.fillStorageByStrings(
		"0 5 __name__=foo,server=apple",
		"0 5 __name__=foo,server=orange",
		// "1 5 __name__=foo,server=apple",
		"1 0 __name__=foo,server=orange",
		// "2 5 __name__=foo,server=apple",
		"2 -42 __name__=foo,server=orange",
		// "3 5 __name__=foo,server=apple",
		"3 5 __name__=foo,server=orange",
		"4 3 __name__=foo,server=apple",
	)
	query, err := s.engine.NewRangeQuery(
		ctx,
		s.storage,
		nil,
		"op_replace_nan(foo[10m], 17)",
		s.time("2m"),
		s.time("6m"),
		time.Minute,
	)
	s.Require().NoError(err)

	result := query.Exec(context.Background())
	s.Require().NoError(result.Err)
	s.checkMatrix(result, map[string][]float64{
		// Minutes             2   3  4  5  6
		`{server="apple"}`:  {17, 17, 3, 3, 17},
		`{server="orange"}`: {-42, 5, 5, 17, 17},
	})
}

func (s *ReplaceNaNSuite) TestBackwardCompatibility() {
	ctx := context.Background()

	s.fillStorageByStrings(
		"0 5 __name__=foo,server=apple",
		"0 5 __name__=foo,server=orange",
		"1 0 __name__=foo,server=orange",
		"2 -42 __name__=foo,server=orange",
		"3 5 __name__=foo,server=orange",
		"4 3 __name__=foo,server=apple",
	)

	opQuery, opErr := s.engine.NewRangeQuery(
		ctx,
		s.storage,
		nil,
		"op_replace_nan(foo[10m], 17)",
		s.time("2m"),
		s.time("6m"),
		time.Minute,
	)

	opResult := opQuery.Exec(context.Background())

	s.Require().NoError(opErr)
	s.Require().NoError(opResult.Err)
}

func TestSmoothie(t *testing.T) {
	ctx := context.Background()

	storage := teststorage.New(t)
	a := storage.Appender(context.Background())
	ls := labels.FromStrings(labels.MetricName, "should_be_kept", "key", "value")
	minute := int64(time.Minute / time.Millisecond)
	_, _ = a.Append(0, ls, 0*minute, 0)
	_, _ = a.Append(0, ls, 1*minute, 1)
	_, _ = a.Append(0, ls, 2*minute, 2)
	_, _ = a.Append(0, ls, 3*minute, 3)
	_, _ = a.Append(0, ls, 4*minute, 4)
	_, _ = a.Append(0, ls, 5*minute, math.Float64frombits(value.StaleNaN))
	_ = a.Commit()

	promql.RegisterOPSmoothie()
	opts := promql.EngineOpts{
		Logger:        nil,
		Reg:           nil,
		MaxSamples:    10000,
		Timeout:       10 * time.Second,
		LookbackDelta: 5 * time.Minute,
	}
	engine := promql.NewEngine(opts)

	query, err := engine.NewRangeQuery(
		ctx,
		storage,
		nil,
		"op_smoothie(should_be_kept[5m])",
		timestamp.Time(0),
		timestamp.Time(10*minute),
		time.Minute,
	)
	require.NoError(t, err)

	result := query.Exec(context.Background())
	require.NoError(t, result.Err)
	m := result.Value.(promql.Matrix)
	require.Len(t, m, 1)
	assert.Equal(t, ls, m[0].Metric)
	assert.Len(t, m[0].Floats, 5)
	assert.EqualValues(t, 0, m[0].Floats[0].T)
	assert.EqualValues(t, 0, m[0].Floats[0].F)
	assert.EqualValues(t, 1*minute, m[0].Floats[1].T)
	assert.EqualValues(t, 0.5, m[0].Floats[1].F)
	assert.EqualValues(t, 2*minute, m[0].Floats[2].T)
	assert.EqualValues(t, 1, m[0].Floats[2].F)
	assert.EqualValues(t, 3*minute, m[0].Floats[3].T)
	assert.EqualValues(t, 1.5, m[0].Floats[3].F)
	assert.EqualValues(t, 4*minute, m[0].Floats[4].T)
	assert.EqualValues(t, 2, m[0].Floats[4].F)
}

func TestSmoothieBackwardCompatibility(t *testing.T) {
	ctx := context.Background()

	storage := teststorage.New(t)
	a := storage.Appender(context.Background())
	ls := labels.FromStrings(labels.MetricName, "should_be_kept", "key", "value")
	minute := int64(time.Minute / time.Millisecond)
	_, _ = a.Append(0, ls, 0*minute, 0)
	_, _ = a.Append(0, ls, 1*minute, 1)
	_, _ = a.Append(0, ls, 2*minute, 2)
	_, _ = a.Append(0, ls, 3*minute, 3)
	_, _ = a.Append(0, ls, 4*minute, 4)
	_, _ = a.Append(0, ls, 5*minute, math.Float64frombits(value.StaleNaN))
	_ = a.Commit()

	promql.RegisterOPSmoothie()
	opts := promql.EngineOpts{
		Logger:        nil,
		Reg:           nil,
		MaxSamples:    10000,
		Timeout:       10 * time.Second,
		LookbackDelta: 5 * time.Minute,
	}
	engine := promql.NewEngine(opts)

	opQuery, opErr := engine.NewRangeQuery(
		ctx,
		storage,
		nil,
		"op_smoothie(should_be_kept[5m])",
		timestamp.Time(0),
		timestamp.Time(10*minute),
		time.Minute,
	)

	opResult := opQuery.Exec(context.Background())
	require.NoError(t, opErr)
	require.NoError(t, opResult.Err)
}

func TestZeroIfNone(t *testing.T) {
	suite.Run(t, new(ZeroIfNoneSuite))
}

type ZeroIfNoneSuite struct {
	BaseSuite
}

func (s *ZeroIfNoneSuite) SetupSuite() {
	promql.RegisterOPZeroIfNone()
	s.BaseSuite.SetupSuite()
}

func (s *ZeroIfNoneSuite) TestSingle() {
	ctx := context.Background()

	s.fillStorageByStrings(
		"1 5 __name__=foo,server=foo",
	)
	expected := `{server="foo"} => 0 @[180000]`

	query, err := s.engine.NewInstantQuery(
		ctx,
		s.storage,
		nil,
		"op_zero_if_none(foo[3m])",
		s.time("3m"),
	)
	s.Require().NoError(err)

	result := query.Exec(context.Background())
	s.Require().NoError(result.Err)
	s.Equal(expected, result.String())
}

func (s *ZeroIfNoneSuite) TestMinimal() {
	ctx := context.Background()

	s.fillStorageByStrings(
		"1 5 __name__=foo,server=foo",
	)

	query, err := s.engine.NewInstantQuery(
		ctx,
		s.storage,
		nil,
		"op_zero_if_none(foo[3m], 60001)",
		s.time("3m"),
	)
	s.Require().NoError(err)

	result := query.Exec(context.Background())
	s.Require().NoError(result.Err)
	s.Zero(result.String())
}

func (s *ZeroIfNoneSuite) TestMultiple() {
	ctx := context.Background()

	s.fillStorageByStrings(
		"0 5 __name__=foo,server=apple",
		"0 5 __name__=foo,server=orange",
		// "1 5 __name__=foo,server=apple",
		"1 0 __name__=foo,server=orange",
		// "2 5 __name__=foo,server=apple",
		"2 -42 __name__=foo,server=orange",
		// "3 5 __name__=foo,server=apple",
		"3 5 __name__=foo,server=orange",
		"4 3 __name__=foo,server=apple",
	)
	query, err := s.engine.NewRangeQuery(
		ctx,
		s.storage,
		nil,
		"op_zero_if_none(foo[10m])",
		s.time("2m"),
		s.time("6m"),
		time.Minute,
	)
	s.Require().NoError(err)

	result := query.Exec(context.Background())
	s.Require().NoError(result.Err)
	s.checkMatrix(result, map[string][]float64{
		// Minutest           2  3  4  5  6
		`{server="apple"}`:  {0, 0, 3, 3, 0},
		`{server="orange"}`: {-42, 5, 5, 0, 0},
	})
}

func (s *ZeroIfNoneSuite) TestBackwardCompatibility() {
	ctx := context.Background()

	s.fillStorageByStrings(
		"0 5 __name__=foo,server=apple",
		"0 5 __name__=foo,server=orange",
		"1 0 __name__=foo,server=orange",
		"2 -42 __name__=foo,server=orange",
		"3 5 __name__=foo,server=orange",
		"4 3 __name__=foo,server=apple",
	)

	opQuery, opErr := s.engine.NewRangeQuery(
		ctx,
		s.storage,
		nil,
		"op_zero_if_none(foo[10m])",
		s.time("2m"),
		s.time("6m"),
		time.Minute,
	)

	opResult := opQuery.Exec(context.Background())

	s.Require().NoError(opErr)
	s.Require().NoError(opResult.Err)
}
