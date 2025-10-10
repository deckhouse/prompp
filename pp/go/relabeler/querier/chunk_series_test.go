package querier

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

type ChunksSeriesSetTestSuite struct {
	suite.Suite
}

func TestChunksSeriesSetTestSuite(t *testing.T) {
	suite.Run(t, new(ChunksSeriesSetTestSuite))
}

func (s *ChunksSeriesSetTestSuite) TestAll() {
	lss := cppbridge.NewQueryableLssStorage()
	ls0 := model.NewLabelSetBuilder().Set("job", "test").Set("ls", "ls1").Build()
	ls0ID := lss.FindOrEmplace(ls0).LabelSetID
	ls1 := model.NewLabelSetBuilder().Set("job", "test").Set("ls", "ls2").Build()
	ls1ID := lss.FindOrEmplace(ls1).LabelSetID

	ds := cppbridge.NewHeadDataStorage()
	encoder := cppbridge.NewHeadEncoderWithDataStorage(ds)

	encoder.Encode(ls0ID, 1, 20)
	encoder.Encode(ls0ID, 4, 30)
	encoder.Encode(ls0ID, 7, 40)
	encoder.Encode(ls0ID, 10, 50)

	encoder.Encode(ls1ID, 2, 21)
	encoder.Encode(ls1ID, 5, 31)
	encoder.Encode(ls1ID, 8, 41)
	encoder.Encode(ls1ID, 11, 51)

	var mint int64 = 2
	var maxt int64 = 8

	selector, status := lss.QuerySelector([]model.LabelMatcher{
		{Name: "job", Value: "test", MatcherType: model.MatcherTypeExactMatch},
	})
	s.Require().Equal(cppbridge.LSSQueryStatusMatch, status)

	labelSetSnapshot := cppbridge.NewLSSWithSnapshotWithoutBitset(lss).Snapshot()
	lssQueryResult := labelSetSnapshot.Query(selector)
	s.Require().Equal(cppbridge.LSSQueryStatusMatch, lssQueryResult.Status())

	serializedChunks, result := ds.Query(cppbridge.HeadDataStorageQuery{
		StartTimestampMs: mint,
		EndTimestampMs:   maxt,
		LabelSetIDs:      lssQueryResult.IDs(),
	})

	s.Require().Equal(cppbridge.DataStorageQueryStatusSuccess, result.Status)
	s.Require().Equal(2, serializedChunks.NumberOfChunks())

	chunkRecoder := cppbridge.NewSerializedChunkRecoder(serializedChunks, cppbridge.TimeInterval{
		MinT: mint,
		MaxT: maxt,
	})

	css := NewChunkSeriesSet(lssQueryResult, labelSetSnapshot, chunkRecoder)
	var ci chunks.Iterator

	// first series
	s.Require().True(css.Next())
	cs := css.At()

	ci = cs.Iterator(ci)
	s.Require().True(ci.Next())

	xorChunk := chunkenc.NewXORChunk()
	xorChunkAppender, err := xorChunk.Appender()
	s.Require().NoError(err)
	xorChunkAppender.Append(4, 30)
	xorChunkAppender.Append(7, 40)

	meta := ci.At()
	s.Require().Equal(int64(4), meta.MinTime)
	s.Require().Equal(int64(7), meta.MaxTime)
	s.Require().Equal(xorChunk.Bytes(), meta.Chunk.Bytes())
	s.Require().False(ci.Next())

	// second series
	s.Require().True(css.Next())
	cs = css.At()

	ci = cs.Iterator(ci)
	s.Require().True(ci.Next())

	xorChunk = chunkenc.NewXORChunk()
	xorChunkAppender, err = xorChunk.Appender()
	s.Require().NoError(err)
	xorChunkAppender.Append(2, 21)
	xorChunkAppender.Append(5, 31)
	xorChunkAppender.Append(8, 41)

	meta = ci.At()
	s.Require().Equal(int64(2), meta.MinTime)
	s.Require().Equal(int64(8), meta.MaxTime)
	s.Require().Equal(xorChunk.Bytes(), meta.Chunk.Bytes())
	s.Require().False(ci.Next())
	s.Require().False(css.Next())
}
