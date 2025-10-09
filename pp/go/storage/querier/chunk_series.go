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
	lssQueryResult   *cppbridge.LSSQueryResult
	labelSetSnapshot *cppbridge.LabelSetSnapshot
	chunkRecoder     *cppbridge.ChunkRecoder

	index            int
	lastRecodedChunk *cppbridge.RecodedChunk
	chunkSeries      *ChunkSeries

	recoderIsExhausted bool
}

// NewChunkSeriesSet init new [ChunkSeriesSet].
func NewChunkSeriesSet(
	lssQueryResult *cppbridge.LSSQueryResult,
	labelSetSnapshot *cppbridge.LabelSetSnapshot,
	chunkRecoder *cppbridge.ChunkRecoder,
) *ChunkSeriesSet {
	return &ChunkSeriesSet{
		lssQueryResult:   lssQueryResult,
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
	recodedChunks := make([]cppbridge.RecodedChunk, 1)
	recodedChunks[0] = *css.lastRecodedChunk

	nextSeriesIDFound := false
	for css.nextChunk() {
		if css.lastRecodedChunk.SeriesId != seriesID {
			nextSeriesIDFound = true
			break
		}
		recodedChunks = append(recodedChunks, *css.lastRecodedChunk)
	}

	if !nextSeriesIDFound && css.recoderIsExhausted {
		css.lastRecodedChunk = nil
	}

	var (
		lsID     uint32
		lsLength uint16
	)

	for {
		if css.index >= css.lssQueryResult.Len() {
			return false
		}

		lsID, lsLength = css.lssQueryResult.GetByIndex(css.index)

		if lsID == seriesID {
			break
		}

		css.index++
	}

	css.chunkSeries = &ChunkSeries{
		labelSet:      labels.NewLabelsWithLSS(css.labelSetSnapshot, lsID, lsLength),
		recodedChunks: recodedChunks,
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
	chunkData := make([]byte, len(lastRecodedChunk.ChunkData))
	copy(chunkData, lastRecodedChunk.ChunkData)
	lastRecodedChunk.ChunkData = chunkData
	css.lastRecodedChunk = &lastRecodedChunk

	return true
}

//
// ChunkSeries
//

// ChunkSeries exposes a single time series and allows iterating over chunks.
type ChunkSeries struct {
	labelSet      labels.Labels
	recodedChunks []cppbridge.RecodedChunk
}

// Iterator returns an iterator that iterates over potentially overlapping
// chunks of the series, sorted by min time.
func (cs *ChunkSeries) Iterator(iterator chunks.Iterator) chunks.Iterator {
	if ci, ok := iterator.(*ChunkSeriesChunksIterator); ok {
		ci.ResetTo(cs.recodedChunks)
		return ci
	}

	return NewChunkSeriesChunksIterator(cs.recodedChunks)
}

// Labels returns the complete set of labels. For series it means all labels identifying the series.
func (cs *ChunkSeries) Labels() labels.Labels {
	return cs.labelSet
}

//
// ChunkSeriesChunksIterator
//

// ChunkSeriesChunksIterator iterator that iterates over chunks of the series, sorted by min time.
type ChunkSeriesChunksIterator struct {
	idx           int
	recodedChunks []cppbridge.RecodedChunk
	xorChunk      *chunkenc.XORChunk
	meta          chunks.Meta
}

// NewChunkSeriesChunksIterator init new [ChunkSeriesChunksIterator].
func NewChunkSeriesChunksIterator(recodedChunks []cppbridge.RecodedChunk) *ChunkSeriesChunksIterator {
	return &ChunkSeriesChunksIterator{
		recodedChunks: recodedChunks,
		xorChunk:      chunkenc.NewXORChunk(),
	}
}

// At returns the current meta.
func (ci *ChunkSeriesChunksIterator) At() chunks.Meta {
	return ci.meta
}

// Err returns the current error - always nil.
func (*ChunkSeriesChunksIterator) Err() error {
	return nil
}

// Next advances the iterator by one.
func (ci *ChunkSeriesChunksIterator) Next() bool {
	if ci.idx >= len(ci.recodedChunks) {
		return false
	}

	ci.meta.MinTime = ci.recodedChunks[ci.idx].MinT
	ci.meta.MaxTime = ci.recodedChunks[ci.idx].MaxT
	ci.xorChunk.Reset(ci.recodedChunks[ci.idx].ChunkData)
	ci.meta.Chunk = ci.xorChunk
	ci.idx++

	return true
}

// ResetTo reset [ChunkSeriesChunksIterator] to recodedChunks.
func (ci *ChunkSeriesChunksIterator) ResetTo(recodedChunks []cppbridge.RecodedChunk) {
	ci.idx = 0
	ci.recodedChunks = recodedChunks
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
