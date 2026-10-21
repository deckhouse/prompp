package block

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp-pkg/blocks/testutils"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/util/testutil"
)

// postingsErrorReader injects iterator errors behind the block reader wrapper.
type postingsErrorReader struct {
	tsdb.IndexReader
	err error
}

// inverseMatchersSuite checks persisted-block selectors against Matcher.Matches.
type inverseMatchersSuite struct {
	suite.Suite
	series []storage.Series
	q      storage.Querier
	ir     tsdb.IndexReader
}

func TestInverseMatchersOracle(t *testing.T) {
	suite.Run(t, new(inverseMatchersSuite))
}

func (s *inverseMatchersSuite) SetupSuite() {
	for i, value := range []string{"", "a", "b", "\n", "a\nb", "\n\n", "a\n", "a.b"} {
		builder := labels.NewBuilder(labels.FromStrings("__name__", "inverse_metric", "id", strconv.Itoa(i)))
		builder.Set("target", value)
		s.series = append(s.series, storage.NewListSeries(builder.Labels(), []chunks.Sample{
			testutils.SampleTest{TS: 100, V: float64(i)},
		}))
	}
	dir := testutils.CreateBlock(s.T(), s.T().TempDir(), s.series)
	block, err := OpenBlock(nil, dir, nil)
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.Require().NoError(block.Close()) })
	s.ir, err = block.Index()
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.Require().NoError(s.ir.Close()) })
	s.q, err = tsdb.NewBlockQuerier(block, 0, 200)
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.Require().NoError(s.q.Close()) })
}

func (s *inverseMatchersSuite) TestAllLabelValues() {
	p := s.ir.PostingsForAllLabelValues(s.T().Context(), "target")
	refs, err := index.ExpandPostings(p)
	s.Require().NoError(err)
	s.Require().Len(refs, 7)
	s.True(slices.IsSorted(refs))
	s.Equal(slices.Compact(slices.Clone(refs)), refs)
	s.False(p.Next())

	p = s.ir.PostingsForAllLabelValues(s.T().Context(), "target")
	s.True(p.Seek(refs[3]))
	s.Equal(refs[3], p.At())
	s.False(p.Seek(refs[len(refs)-1] + 1))
	s.Require().NoError(p.Err())
	refs, err = index.ExpandPostings(s.ir.PostingsForAllLabelValues(s.T().Context(), "missing"))
	s.Require().NoError(err)
	s.Empty(refs)
}

func (s *inverseMatchersSuite) TestAllLabelValuesCancellation() {
	ctx, cancel := context.WithCancel(s.T().Context())
	cancel()
	s.Require().ErrorIs(s.ir.PostingsForAllLabelValues(ctx, "target").Err(), context.Canceled)

	duringRead := &testutil.MockContextErrAfter{FailAfter: 2}
	s.Require().ErrorIs(s.ir.PostingsForAllLabelValues(duringRead, "target").Err(), context.Canceled)
}

func (s *inverseMatchersSuite) TestAllLabelValuesV1() {
	ir, err := index.NewFileReader("../../../tsdb/testdata/index_format_v1/index")
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.Require().NoError(ir.Close()) })
	r := blockIndexReader{ir: ir}
	names, err := ir.LabelNames(s.T().Context())
	s.Require().NoError(err)
	s.Require().NotEmpty(names)

	for _, name := range names {
		s.Run(name, func() {
			values, valuesErr := ir.LabelValues(s.T().Context(), name)
			s.Require().NoError(valuesErr)
			p, postingsErr := ir.Postings(s.T().Context(), name, values...)
			s.Require().NoError(postingsErr)
			expected, expectedErr := index.ExpandPostings(p)
			s.Require().NoError(expectedErr)
			s.Require().NotEmpty(expected)
			actual, actualErr := index.ExpandPostings(r.PostingsForAllLabelValues(s.T().Context(), name))
			s.Require().NoError(actualErr)
			s.Equal(expected, actual)
			p = r.PostingsForAllLabelValues(s.T().Context(), name)
			s.True(p.Seek(expected[len(expected)/2]))
			s.Equal(expected[len(expected)/2], p.At())
			s.False(p.Seek(expected[len(expected)-1] + 1))
			s.Require().NoError(p.Err())
		})
	}

	refs, err := index.ExpandPostings(r.PostingsForAllLabelValues(s.T().Context(), "missing"))
	s.Require().NoError(err)
	s.Empty(refs)
	ctx, cancel := context.WithCancel(s.T().Context())
	cancel()
	s.Require().ErrorIs(r.PostingsForAllLabelValues(ctx, "missing").Err(), context.Canceled)
}

func (s *inverseMatchersSuite) TestSelectorErrors() {
	failure := errors.New("index read failed")
	r := blockIndexReader{ir: postingsErrorReader{
		IndexReader: s.ir,
		err:         fmt.Errorf("decode postings: %w", failure),
	}}

	for _, matcher := range []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, "target", ""),
		labels.MustNewMatcher(labels.MatchRegexp, "target", ""),
		labels.MustNewMatcher(labels.MatchNotRegexp, "target", "a.*"),
	} {
		s.Run(matcher.String(), func() {
			_, err := tsdb.PostingsForMatchers(s.T().Context(), r, matcher)
			s.Require().ErrorIs(err, failure)
		})
	}
}

func (s *inverseMatchersSuite) TestSelectors() {
	for _, typ := range []labels.MatchType{labels.MatchEqual, labels.MatchNotEqual, labels.MatchRegexp, labels.MatchNotRegexp} {
		for _, pattern := range []string{"", ".*", ".+", "(?s:.+)", "a|b", "a.*", ".*b"} {
			for _, reverse := range []bool{false, true} {
				s.Run(fmt.Sprintf("%v_%q_reverse=%t", typ, pattern, reverse), func() {
					ms := []*labels.Matcher{
						labels.MustNewMatcher(labels.MatchEqual, "__name__", "inverse_metric"),
						labels.MustNewMatcher(typ, "target", pattern),
						labels.MustNewMatcher(labels.MatchNotEqual, "target", "b"),
					}
					if reverse {
						slices.Reverse(ms)
					}
					expected := []string{}
					for _, series := range s.series {
						if testutils.LabelsMatch(series.Labels(), ms) {
							expected = append(expected, series.Labels().Get("id"))
						}
					}
					actual := []string{}
					set := s.q.Select(s.T().Context(), false, nil, ms...)
					for set.Next() {
						actual = append(actual, set.At().Labels().Get("id"))
					}
					s.Require().NoError(set.Err())
					slices.Sort(actual)
					s.Equal(expected, actual)
				})
			}
		}
	}
}

func (r postingsErrorReader) PostingsForAllLabelValues(context.Context, string) index.Postings {
	return index.ErrPostings(r.err)
}

func (r postingsErrorReader) PostingsForLabelMatching(context.Context, string, func(string) bool) index.Postings {
	return index.ErrPostings(r.err)
}

func TestBlockIndexReaderAllValuesError(t *testing.T) {
	failure := errors.New("index read failed")
	r := blockIndexReader{ir: postingsErrorReader{err: failure}}

	require.ErrorIs(t, r.PostingsForAllLabelValues(t.Context(), "target").Err(), failure)
}

func TestOOOCompactionAllLabelValuesUnsupported(t *testing.T) {
	r := &tsdb.OOOCompactionHeadIndexReader{}

	p := r.PostingsForAllLabelValues(t.Context(), "target")

	require.False(t, p.Next())
	require.ErrorContains(t, p.Err(), "not supported")
}
