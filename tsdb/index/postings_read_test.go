package index

import (
	"encoding/binary"
	"errors"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/storage"
)

// PostingsReadSuite checks intersection results for list and on-disk postings representations.
type PostingsReadSuite struct {
	suite.Suite

	refs [][]storage.SeriesRef
}

func TestPostingsReadSuite(t *testing.T) {
	suite.Run(t, new(PostingsReadSuite))
}

func (s *PostingsReadSuite) SetupTest() {
	s.refs = [][]storage.SeriesRef{{2, 5, 9, 18, 21, 30}, {3, 7, 14, 19, 21, 30}, {1, 21, 30}}
}

func (s *PostingsReadSuite) TestErrors() {
	expected := errors.New("postings read failed")
	intersection := Intersect(NewListPostings(s.refs[0]), ErrPostings(expected), NewListPostings(s.refs[2]))
	s.False(intersection.Next())
	s.Require().ErrorIs(intersection.Err(), expected)
	intersection = Intersect(NewListPostings(s.refs[0]), ErrPostings(expected), NewListPostings(s.refs[2]))
	s.False(intersection.Seek(1))
	s.Require().ErrorIs(intersection.Err(), expected)
}

func (s *PostingsReadSuite) TestNextAndSeek() {
	for _, bigEndian := range []bool{false, true} {
		for _, reverse := range []bool{false, true} {
			postings := s.makePostings(s.refs, bigEndian)

			if reverse {
				slices.Reverse(postings)
			}

			intersection := Intersect(postings...)
			s.Require().True(intersection.Seek(1))
			s.Equal(storage.SeriesRef(21), intersection.At())
			s.Require().True(intersection.Seek(1))
			s.Equal(storage.SeriesRef(21), intersection.At())
			s.Require().True(intersection.Next())
			s.Equal(storage.SeriesRef(30), intersection.At())
			s.Require().True(intersection.Seek(25))
			s.Equal(storage.SeriesRef(30), intersection.At())
			s.False(intersection.Next())
			s.False(intersection.Next())
			s.NoError(intersection.Err())
		}
	}
}

func (s *PostingsReadSuite) TestPatterns() {
	for _, pattern := range []string{"aligned", "offset", "sparse_first", "sparse_last", "disjoint"} {
		refs, expectedCount := postingsReadFixture(pattern)

		for _, bigEndian := range []bool{false, true} {
			values, err := ExpandPostings(Intersect(s.makePostings(refs, bigEndian)...))
			s.Require().NoError(err)
			s.Len(values, expectedCount)
			intersection := Intersect(s.makePostings(refs, bigEndian)...)

			if intersection.Seek(1) {
				s.Require().NotEmpty(values)
				s.Equal(values[0], intersection.At())
				actual := []storage.SeriesRef{intersection.At()}
				remaining, err := ExpandPostings(intersection)
				s.Require().NoError(err)
				s.Equal(values, append(actual, remaining...))
			} else {
				s.Empty(values)
			}

			s.NoError(intersection.Err())
		}
	}
}

func (s *PostingsReadSuite) TestRandomIntersections() {
	random := rand.New(rand.NewPCG(1, 2))

	for trial := range 100 {
		refs := make([][]storage.SeriesRef, 2+trial%4)
		counts := make([]int, 129)

		for i := range refs {
			for id := storage.SeriesRef(1); id < 129; id++ {
				if random.IntN(4) == 0 {
					refs[i] = append(refs[i], id)
					counts[id]++
				}
			}
		}

		var expected []storage.SeriesRef

		for id := storage.SeriesRef(1); id < 129; id++ {
			if counts[id] == len(refs) {
				expected = append(expected, id)
			}
		}

		for _, bigEndian := range []bool{false, true} {
			actual, err := ExpandPostings(Intersect(s.makePostings(refs, bigEndian)...))
			s.Require().NoError(err)
			s.Equal(expected, actual)
		}
	}
}

func (s *PostingsReadSuite) TestZeroReference() {
	refs := [][]storage.SeriesRef{{0, 1, 2, 3}, {0, 1, 2, 3}, {0, 1, 2, 3}}

	for _, bigEndian := range []bool{false, true} {
		values, err := ExpandPostings(Intersect(s.makePostings(refs, bigEndian)...))
		s.Require().NoError(err)
		s.Equal(refs[0], values)
	}
}

func (s *PostingsReadSuite) makePostings(refs [][]storage.SeriesRef, bigEndian bool) []Postings {
	postings := make([]Postings, len(refs))

	for i, values := range refs {
		if !bigEndian {
			postings[i] = NewListPostings(values)
			continue
		}

		encoded := make([]byte, 4*len(values))

		for j, value := range values {
			binary.BigEndian.PutUint32(encoded[4*j:], uint32(value)) // #nosec G115 // fixture IDs fit uint32
		}

		postings[i] = newBigEndianPostings(encoded)
	}

	return postings
}
