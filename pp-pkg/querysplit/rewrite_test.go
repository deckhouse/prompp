package querysplit

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/promql/parser"
)

func TestResolveStartEnd(t *testing.T) {
	for _, tc := range []struct {
		query    string
		expected string
	}{
		{query: `x @ start()`, expected: `x @ 1000.000`},
		{query: `rate(x[5m] @ end())`, expected: `rate(x[5m] @ 2000.000)`},
		{query: `max_over_time(x[1h:1m] @ end())`, expected: `max_over_time(x[1h:1m] @ 2000.000)`},
		{query: `x @ 1500 + y`, expected: `x @ 1500.000 + y`},
		{query: `rate(x[5m])`, expected: `rate(x[5m])`},
	} {
		t.Run(tc.query, func(t *testing.T) {
			expr, err := parser.ParseExpr(tc.query)
			require.NoError(t, err)

			resolveStartEnd(expr, 1_000_000, 2_000_000)

			require.Equal(t, tc.expected, expr.String())
			reparsed, err := parser.ParseExpr(expr.String())
			require.NoError(t, err)
			require.Equal(t, tc.expected, reparsed.String())
		})
	}
}
