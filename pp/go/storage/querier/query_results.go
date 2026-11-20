package querier

import (
	"sync"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

//
// querySeriesSet
//

// querySeriesSet is the iteration contract common to [storage.SeriesSet] and [storage.ChunkSeriesSet].
type querySeriesSet[T any] interface {
	// At returns the series selected by the latest successful call to [querySeriesSet.Next].
	At() T
	// Err reports why iteration stopped, if it failed.
	Err() error
	// Next advances to another series, returning false when no series remains or iteration fails.
	Next() bool
	// Warnings reports non-fatal issues encountered while selecting series.
	Warnings() annotations.Annotations
}

//
// queryResult
//

// queryResult owns a successful Select result until its querier closes it.
// Its wrapper stops iteration even when the underlying merge already has prefetched series.
type queryResult[T, R any] struct {
	set       querySeriesSet[T]
	resources R
	next      *queryResult[T, R]
}

// At returns the current series, or the zero value after the querier closes.
func (r *queryResult[T, R]) At() T {
	if r.set == nil {
		var zero T
		return zero
	}

	return r.set.At()
}

// Err reports the wrapped set's iteration error while the querier is open.
func (r *queryResult[T, R]) Err() error {
	if r.set == nil {
		return nil
	}

	return r.set.Err()
}

// Next advances the set unless the querier has closed, even if the set holds prefetched series.
func (r *queryResult[T, R]) Next() bool {
	return r.set != nil && r.set.Next()
}

// Warnings reports selection warnings while the querier is open.
func (r *queryResult[T, R]) Warnings() annotations.Annotations {
	if r.set == nil {
		return nil
	}

	return r.set.Warnings()
}

//
// queryResults
//

// queryResults keeps all successful Select results, not just the most recent one.
// The mutex serializes registration by parallel Select calls. Close must run after all external use.
type queryResults[T, R any] struct {
	mu       sync.Mutex
	first    *queryResult[T, R]
	isClosed bool
}

// register takes ownership of a selection and its resources until the querier closes.
func (r *queryResults[T, R]) register(set querySeriesSet[T], resources R) *queryResult[T, R] {
	result := &queryResult[T, R]{set: set, resources: resources}

	r.mu.Lock()
	result.next = r.first
	r.first = result
	r.mu.Unlock()

	return result
}

// releaseAll invalidates each result before releasing its resources. A nil release only invalidates results.
// Call it only after all concurrent selections and iterations have ended.
func (r *queryResults[T, R]) releaseAll(release func(R)) {
	r.isClosed = true

	for r.first != nil {
		result := r.first
		r.first = result.next
		result.next = nil
		result.set = nil
		resources := result.resources
		var zero R

		result.resources = zero

		if release != nil {
			release(resources)
		}
	}
}

//
// chunkResultResources
//

// chunkResultResources holds the native inputs of a published chunk selection in pooled shard slices.
type chunkResultResources struct {
	sets           []storage.ChunkSeriesSet
	serializedData []*cppbridge.DataStorageSerializedData
}

// closeChunkSeriesSets drops each shard recoder before the caller frees its serialized input.
func closeChunkSeriesSets(sets []storage.ChunkSeriesSet) {
	for _, set := range sets {
		if chunks, ok := set.(*ChunkSeriesSet); ok {
			chunks.releaseRecoder()
		}
	}
}

// closeSerializedData releases prepared buffers, skipping empty and not-yet-materialized results.
func closeSerializedData(buffers []*cppbridge.DataStorageSerializedData) {
	for _, buffer := range buffers {
		buffer.Close()
	}
}
