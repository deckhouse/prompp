package pool

import (
	"github.com/prometheus/prometheus/util/zeropool"
)

// SlicePool is a pool of slices.
type SlicePool[T any] struct {
	buckets []zeropool.Pool[[]T]
	sizes   []int
}

// NewSlicePool creates a new [SlicePool].
func NewSlicePool[T any](sizes []int) SlicePool[T] {
	if len(sizes) == 0 {
		panic("invalid sizes")
	}

	for _, size := range sizes {
		if size < 0 {
			panic("invalid size")
		}
	}

	buckets := make([]zeropool.Pool[[]T], len(sizes))
	for i, size := range sizes {
		buckets[i] = zeropool.New(func() []T { return make([]T, size) })
	}

	return SlicePool[T]{
		buckets: buckets,
		sizes:   sizes,
	}
}

// Get returns a new slice of the given size.
func (p *SlicePool[T]) Get(size int) []T {
	if size < 0 {
		panic("invalid size")
	}

	for i, bktSize := range p.sizes {
		if size > bktSize {
			continue
		}

		return p.buckets[i].Get()[:size]
	}

	return make([]T, size)
}

// Put adds a slice to the pool.
func (p *SlicePool[T]) Put(item []T) {
	// If the item is larger than the largest size in the pool, don't put it back.
	if cap(item) > p.sizes[len(p.sizes)-1] {
		return
	}

	for i, size := range p.sizes {
		if cap(item) > size {
			continue
		}

		p.buckets[i].Put(item)
		return
	}
}
