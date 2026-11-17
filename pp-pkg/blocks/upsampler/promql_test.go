package upsampler_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp-pkg/blocks/upsampler"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
)

func TestOverTimePromQLApproximation(t *testing.T) {
	engine := promql.NewEngine(promql.EngineOpts{
		MaxSamples: 1000,
		Timeout:    10 * time.Second,
	})

	for _, tc := range []struct {
		name         string
		expression   string
		without      float64
		withNonempty float64
		withEmpty    float64
	}{
		{"count", "count_over_time(test_metric[2m])", 1, 2, 2},
		{"sum", "sum_over_time(test_metric[2m])", 30, 40, 20},
		{"avg", "avg_over_time(test_metric[2m])", 30, 20, 10},
		{"min", "min_over_time(test_metric[2m])", 30, 10, 10},
		{"max", "max_over_time(test_metric[2m])", 30, 30, 10},
		{"quantile", "quantile_over_time(0.5, test_metric[2m])", 30, 20, 10},
		{"stddev", "stddev_over_time(test_metric[2m])", 0, 10, 0},
		{"stdvar", "stdvar_over_time(test_metric[2m])", 0, 100, 0},
		{"last", "last_over_time(test_metric[2m])", 30, 30, 10},
		{"present", "present_over_time(test_metric[2m])", 1, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, scenario := range []struct {
				name     string
				upsample bool
				want     []promql.FPoint
			}{
				{"without", false, []promql.FPoint{{T: 300_000, F: tc.without}}},
				{"with", true, []promql.FPoint{
					{T: 240_000, F: tc.withEmpty},
					{T: 300_000, F: tc.withNonempty},
				}},
			} {
				t.Run(scenario.name, func(t *testing.T) {
					// Arrange
					queryable := storage.QueryableFunc(func(int64, int64) (storage.Querier, error) {
						base := &mockQuerier{
							selectFunc: func(context.Context, bool, *storage.SelectHints, ...*labels.Matcher) storage.SeriesSet {
								return newSingleSeriesSet(newMockIterator([]struct {
									t int64
									v float64
								}{
									{60_000, 10},
									{300_000, 30},
								}))
							},
						}

						if scenario.upsample {
							return upsampler.NewQuerier(base, (2 * time.Minute).Milliseconds()), nil
						}

						return base, nil
					})
					ctx := context.Background()
					query, err := engine.NewRangeQuery(
						ctx, queryable, nil, tc.expression, time.Unix(240, 0), time.Unix(300, 0), time.Minute,
					)
					require.NoError(t, err)
					defer query.Close()

					// Act
					result := query.Exec(ctx)
					matrix, err := result.Matrix()

					// Assert
					require.NoError(t, err)
					require.Len(t, matrix, 1)
					require.Equal(t, scenario.want, matrix[0].Floats)
				})
			}
		})
	}
}
