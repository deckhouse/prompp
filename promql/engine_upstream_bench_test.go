package promql_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/teststorage"
)

// BenchmarkEngineUpstream includes query construction and cleanup to account for per-query
// signature preparation as well as matching and function evaluation across steps.
func BenchmarkEngineUpstream(b *testing.B) {
	ctx := context.Background()
	engine := promql.NewEngine(promql.EngineOpts{
		MaxSamples:           10_000_000,
		Timeout:              time.Minute,
		EnableAtModifier:     true,
		EnableNegativeOffset: true,
	})
	b.Cleanup(func() { require.NoError(b, engine.Close()) })

	for _, seriesCount := range []int{10, 1000} {
		b.Run(fmt.Sprintf("series=%d", seriesCount), func(b *testing.B) {
			stor := teststorage.New(b)
			stor.DB.DisableCompactions()
			b.Cleanup(func() { require.NoError(b, stor.Close()) })
			app := stor.Appender(ctx)

			for i := range seriesCount {
				for _, name := range []string{"lhs", "rhs", "group_rhs"} {
					if name == "group_rhs" && i%4 != 0 {
						continue
					}

					metric := labels.FromStrings(
						"__name__", name, "instance", strconv.Itoa(i),
						"join", strconv.Itoa(i/4), "extra", "value",
					)
					var ref storage.SeriesRef

					for step := range 125 {
						var err error
						ref, err = app.Append(ref, metric, int64(step)*10_000, float64((step+i)%120))
						require.NoError(b, err)
					}
				}
			}

			require.NoError(b, app.Commit())
			cases := []struct {
				name string
				expr string
			}{
				{"one_to_one", "lhs + rhs"},
				{"bool", "lhs == bool rhs"},
				{"group_left", "lhs * on(join) group_left(extra) group_rhs"},
				{"group_right", "group_rhs * on(join) group_right(extra) lhs"},
				{"and", "lhs and rhs{instance=~'.*[02468]'}"},
				{"or", "lhs or rhs{instance=~'.*[02468]'}"},
				{"unless", "lhs unless rhs{instance=~'.*[02468]'}"},
				{"changing_matches", "(lhs > 60) + (rhs < 90)"},
				{"abs", "abs(lhs)"},
				{"clamp", "clamp(lhs, (10), (90))"},
				{"rate", "rate(lhs[1m])"},
				{"constant_matrix", "rate(lhs[1m] @ 1200)"},
				{"parentheses", "sum((abs(((lhs))))) + (1 + 2)"},
			}

			for _, tc := range cases {
				b.Run(tc.name, func(b *testing.B) {
					for _, steps := range []int{1, 120} {
						b.Run(fmt.Sprintf("steps=%d", steps), func(b *testing.B) {
							b.ReportAllocs()
							b.ResetTimer()

							for i := 0; i < b.N; i++ {
								query, err := engine.NewRangeQuery(ctx, stor, nil, tc.expr,
									time.Unix(int64(125-steps)*10, 0), time.Unix(1240, 0), 10*time.Second)
								if err != nil {
									b.Fatal(err)
								}

								result := query.Exec(ctx)
								query.Close()

								if result.Err != nil {
									b.Fatal(result.Err)
								}
							}
						})
					}
				})
			}
		})
	}
}

// BenchmarkEnginePreprocess measures parsing and preprocessing, which run once per query.
func BenchmarkEnginePreprocess(b *testing.B) {
	for _, expr := range []string{
		"1", "1 + 2", "clamp(lhs, (10), (90))", "rate(lhs[1m] @ 1200)",
		"sum((abs(((lhs))))) + (1 + 2)", "topk((5), lhs)",
	} {
		b.Run(expr, func(b *testing.B) {
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				parsed, err := parser.ParseExpr(expr)
				if err != nil {
					b.Fatal(err)
				}

				promql.PreprocessExpr(parsed, time.Unix(0, 0), time.Unix(1200, 0))
			}
		})
	}
}
