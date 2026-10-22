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

// postingsTestBytes exposes mutable index bytes through [index.ByteSlice] for corruption tests.
type postingsTestBytes []byte

// postingsOffsetsSuite checks predicate traversal and the subsequent decoding phase through the block wrapper.
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
			ctx.FailAfter = ctx.Count() + 2
		}

		return true
	})

	s.Equal(len(s.values), calls)
	s.Require().ErrorIs(p.Err(), context.Canceled)
	s.Equal(ctx.FailAfter, ctx.Count())
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

func (s *postingsOffsetsSuite) TestPredicateOnce() {
	for _, tc := range []struct {
		name  string
		match func(string) bool
	}{
		{"none", func(string) bool { return false }},
		{"single", func(v string) bool { return v == "v000001" }},
		{"sparse", func(v string) bool { return strings.HasSuffix(v, "001") }},
		{"broad", func(string) bool { return true }},
	} {
		s.Run(tc.name, func() {
			calls := make(map[string]int, len(s.values))
			r := blockIndexReader{ir: s.reader}
			p := r.PostingsForLabelMatching(s.T().Context(), "target", func(v string) bool {
				calls[v]++

				return tc.match(v)
			})
			actual, err := index.ExpandPostings(p)
			s.Require().NoError(err)
			var matched []string

			for _, value := range s.values {
				s.Equal(1, calls[value])

				if tc.match(value) {
					matched = append(matched, value)
				}
			}

			expectedPostings, err := s.reader.Postings(s.T().Context(), "target", matched...)
			s.Require().NoError(err)
			expected, err := index.ExpandPostings(expectedPostings)
			s.Require().NoError(err)
			s.Equal(expected, actual)
			s.False(p.Next())

			if len(expected) > 0 {
				p = r.PostingsForLabelMatching(s.T().Context(), "target", tc.match)
				s.True(p.Seek(expected[len(expected)/2]))
				s.Equal(expected[len(expected)/2], p.At())
				s.False(p.Seek(expected[len(expected)-1] + 1))
				s.Require().NoError(p.Err())
			}
		})
	}
}

func (b postingsTestBytes) Len() int {
	return len(b)
}

func (b postingsTestBytes) Range(start, end int) []byte {
	return b[start:end]
}
