package pool_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/util/pool"
)

func TestSlicePool(t *testing.T) {
	testPool := pool.NewSlicePool[int]([]int{0, 1, 2, 4})

	cases := []struct {
		size        int
		expectedCap int
	}{
		{
			size:        0,
			expectedCap: 0,
		},
		{
			size:        2,
			expectedCap: 2,
		},
		{
			size:        3,
			expectedCap: 4,
		},
		{
			size:        5,
			expectedCap: 5,
		},
	}
	for _, c := range cases {
		ret := testPool.Get(c.size)
		require.Equal(t, c.expectedCap, cap(ret))
		testPool.Put(ret)
	}
}
