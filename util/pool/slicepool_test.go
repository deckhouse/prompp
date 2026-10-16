package pool_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/util/pool"
)

// SlicePoolSuite checks bucket sizing and safe reuse of returned slices.
type SlicePoolSuite struct {
	suite.Suite

	pool pool.SlicePool[int]
}

func TestSlicePool(t *testing.T) {
	suite.Run(t, new(SlicePoolSuite))
}

func (s *SlicePoolSuite) SetupTest() {
	s.pool = pool.NewSlicePool[int]([]int{0, 1, 2, 4, 8})
}

func (s *SlicePoolSuite) TestGet() {
	cases := []struct {
		size        int
		expectedCap int
	}{
		{size: 0, expectedCap: 0},
		{size: 2, expectedCap: 2},
		{size: 3, expectedCap: 4},
		{size: 5, expectedCap: 8},
		{size: 9, expectedCap: 9},
	}

	for _, c := range cases {
		item := s.pool.Get(c.size)
		s.Len(item, c.size)
		s.Equal(c.expectedCap, cap(item))
		s.pool.Put(item)
	}
}

func (s *SlicePoolSuite) TestPutBelowSmallestBucket() {
	testPool := pool.NewSlicePool[int]([]int{4, 8})
	testPool.Put(make([]int, 0, 3))

	item := testPool.Get(4)

	s.Len(item, 4)
	s.Equal(4, cap(item))
}

func (s *SlicePoolSuite) TestPutBetweenBucketSizes() {
	s.pool.Put(make([]int, 0, 5))

	item := s.pool.Get(8)

	s.Len(item, 8)
	s.Equal(8, cap(item))
}

func (s *SlicePoolSuite) TestPutOversized() {
	s.pool.Put(make([]int, 0, 9))

	item := s.pool.Get(8)

	s.Len(item, 8)
	s.Equal(8, cap(item))
}

func (s *SlicePoolSuite) TestPutShortenedSlice() {
	item := s.pool.Get(8)
	s.pool.Put(item[:0])

	item = s.pool.Get(8)

	s.Len(item, 8)
	s.Equal(8, cap(item))
}
