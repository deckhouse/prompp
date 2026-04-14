package querier

import (
	"container/heap"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

// mergeShardSeriesSet merges many SeriesSets together from different shards.
type mergeShardSeriesSet struct {
	sets       []storage.SeriesSet
	heap       seriesSetHeap
	currentSet storage.SeriesSet
	inited     bool
}

// NewMergeShardSeriesSet returns a new SeriesSet that merges many SeriesSets together.
func NewMergeShardSeriesSet(sets []storage.SeriesSet) storage.SeriesSet {
	if len(sets) == 1 {
		return sets[0]
	}

	// TODO init heap, sets not needed in constructor
	return &mergeShardSeriesSet{
		sets:   append(make([]storage.SeriesSet, 0, len(sets)), sets...),
		inited: false,
	}
}

// At returns the current SeriesSet, implement [storage.SeriesSet] interface.
func (s *mergeShardSeriesSet) At() storage.Series {
	return s.currentSet.At()
}

// Err returns the error of the current SeriesSet, implement [storage.SeriesSet] interface.
func (s *mergeShardSeriesSet) Err() error {
	for _, set := range s.sets {
		if err := set.Err(); err != nil {
			return err
		}
	}

	// TODO return nil instead of empty error
	return nil
}

// Next advances the iterator by one and returns false if there are no more values,
// implement [storage.SeriesSet] interface.
func (s *mergeShardSeriesSet) Next() bool {
	if !s.inited {
		s.heap = make(seriesSetHeap, 0, len(s.sets))
		for _, set := range s.sets {
			if set.Next() {
				heap.Push(&s.heap, set)
			}
		}

		s.inited = true
	}

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
func (s *mergeShardSeriesSet) Warnings() annotations.Annotations {
	var ws annotations.Annotations
	for _, set := range s.sets {
		ws.Merge(set.Warnings())
	}

	// TODO return nil instead of empty annotations.Annotations
	return ws
}

//
// seriesSetHeap
//

// seriesSetHeap is a heap of SeriesSets.
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
	*h = old[0 : n-1]
	return x
}

// Push pushes the element x onto the heap.
func (h *seriesSetHeap) Push(x any) {
	*h = append(*h, x.(storage.SeriesSet))
}

// Swap swaps the elements with indices i and j.
func (h seriesSetHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
