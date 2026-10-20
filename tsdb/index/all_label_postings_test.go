package index

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/testutil"
)

// allLabelPostingsSuite exercises the all-values API on memory and both persisted index formats.
type allLabelPostingsSuite struct {
	suite.Suite
	memory  *MemPostings
	readers []*Reader
}

func TestAllLabelPostings(t *testing.T) {
	suite.Run(t, new(allLabelPostingsSuite))
}

func (s *allLabelPostingsSuite) SetupTest() {
	s.memory = NewMemPostings()
	s.memory.m["target"] = map[string][]storage.SeriesRef{
		"a": {1, 3, 5}, "b": {1, 2, 5}, "empty": nil,
	}

	input := make(indexWriterSeriesSlice, 300)

	for i := range input {
		input[i] = &indexWriterSeries{
			labels: labels.FromStrings("id", fmt.Sprintf("%04d", i), "target", strconv.Itoa(i)),
		}
	}

	v2, _, _ := createFileReader(s.T().Context(), s.T(), input)
	v1, err := NewFileReader("../testdata/index_format_v1/index")
	s.Require().NoError(err)
	s.Require().Equal(FormatV1, v1.version)
	s.T().Cleanup(func() { s.Require().NoError(v1.Close()) })
	s.readers = []*Reader{v1, v2}
}

func (s *allLabelPostingsSuite) TestCancellationBeforeRead() {
	ctx, cancel := context.WithCancel(s.T().Context())
	cancel()

	s.Require().ErrorIs(s.memory.PostingsForAllLabelValues(ctx, "target").Err(), context.Canceled)

	for _, reader := range s.readers {
		s.Require().ErrorIs(reader.PostingsForAllLabelValues(ctx, "missing").Err(), context.Canceled)
	}
}

func (s *allLabelPostingsSuite) TestCancellationDuringMemoryReadReleasesLock() {
	for i := range 1000 {
		s.memory.Add(storage.SeriesRef(i+10), labels.FromStrings("target", strconv.Itoa(i)))
	}

	ctx := &testutil.MockContextErrAfter{FailAfter: 3}
	p := s.memory.PostingsForAllLabelValues(ctx, "target")
	s.Require().ErrorIs(p.Err(), context.Canceled)
	s.Require().True(s.memory.mtx.TryLock(), "cancellation must release RLock")
	s.memory.mtx.Unlock()
}

func (s *allLabelPostingsSuite) TestCancellationDuringPersistedRead() {
	for _, reader := range s.readers {
		s.Run(fmt.Sprintf("v%d", reader.version), func() {
			name := "target"
			r := *reader

			if r.version == FormatV1 {
				// Reuse an actual V1 postings offset to force a traversal longer than the context check interval.
				names, err := r.LabelNames(s.T().Context())
				s.Require().NoError(err)
				values, err := r.LabelValues(s.T().Context(), names[0])
				s.Require().NoError(err)
				offset := r.postingsV1[names[0]][values[0]]
				entries := make(map[string]uint64, 1000)

				for i := range 1000 {
					entries[strconv.Itoa(i)] = offset
				}

				r.postingsV1 = map[string]map[string]uint64{name: entries}
			}

			ctx := &testutil.MockContextErrAfter{FailAfter: 2}
			s.Require().ErrorIs(r.PostingsForAllLabelValues(ctx, name).Err(), context.Canceled)
		})
	}
}

func (s *allLabelPostingsSuite) TestCorruptedPostings() {
	for _, reader := range s.readers {
		s.Run(fmt.Sprintf("v%d", reader.version), func() {
			names, err := reader.LabelNames(s.T().Context())
			s.Require().NoError(err)
			name := names[0]
			var offset uint64

			if reader.version == FormatV1 {
				values, err := reader.LabelValues(s.T().Context(), name)
				s.Require().NoError(err)
				offset = reader.postingsV1[name][values[0]]
			} else {
				s.Require().NoError(reader.traversePostingOffsets(
					s.T().Context(), reader.postings[name][0].off,
					func(_ string, postingsOffset uint64) (bool, error) {
						offset = postingsOffset

						return false, nil
					},
				))
			}

			r := *reader
			data := slices.Clone(reader.b.Range(0, reader.b.Len()))
			data[int(offset)+4] ^= 0xff
			r.b = realByteSlice(data)

			s.Require().Error(r.PostingsForAllLabelValues(s.T().Context(), name).Err())
		})
	}
}

func (s *allLabelPostingsSuite) TestMemorySortedUnion() {
	p := s.memory.PostingsForAllLabelValues(s.T().Context(), "target")
	refs, err := ExpandPostings(p)
	s.Require().NoError(err)
	s.Equal([]storage.SeriesRef{1, 2, 3, 5}, refs)
	s.False(p.Next())

	p = s.memory.PostingsForAllLabelValues(s.T().Context(), "target")
	s.True(p.Seek(3))
	s.Equal(storage.SeriesRef(3), p.At())
	s.True(p.Next())
	s.Equal(storage.SeriesRef(5), p.At())
	s.False(p.Seek(6))
	s.Require().NoError(p.Err())
}

func (s *allLabelPostingsSuite) TestMissingLabel() {
	refs, err := ExpandPostings(s.memory.PostingsForAllLabelValues(s.T().Context(), "missing"))
	s.Require().NoError(err)
	s.Empty(refs)

	for _, reader := range s.readers {
		refs, err = ExpandPostings(reader.PostingsForAllLabelValues(s.T().Context(), "missing"))
		s.Require().NoError(err)
		s.Empty(refs)
	}
}

func (s *allLabelPostingsSuite) TestPersistedSortedUnionAndSeek() {
	for _, reader := range s.readers {
		s.Run(fmt.Sprintf("v%d", reader.version), func() {
			names, err := reader.LabelNames(s.T().Context())
			s.Require().NoError(err)

			for _, name := range names {
				values, err := reader.LabelValues(s.T().Context(), name)
				s.Require().NoError(err)
				existing, err := reader.Postings(s.T().Context(), name, values...)
				s.Require().NoError(err)
				expected, err := ExpandPostings(existing)
				s.Require().NoError(err)
				p := reader.PostingsForAllLabelValues(s.T().Context(), name)
				actual, err := ExpandPostings(p)
				s.Require().NoError(err)
				s.Require().NotEmpty(expected)
				s.Equal(expected, actual)
				s.Equal(slices.Compact(slices.Clone(actual)), actual)
				s.True(slices.IsSorted(actual))
				s.False(p.Next())

				p = reader.PostingsForAllLabelValues(s.T().Context(), name)
				middle := len(expected) / 2
				s.True(p.Seek(expected[middle]))
				s.Equal(expected[middle], p.At())
				s.False(p.Seek(expected[len(expected)-1] + 1))
				s.False(p.Next())
				s.Require().NoError(p.Err())
			}
		})
	}
}
