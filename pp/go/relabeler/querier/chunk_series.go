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

type ChunkSeriesSet struct {
	lssQueryResult   *cppbridge.LSSQueryResult
	labelSetSnapshot *cppbridge.LabelSetSnapshot
	chunkRecoder     *cppbridge.ChunkRecoder

	index            int
	lastRecodedChunk *cppbridge.RecodedChunk
	chunkSeries      *ChunkSeries

	recoderIsExhausted bool
}

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

func (css *ChunkSeriesSet) Next() bool {
	if css.lastRecodedChunk == nil && !css.next() {
		return false
	}

	seriesID := css.lastRecodedChunk.SeriesId
	recodedChunks := make([]cppbridge.RecodedChunk, 1)
	recodedChunks[0] = *css.lastRecodedChunk

	nextSeriesIDFound := false
	for css.next() {
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

func (css *ChunkSeriesSet) next() bool {
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

func (css *ChunkSeriesSet) At() storage.ChunkSeries {
	return css.chunkSeries
}

func (css *ChunkSeriesSet) Err() error {
	return nil
}

func (css *ChunkSeriesSet) Warnings() annotations.Annotations {
	return nil
}

//
// ChunkSeries
//

type ChunkSeries struct {
	labelSet      labels.Labels
	recodedChunks []cppbridge.RecodedChunk
}

func (cs *ChunkSeries) Labels() labels.Labels {
	return cs.labelSet
}

func (cs *ChunkSeries) Iterator(iterator chunks.Iterator) chunks.Iterator {
	if ci, ok := iterator.(*ChunkSeriesChunksIterator); ok {
		ci.ResetTo(cs.recodedChunks)
		return ci
	}
	return NewChunkSeriesChunksIterator(cs.recodedChunks)
}

//
// ChunkSeriesChunksIterator
//

type ChunkSeriesChunksIterator struct {
	idx           int
	recodedChunks []cppbridge.RecodedChunk
	xorChunk      *chunkenc.XORChunk
	meta          chunks.Meta
}

func NewChunkSeriesChunksIterator(recodedChunks []cppbridge.RecodedChunk) *ChunkSeriesChunksIterator {
	return &ChunkSeriesChunksIterator{
		recodedChunks: recodedChunks,
		xorChunk:      chunkenc.NewXORChunk(),
	}
}

func (ci *ChunkSeriesChunksIterator) ResetTo(recodedChunks []cppbridge.RecodedChunk) {
	ci.idx = 0
	ci.recodedChunks = recodedChunks
}

func (ci *ChunkSeriesChunksIterator) At() chunks.Meta {
	return ci.meta
}

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

func (ci *ChunkSeriesChunksIterator) Err() error {
	return nil
}

type EmptyChunkSeriesSet struct{}

func (EmptyChunkSeriesSet) Next() bool {
	return false
}

func (EmptyChunkSeriesSet) At() storage.ChunkSeries {
	return nil
}

func (EmptyChunkSeriesSet) Err() error {
	return nil
}

func (EmptyChunkSeriesSet) Warnings() annotations.Annotations {
	return nil
}
