package querier

import (
	"container/heap"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

// mergeShardSeriesSet merges many [storage.SeriesSet] together from different shards.
type mergeShardSeriesSet struct {
	heap       seriesSetHeap
	currentSet storage.SeriesSet
}

// NewMergeShardSeriesSet returns a new [storage.SeriesSet] that merges many [storage.SeriesSet] together.
func NewMergeShardSeriesSet(sets []storage.SeriesSet) storage.SeriesSet {
	if len(sets) == 1 {
		return sets[0]
	}

	s := &mergeShardSeriesSet{
		heap: make(seriesSetHeap, 0, len(sets)),
	}
	for _, set := range sets {
		// shard series dont have errors and not nil, so we can safely call Next
		if set.Next() {
			heap.Push(&s.heap, set)
		}
	}

	return s
}

// At returns the current [storage.Series], implement [storage.SeriesSet] interface.
func (s *mergeShardSeriesSet) At() storage.Series {
	return s.currentSet.At()
}

// Err returns the error of the current [SeriesSet], implement [storage.SeriesSet] interface.
// Always returns nil, because shards should not have any errors.
func (*mergeShardSeriesSet) Err() error {
	return nil
}

// Next advances the iterator by one and returns false if there are no more values,
// implement [storage.SeriesSet] interface.
func (s *mergeShardSeriesSet) Next() bool {
	if s.currentSet != nil && s.currentSet.Next() {
		heap.Push(&s.heap, s.currentSet)
	}

	if len(s.heap) == 0 {
		return false
	}

	// Now, pop items of the heap that have equal label sets.
	s.currentSet = heap.Pop(&s.heap).(storage.SeriesSet)

	return true
}

// Warnings returns the warnings of the current SeriesSet, implement [storage.SeriesSet] interface.
// Always returns empty [annotations.Annotations], because shards should not have any warnings.
func (*mergeShardSeriesSet) Warnings() annotations.Annotations {
	return nil
}

//
// seriesSetHeap
//

// seriesSetHeap is a heap of [storage.SeriesSet].
type seriesSetHeap []storage.SeriesSet

// Len returns the length of the heap.
func (h seriesSetHeap) Len() int { return len(h) }

// Less compares the elements with indices i and j.
func (h seriesSetHeap) Less(i, j int) bool {
	return labels.Compare(h[i].At().Labels(), h[j].At().Labels()) < 0
}

// Pop pops the element with the highest priority from the heap.
func (h *seriesSetHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	old[n-1] = nil
	*h = old[0 : n-1]
	return x
}

// Push pushes the element x onto the heap.
func (h *seriesSetHeap) Push(x any) {
	*h = append(*h, x.(storage.SeriesSet))
}

// Swap swaps the elements with indices i and j.
func (h seriesSetHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

//
// mergeShardChunkSeriesSet
//

// mergeShardChunkSeriesSet merges many [storage.ChunkSeriesSet] together from different shards.
type mergeShardChunkSeriesSet struct {
	heap       chunkSeriesSetHeap
	currentSet storage.ChunkSeriesSet
}

// NewMergeShardChunkSeriesSet returns a new [storage.ChunkSeriesSet]
// that merges many [storage.ChunkSeriesSet] together.
func NewMergeShardChunkSeriesSet(sets []storage.ChunkSeriesSet) storage.ChunkSeriesSet {
	if len(sets) == 1 {
		return sets[0]
	}

	s := &mergeShardChunkSeriesSet{
		heap: make(chunkSeriesSetHeap, 0, len(sets)),
	}
	for _, set := range sets {
		// shard series dont have errors and not nil, so we can safely call Next
		if set.Next() {
			heap.Push(&s.heap, set)
		}
	}

	return s
}

// At returns the current [storage.ChunkSeries], implement [storage.ChunkSeriesSet] interface.
func (s *mergeShardChunkSeriesSet) At() storage.ChunkSeries {
	return s.currentSet.At()
}

// Err returns the error of the current [ChunkSeries], implement [storage.ChunkSeriesSet] interface.
// Always returns nil, because shards should not have any errors.
func (*mergeShardChunkSeriesSet) Err() error {
	return nil
}

// Next advances the iterator by one and returns false if there are no more values,
// implement [storage.ChunkSeriesSet] interface.
func (s *mergeShardChunkSeriesSet) Next() bool {
	if s.currentSet != nil && s.currentSet.Next() {
		heap.Push(&s.heap, s.currentSet)
	}

	if len(s.heap) == 0 {
		return false
	}

	// Now, pop items of the heap that have equal label sets.
	s.currentSet = heap.Pop(&s.heap).(storage.ChunkSeriesSet)

	return true
}

// Warnings returns the warnings of the current [ChunkSeries], implement [storage.ChunkSeriesSet] interface.
// Always returns empty [annotations.Annotations], because shards should not have any warnings.
func (*mergeShardChunkSeriesSet) Warnings() annotations.Annotations {
	return nil
}

//
// chunkSeriesSetHeap
//

// chunkSeriesSetHeap is a heap of [storage.ChunkSeriesSet].
type chunkSeriesSetHeap []storage.ChunkSeriesSet

// Len returns the length of the heap.
func (h chunkSeriesSetHeap) Len() int { return len(h) }

// Less compares the elements with indices i and j.
func (h chunkSeriesSetHeap) Less(i, j int) bool {
	return labels.Compare(h[i].At().Labels(), h[j].At().Labels()) < 0
}

// Pop pops the element with the highest priority from the heap.
func (h *chunkSeriesSetHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	old[n-1] = nil
	*h = old[0 : n-1]
	return x
}

// Push pushes the element x onto the heap.
func (h *chunkSeriesSetHeap) Push(x any) {
	*h = append(*h, x.(storage.ChunkSeriesSet))
}

// Swap swaps the elements with indices i and j.
func (h chunkSeriesSetHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
