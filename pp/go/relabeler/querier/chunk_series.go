package querier

import (
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/util/annotations"
	"time"
)

type LabelSetsIterator struct {
	idx       int
	labelSets []*cppbridge.LabelsCpp
}

func newLabelSetIterator(labelSets []*cppbridge.LabelsCpp) *LabelSetsIterator {
	return &LabelSetsIterator{labelSets: labelSets}
}

func (lsi *LabelSetsIterator) Seek(labelSetID uint32) (*cppbridge.LabelsCpp, bool) {
	for {
		if lsi.idx >= len(lsi.labelSets) {
			return nil, false
		}

		if lsi.labelSets[lsi.idx].ID() == labelSetID {
			return lsi.labelSets[lsi.idx], true
		}

		lsi.idx++
	}
}

type ChunkSeriesSet struct {
	labelSetsIterator  *LabelSetsIterator
	chunkRecoder       *cppbridge.ChunkRecoder
	recoderIsExhausted bool

	lastRecodedChunk *cppbridge.RecodedChunk
	chunkSeries      *ChunkSeries
}

func NewChunkSeriesSet(labelSets []*cppbridge.LabelsCpp, chunkRecoder *cppbridge.ChunkRecoder) *ChunkSeriesSet {
	return &ChunkSeriesSet{
		labelSetsIterator: newLabelSetIterator(labelSets),
		chunkRecoder:      chunkRecoder,
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

	labelSet, ok := css.labelSetsIterator.Seek(seriesID)
	if !ok {
		return false
	}

	css.chunkSeries = NewChunkSeries(labelSet, recodedChunks)
	return true
}

func (css *ChunkSeriesSet) next() bool {
	if css.recoderIsExhausted {
		return false
	}

	lastRecodedChunk := css.chunkRecoder.RecodeNextChunk()
	css.recoderIsExhausted = !lastRecodedChunk.HasMoreData
	//fmt.Println("last recoded chunk", lastRecodedChunk.SeriesId, lastRecodedChunk.MinT, lastRecodedChunk.MaxT, lastRecodedChunk.HasMoreData)
	chunkData := make([]byte, len(lastRecodedChunk.ChunkData))
	copy(chunkData, lastRecodedChunk.ChunkData)
	lastRecodedChunk.ChunkData = chunkData
	css.lastRecodedChunk = &lastRecodedChunk
	//fmt.Println("css.next end")
	time.Sleep(time.Millisecond * 500)
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

type ChunkSeries struct {
	labelSet      *cppbridge.LabelsCpp
	recodedChunks []cppbridge.RecodedChunk
}

func NewChunkSeries(labelSet *cppbridge.LabelsCpp, recodedChunks []cppbridge.RecodedChunk) *ChunkSeries {
	return &ChunkSeries{
		labelSet:      labelSet,
		recodedChunks: recodedChunks,
	}
}

func (cs *ChunkSeries) Labels() labels.Labels {
	return cs.labelSet.Labels()
}

func (cs *ChunkSeries) Iterator(iterator chunks.Iterator) chunks.Iterator {
	if ci, ok := iterator.(*ChunkSeriesChunksIterator); ok {
		ci.ResetTo(cs.recodedChunks)
		return ci
	}
	return NewChunkSeriesChunksIterator(cs.recodedChunks)
}

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
