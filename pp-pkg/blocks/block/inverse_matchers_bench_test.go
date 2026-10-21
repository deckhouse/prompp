package block

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp-pkg/blocks/testutils"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/index"
)

// inverseMatcherCase pairs a workload name with its selector.
type inverseMatcherCase struct {
	name     string
	matchers []*labels.Matcher
}

// BenchmarkInverseMatchersBlock measures warm-cache selection through the Prom++ block wrapper.
// Fixture creation and result validation run outside the timer.
func BenchmarkInverseMatchersBlock(b *testing.B) {
	for _, cardinality := range []int{100, 10000} {
		b.Run(fmt.Sprintf("cardinality=%d", cardinality), func(b *testing.B) {
			series := inverseMatcherSeries(cardinality)
			dir := testutils.CreateBlock(b, b.TempDir(), series)
			block, err := OpenBlock(nil, dir, nil)
			require.NoError(b, err)
			b.Cleanup(func() { require.NoError(b, block.Close()) })
			ir, err := block.Index()
			require.NoError(b, err)
			b.Cleanup(func() { require.NoError(b, ir.Close()) })
			q, err := tsdb.NewBlockQuerier(block, 0, 450000)
			require.NoError(b, err)
			b.Cleanup(func() { require.NoError(b, q.Close()) })

			for _, tc := range inverseMatcherCases() {
				b.Run(tc.name, func(b *testing.B) {
					expected := 0
					expectedSum := 0.0

					for i, s := range series {
						if inverseMatcherMatches(s.Labels(), tc.matchers) {
							expected++
							expectedSum += float64(i * 16)
						}
					}

					p, err := tsdb.PostingsForMatchers(b.Context(), ir, tc.matchers...)
					require.NoError(b, err)
					refs, err := index.ExpandPostings(p)
					require.NoError(b, err)
					require.Len(b, refs, expected)
					count, sum, err := inverseMatcherRead(b.Context(), q, tc.matchers)
					require.NoError(b, err)
					require.Equal(b, expected*16, count)
					require.Equal(b, expectedSum, sum)

					b.Run("Build", func(b *testing.B) {
						b.ReportAllocs()
						b.ResetTimer()

						for b.Loop() {
							postings, buildErr := tsdb.PostingsForMatchers(b.Context(), ir, tc.matchers...)
							if buildErr != nil {
								b.Fatal(buildErr)
							}

							if postingsErr := postings.Err(); postingsErr != nil {
								b.Fatal(postingsErr)
							}
						}
					})
					b.Run("Select", func(b *testing.B) {
						b.ReportAllocs()
						b.ResetTimer()

						for b.Loop() {
							count, sum, err = inverseMatcherRead(b.Context(), q, tc.matchers)
							if err != nil {
								b.Fatal(err)
							}
						}

						require.Equal(b, expected*16, count)
						require.Equal(b, expectedSum, sum)
					})
				})
			}
		})
	}
}

func inverseMatcherCases() []inverseMatcherCase {
	matcher := labels.MustNewMatcher
	base := matcher(labels.MatchEqual, "__name__", "inverse_metric")
	selective := matcher(labels.MatchEqual, "group", "1")

	return []inverseMatcherCase{
		{"equal_control", []*labels.Matcher{base, matcher(labels.MatchEqual, "target", "v000001")}},
		{"negative_equal", []*labels.Matcher{base, matcher(labels.MatchNotEqual, "target", "v000001")}},
		{"nonempty", []*labels.Matcher{base, matcher(labels.MatchNotEqual, "target", "")}},
		{"empty", []*labels.Matcher{base, matcher(labels.MatchEqual, "target", "")}},
		{"negative_regex_rare", []*labels.Matcher{base, matcher(labels.MatchNotRegexp, "target", "v00000.*")}},
		{"negative_regex_common", []*labels.Matcher{base, matcher(labels.MatchNotRegexp, "target", "v00.*")}},
		{"selective_nonempty", []*labels.Matcher{base, selective, matcher(labels.MatchNotEqual, "target", "")}},
		{"missing_nonempty", []*labels.Matcher{base, matcher(labels.MatchNotEqual, "absent", "")}},
		{"regex_plus", []*labels.Matcher{base, matcher(labels.MatchRegexp, "target", ".+")}},
		{"negative_regex_plus", []*labels.Matcher{base, matcher(labels.MatchNotRegexp, "target", ".+")}},
	}
}

func inverseMatcherMatches(ls labels.Labels, matchers []*labels.Matcher) bool {
	for _, m := range matchers {
		if !m.Matches(ls.Get(m.Name)) {
			return false
		}
	}

	return true
}

func inverseMatcherRead(ctx context.Context, q storage.Querier, matchers []*labels.Matcher) (int, float64, error) {
	set := q.Select(ctx, false, nil, matchers...)
	count := 0
	sum := 0.0
	var it chunkenc.Iterator

	for set.Next() {
		it = set.At().Iterator(it)

		for it.Next() != chunkenc.ValNone {
			_, value := it.At()
			count++
			sum += value
		}

		if err := it.Err(); err != nil {
			return 0, 0, err
		}
	}

	return count, sum, set.Err()
}

func inverseMatcherSeries(cardinality int) []storage.Series {
	series := make([]storage.Series, cardinality)

	for i := range series {
		builder := labels.NewBuilder(labels.FromStrings(
			"__name__", "inverse_metric", "id", fmt.Sprintf("%06d", i), "group", strconv.Itoa(i%100),
		))

		if i%5 != 0 {
			builder.Set("target", fmt.Sprintf("v%06d", i))
		}

		samples := make([]chunks.Sample, 16)

		for j := range samples {
			samples[j] = testutils.SampleTest{TS: int64(j * 30000), V: float64(i)}
		}

		series[i] = storage.NewListSeries(builder.Labels(), samples)
	}

	return series
}
