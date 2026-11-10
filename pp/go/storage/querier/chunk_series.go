package querier

import (
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/util/annotations"
)

//
// ChunkSeriesSet
//

// ChunkSeriesSet contains a set of chunked series.
type ChunkSeriesSet struct {
	labelSetSnapshot *cppbridge.LabelSetSnapshot
	chunkRecoder     *cppbridge.ChunkRecoder

	lastRecodedChunk *cppbridge.RecodedChunk
	chunkSeries      *ChunkSeries

	recoderIsExhausted bool
}

// NewChunkSeriesSet init new [ChunkSeriesSet].
// The series id is taken directly from the recoded chunks during iteration, so the
// query result buffer does not need to be retained (see [cppbridge.LSSQueryResult.Close]).
func NewChunkSeriesSet(
	labelSetSnapshot *cppbridge.LabelSetSnapshot,
	chunkRecoder *cppbridge.ChunkRecoder,
) *ChunkSeriesSet {
	return &ChunkSeriesSet{
		labelSetSnapshot: labelSetSnapshot,
		chunkRecoder:     chunkRecoder,
	}
}

// At returns full chunk series. Returned series should be iterable even after Next is called.
func (css *ChunkSeriesSet) At() storage.ChunkSeries {
	return css.chunkSeries
}

// Err returns the current error - always nil.
func (*ChunkSeriesSet) Err() error {
	return nil
}

// Next advances the iterator by one and returns false if there are no more values.
func (css *ChunkSeriesSet) Next() bool {
	if css.lastRecodedChunk == nil && !css.nextChunk() {
		return false
	}

	seriesID := css.lastRecodedChunk.SeriesId
	seriesChunks := make([]chunkSeriesChunk, 1)
	seriesChunks[0] = newChunkSeriesChunk(*css.lastRecodedChunk)

	nextSeriesIDFound := false
	for css.nextChunk() {
		if css.lastRecodedChunk.SeriesId != seriesID {
			nextSeriesIDFound = true
			break
		}
		seriesChunks = append(seriesChunks, newChunkSeriesChunk(*css.lastRecodedChunk))
	}

	if !nextSeriesIDFound && css.recoderIsExhausted {
		css.lastRecodedChunk = nil
	}

	css.chunkSeries = &ChunkSeries{
		labelSet: labels.NewLabelsWithLSS(css.labelSetSnapshot, seriesID),
		chunks:   seriesChunks,
	}

	return true
}

// Warnings a collection of warnings for the whole set - always nil.
func (*ChunkSeriesSet) Warnings() annotations.Annotations {
	return nil
}

// nextChunk advances the iterator by one and returns false if there are no more values.
func (css *ChunkSeriesSet) nextChunk() bool {
	if css.recoderIsExhausted {
		return false
	}

	lastRecodedChunk := css.chunkRecoder.RecodeNextChunk()
	css.recoderIsExhausted = !lastRecodedChunk.HasMoreData
	if lastRecodedChunk.SeriesId == cppbridge.InvalidSeriesId {
		return false
	}

	chunkData := make([]byte, len(lastRecodedChunk.ChunkData))
	copy(chunkData, lastRecodedChunk.ChunkData)
	lastRecodedChunk.ChunkData = chunkData
	css.lastRecodedChunk = &lastRecodedChunk

	return true
}

//
// ChunkSeries
//

// chunkSeriesChunk stores millisecond time bounds and an immutable XOR wrapper over an owned payload.
type chunkSeriesChunk struct {
	cppbridge.TimeInterval
	chunk chunkenc.XORChunk
}

// newChunkSeriesChunk retains the existing Go-owned payload without copying it.
func newChunkSeriesChunk(recodedChunk cppbridge.RecodedChunk) chunkSeriesChunk {
	result := chunkSeriesChunk{TimeInterval: recodedChunk.TimeInterval}
	result.chunk.Reset(recodedChunk.ChunkData)

	return result
}

// ChunkSeries exposes a single time series and allows iterating over chunks.
type ChunkSeries struct {
	labelSet labels.Labels
	chunks   []chunkSeriesChunk
}

// Iterator returns an iterator that iterates over potentially overlapping
// chunks of the series, sorted by min time.
func (cs *ChunkSeries) Iterator(iterator chunks.Iterator) chunks.Iterator {
	if ci, ok := iterator.(*ChunkSeriesChunksIterator); ok {
		ci.idx = 0
		ci.chunks = cs.chunks

		return ci
	}

	return &ChunkSeriesChunksIterator{chunks: cs.chunks}
}

// Labels returns the complete set of labels. For series it means all labels identifying the series.
func (cs *ChunkSeries) Labels() labels.Labels {
	return cs.labelSet
}

//
// ChunkSeriesChunksIterator
//

// ChunkSeriesChunksIterator iterates over chunks sorted by min time.
// Returned chunks remain valid after advancing or resetting the iterator.
type ChunkSeriesChunksIterator struct {
	idx    int
	chunks []chunkSeriesChunk
	meta   chunks.Meta
}

// At returns the current meta, whose chunk remains unchanged after [ChunkSeriesChunksIterator.Next]
// and reuse through [ChunkSeries.Iterator].
func (ci *ChunkSeriesChunksIterator) At() chunks.Meta {
	return ci.meta
}

// Err returns the current error - always nil.
func (*ChunkSeriesChunksIterator) Err() error {
	return nil
}

// Next advances the iterator by one.
func (ci *ChunkSeriesChunksIterator) Next() bool {
	if ci.idx >= len(ci.chunks) {
		return false
	}

	chunk := &ci.chunks[ci.idx]
	ci.meta.MinTime = chunk.MinT
	ci.meta.MaxTime = chunk.MaxT
	ci.meta.Chunk = &chunk.chunk
	ci.idx++

	return true
}

//
// EmptyChunkSeriesSet
//

// EmptyChunkSeriesSet implementation [ChunkSeriesSet], do nothing.
type EmptyChunkSeriesSet struct{}

// At implementation [ChunkSeriesSet], do nothing.
func (EmptyChunkSeriesSet) At() storage.ChunkSeries {
	return nil
}

// Err implementation [ChunkSeriesSet], do nothing.
func (EmptyChunkSeriesSet) Err() error {
	return nil
}

// Next implementation [ChunkSeriesSet], do nothing.
func (EmptyChunkSeriesSet) Next() bool {
	return false
}

// Warnings implementation [ChunkSeriesSet], do nothing.
func (EmptyChunkSeriesSet) Warnings() annotations.Annotations {
	return nil
}
