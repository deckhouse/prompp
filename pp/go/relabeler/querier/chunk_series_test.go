package querier

import (
	"testing"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
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
	ls0ID := lss.FindOrEmplace(ls0)
	ls1 := model.NewLabelSetBuilder().Set("job", "test").Set("ls", "ls2").Build()
	ls1ID := lss.FindOrEmplace(ls1)

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

	lssQueryResult := lss.Query([]model.LabelMatcher{
		{Name: "job", Value: "test", MatcherType: model.MatcherTypeExactMatch},
	}, cppbridge.LSSQuerySourceFederate)

	require.Equal(s.T(), cppbridge.LSSQueryStatusMatch, lssQueryResult.Status())

	serializedChunks := ds.Query(cppbridge.HeadDataStorageQuery{
		StartTimestampMs: mint,
		EndTimestampMs:   maxt,
		LabelSetIDs:      lssQueryResult.IDs(),
	})

	require.Equal(s.T(), 2, serializedChunks.NumberOfChunks())

	chunkRecoder := cppbridge.NewSerializedChunkRecoder(serializedChunks, cppbridge.TimeInterval{
		MinT: mint,
		MaxT: maxt,
	})

	css := NewChunkSeriesSet(lssQueryResult, chunkRecoder)
	var ci chunks.Iterator

	// first series
	require.True(s.T(), css.Next())
	cs := css.At()

	ci = cs.Iterator(ci)
	require.True(s.T(), ci.Next())

	xorChunk := chunkenc.NewXORChunk()
	xorChunkAppender, err := xorChunk.Appender()
	require.NoError(s.T(), err)
	xorChunkAppender.Append(4, 30)
	xorChunkAppender.Append(7, 40)

	meta := ci.At()
	require.Equal(s.T(), int64(4), meta.MinTime)
	require.Equal(s.T(), int64(7), meta.MaxTime)
	require.Equal(s.T(), xorChunk.Bytes(), meta.Chunk.Bytes())
	require.False(s.T(), ci.Next())

	// second series
	require.True(s.T(), css.Next())
	cs = css.At()

	ci = cs.Iterator(ci)
	require.True(s.T(), ci.Next())

	xorChunk = chunkenc.NewXORChunk()
	xorChunkAppender, err = xorChunk.Appender()
	require.NoError(s.T(), err)
	xorChunkAppender.Append(2, 21)
	xorChunkAppender.Append(5, 31)
	xorChunkAppender.Append(8, 41)

	meta = ci.At()
	require.Equal(s.T(), int64(2), meta.MinTime)
	require.Equal(s.T(), int64(8), meta.MaxTime)
	require.Equal(s.T(), xorChunk.Bytes(), meta.Chunk.Bytes())
	require.False(s.T(), ci.Next())

	require.False(s.T(), css.Next())
}
