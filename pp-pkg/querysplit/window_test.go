package querysplit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/promql/parser"
)

func TestLookbackWindow(t *testing.T) {
	for _, tc := range []struct {
		name     string
		query    string
		expected time.Duration
	}{
		{name: "instant selector", query: `x`, expected: 5 * time.Minute},
		{name: "range selector", query: `rate(x[10m])`, expected: 10 * time.Minute},
		{name: "subquery", query: `max_over_time(rate(x[5m])[1h:1m])`, expected: time.Hour + 5*time.Minute},
		{name: "offset", query: `rate(x[5m] offset 1h)`, expected: time.Hour + 5*time.Minute},
		{name: "negative offset", query: `rate(x[5m] offset -1h)`, expected: 5 * time.Minute},
		{name: "subquery offset", query: `max_over_time(x[1h:1m] offset 1d)`, expected: 25*time.Hour + 5*time.Minute},
		{name: "@ modifier", query: `rate(x[5m]) + max_over_time(y[1d] @ 1700000000)`, expected: 5 * time.Minute},
		{name: "@ end()", query: `x @ end()`, expected: 0},
		{name: "@ on subquery", query: `max_over_time(x[1d:1m] @ start())`, expected: 0},
		{name: "widest selector", query: `x + rate(y[1h])`, expected: time.Hour},
		{name: "no selectors", query: `time()`, expected: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expr, err := parser.ParseExpr(tc.query)
			require.NoError(t, err)

			require.Equal(t, tc.expected, lookbackWindow(expr, 5*time.Minute))
		})
	}
}

func TestHasVaryingAggregationParam(t *testing.T) {
	for _, tc := range []struct {
		query    string
		expected bool
	}{
		{query: `sum(x)`, expected: false},
		{query: `topk(5, x)`, expected: false},
		{query: `quantile(-(0.5 + 0.4), x)`, expected: false},
		{query: `count_values("v", x)`, expected: false},
		{query: `topk(scalar(y), x)`, expected: true},
		{query: `quantile(time() % 17 / 17, x)`, expected: true},
		{query: `sum(topk(scalar(y), x))`, expected: true},
	} {
		t.Run(tc.query, func(t *testing.T) {
			expr, err := parser.ParseExpr(tc.query)
			require.NoError(t, err)

			require.Equal(t, tc.expected, hasVaryingAggregationParam(expr))
		})
	}
}
