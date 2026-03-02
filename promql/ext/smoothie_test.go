package promqlext_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/promql"
	promqlext "github.com/prometheus/prometheus/promql/ext"
	"github.com/prometheus/prometheus/util/teststorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	promqlext.RegisterOPSmoothie()
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
		time.Duration(time.Minute),
	)
	require.NoError(t, err)

	result := query.Exec(context.Background())
	require.NoError(t, result.Err)
	m := result.Value.(promql.Matrix)
	require.Len(t, m, 1)
	assert.Equal(t, ls, m[0].Metric)
	assert.Len(t, m[0].Floats, 5)
	assert.EqualValues(t, m[0].Floats[0].T, 0)
	assert.EqualValues(t, m[0].Floats[0].F, 0)
	assert.EqualValues(t, m[0].Floats[1].T, 1*minute)
	assert.EqualValues(t, m[0].Floats[1].F, 0.5)
	assert.EqualValues(t, m[0].Floats[2].T, 2*minute)
	assert.EqualValues(t, m[0].Floats[2].F, 1)
	assert.EqualValues(t, m[0].Floats[3].T, 3*minute)
	assert.EqualValues(t, m[0].Floats[3].F, 1.5)
	assert.EqualValues(t, m[0].Floats[4].T, 4*minute)
	assert.EqualValues(t, m[0].Floats[4].F, 2)
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

	promqlext.RegisterOPSmoothie()
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
	okQuery, okErr := engine.NewRangeQuery(
		ctx,
		storage,
		nil,
		"ok_smoothie(should_be_kept[5m])",
		timestamp.Time(0),
		timestamp.Time(10*minute),
		time.Minute,
	)
	opResult := opQuery.Exec(context.Background())
	okResult := okQuery.Exec(context.Background())
	require.NoError(t, opErr)
	require.NoError(t, okErr)
	require.NoError(t, opResult.Err)
	require.NoError(t, okResult.Err)
	require.Equal(t, opResult, okResult)
}
