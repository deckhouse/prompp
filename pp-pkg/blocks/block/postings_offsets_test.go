package block

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp-pkg/blocks/testutils"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/encoding"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/util/testutil"
)

const (
	// postingsCaseNone names the subtest where no label values match.
	postingsCaseNone = "none"
	// postingsCaseSparse names the subtest where isolated label values match.
	postingsCaseSparse = "sparse"
)

// postingsOffsetsSuite covers tsdb/index's postingsForLabelMatchingOffsets via
// [index.Reader.PostingsForLabelMatching] and the pass-through [blockIndexReader].
// These fork-specific tests live here to keep tsdb/index/index_test.go unchanged
// and reduce conflicts when backporting upstream changes.
type postingsOffsetsSuite struct {
	suite.Suite
	path   string
	reader *index.Reader
	values []string
}

func TestPostingsOffsets(t *testing.T) {
	suite.Run(t, new(postingsOffsetsSuite))
}

func (s *postingsOffsetsSuite) SetupSuite() {
	series := make([]storage.Series, 2000)
	s.values = make([]string, len(series))

	for i := range series {
		s.values[i] = fmt.Sprintf("v%06d", i)
		series[i] = storage.NewListSeries(labels.FromStrings("target", s.values[i]), []chunks.Sample{
			testutils.SampleTest{TS: 100, V: float64(i)},
		})
	}

	dir := testutils.CreateBlock(s.T(), s.T().TempDir(), series)
	s.path = filepath.Join(dir, "index")
	reader, err := index.NewFileReader(s.path)
	s.Require().NoError(err)
	s.reader = reader
	s.T().Cleanup(func() { s.Require().NoError(reader.Close()) })
}

func (s *postingsOffsetsSuite) TestCancellationDuringDecoding() {
	ctx := &testutil.MockContextErrAfter{FailAfter: ^uint64(0)}
	calls := 0
	r := blockIndexReader{ir: s.reader}
	p := r.PostingsForLabelMatching(ctx, "target", func(string) bool {
		calls++

		if calls == len(s.values) {
			// Traversal makes no further ctx.Err() calls after this final callback.
			// Allow decoding's first check at i == 0, then cancel at its next check at i == 128.
			ctx.FailAfter = ctx.Count() + 2
		}

		return true
	})

	s.Equal(len(s.values), calls)
	s.Require().ErrorIs(p.Err(), context.Canceled)
}

func (s *postingsOffsetsSuite) TestDecodeError() {
	ranges, err := s.reader.PostingsRanges()
	s.Require().NoError(err)
	data, err := os.ReadFile(s.path)
	s.Require().NoError(err)
	postingRange := ranges[labels.Label{Name: "target", Value: s.values[0]}]
	data[postingRange.Start] ^= 0xff
	reader, err := index.NewReader(postingsTestBytes(data))
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.Require().NoError(reader.Close()) })
	r := blockIndexReader{ir: reader}
	p := r.PostingsForLabelMatching(s.T().Context(), "target", func(string) bool { return true })

	s.Require().ErrorIs(p.Err(), encoding.ErrInvalidChecksum)
}

func (s *postingsOffsetsSuite) TestExhaustion() {
	for _, tc := range []struct {
		name  string
		match func(string) bool
	}{
		{postingsCaseNone, func(string) bool { return false }},
		{postingsCaseSparse, func(v string) bool { return strings.HasSuffix(v, "001") }},
	} {
		s.Run(tc.name, func() {
			r := blockIndexReader{ir: s.reader}
			p := r.PostingsForLabelMatching(s.T().Context(), "target", tc.match)
			_, err := index.ExpandPostings(p)
			s.Require().NoError(err)

			s.False(p.Next())
			s.Require().NoError(p.Err())
		})
	}
}

func (s *postingsOffsetsSuite) TestPredicateOnce() {
	for _, tc := range []struct {
		name  string
		match func(string) bool
	}{
		{postingsCaseNone, func(string) bool { return false }},
		{"single", func(v string) bool { return v == "v000001" }},
		{postingsCaseSparse, func(v string) bool { return strings.HasSuffix(v, "001") }},
		{"broad", func(string) bool { return true }},
	} {
		s.Run(tc.name, func() {
			calls := make(map[string]int, len(s.values))
			r := blockIndexReader{ir: s.reader}
			p := r.PostingsForLabelMatching(s.T().Context(), "target", func(v string) bool {
				calls[v]++

				return tc.match(v)
			})
			_, err := index.ExpandPostings(p)
			s.Require().NoError(err)

			for _, value := range s.values {
				s.Equal(1, calls[value], "predicate calls for %s", value)
			}
		})
	}
}

func (s *postingsOffsetsSuite) TestResults() {
	for _, tc := range []struct {
		name  string
		match func(string) bool
	}{
		{postingsCaseNone, func(string) bool { return false }},
		{"single", func(v string) bool { return v == "v000001" }},
		{postingsCaseSparse, func(v string) bool { return strings.HasSuffix(v, "001") }},
		{"broad", func(string) bool { return true }},
	} {
		s.Run(tc.name, func() {
			r := blockIndexReader{ir: s.reader}
			p := r.PostingsForLabelMatching(s.T().Context(), "target", tc.match)
			actual, err := index.ExpandPostings(p)
			s.Require().NoError(err)
			var matched []string

			for _, value := range s.values {
				if tc.match(value) {
					matched = append(matched, value)
				}
			}

			expectedPostings, err := s.reader.Postings(s.T().Context(), "target", matched...)
			s.Require().NoError(err)
			expected, err := index.ExpandPostings(expectedPostings)
			s.Require().NoError(err)
			s.Equal(expected, actual)
		})
	}
}

func (s *postingsOffsetsSuite) TestSeek() {
	expectedPostings, err := s.reader.Postings(s.T().Context(), "target", "v000001", "v001001")
	s.Require().NoError(err)
	expected, err := index.ExpandPostings(expectedPostings)
	s.Require().NoError(err)
	s.Require().Len(expected, 2)
	r := blockIndexReader{ir: s.reader}
	p := r.PostingsForLabelMatching(s.T().Context(), "target", func(v string) bool {
		return strings.HasSuffix(v, "001")
	})

	s.Require().True(p.Seek(expected[1]))
	s.Equal(expected[1], p.At())
	s.False(p.Seek(expected[1] + 1))
	s.Require().NoError(p.Err())
}

//
// postingsTestBytes
//

// postingsTestBytes exposes mutable index bytes through [index.ByteSlice] for corruption tests.
type postingsTestBytes []byte

func (b postingsTestBytes) Len() int {
	return len(b)
}

func (b postingsTestBytes) Range(start, end int) []byte {
	return b[start:end]
}
