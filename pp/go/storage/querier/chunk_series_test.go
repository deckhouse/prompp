package querier

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

// ChunkSeriesChunksIteratorSuite checks the lifetime of chunks retained by consumers.
type ChunkSeriesChunksIteratorSuite struct {
	suite.Suite
	expected [][]chunkSeriesTestSample
	recoded  []cppbridge.RecodedChunk
}

func TestChunkSeriesChunksIteratorSuite(t *testing.T) {
	suite.Run(t, new(ChunkSeriesChunksIteratorSuite))
}

func (s *ChunkSeriesChunksIteratorSuite) SetupTest() {
	s.expected = [][]chunkSeriesTestSample{
		{{timestamp: 10, value: 1}, {timestamp: 11, value: 2}},
		{{timestamp: 20, value: 3}, {timestamp: 21, value: 4}, {timestamp: 22, value: 5}},
		{{timestamp: 30, value: 6}},
	}
	s.recoded = make([]cppbridge.RecodedChunk, len(s.expected))

	for i, samples := range s.expected {
		s.recoded[i] = makeChunkSeriesTestChunk(s.T(), samples)
	}
}

func (s *ChunkSeriesChunksIteratorSuite) TestAtRetainsChunkAfterNext() {
	// Arrange
	iterator := NewChunkSeriesChunksIterator(s.recoded)
	s.Require().True(iterator.Next())
	first := iterator.At()

	// Act
	advanced := iterator.Next()

	// Assert
	s.Require().True(advanced)
	s.Equal(int64(10), first.MinTime)
	s.Equal(int64(11), first.MaxTime)
	s.Equal(s.expected[0], readChunkSeriesTestSamples(s.T(), first.Chunk))
	s.Equal(s.expected[1], readChunkSeriesTestSamples(s.T(), iterator.At().Chunk))
	s.Same(&s.recoded[0].ChunkData[0], &first.Chunk.Bytes()[0])
}

func (s *ChunkSeriesChunksIteratorSuite) TestIndependentIterators() {
	// Arrange
	series := &ChunkSeries{recodedChunks: s.recoded}
	first := series.Iterator(nil)
	second := series.Iterator(nil)
	s.Require().True(first.Next())
	s.Require().True(second.Next())
	retained := second.At()

	// Act
	advanced := first.Next()

	// Assert
	s.Require().True(advanced)
	s.Equal(s.expected[0], readChunkSeriesTestSamples(s.T(), retained.Chunk))
	s.Equal(s.expected[0], readChunkSeriesTestSamples(s.T(), second.At().Chunk))
	s.Equal(s.expected[1], readChunkSeriesTestSamples(s.T(), first.At().Chunk))
}

func (s *ChunkSeriesChunksIteratorSuite) TestIteratorReuseRetainsPreviousSeries() {
	// Arrange
	firstSeries := &ChunkSeries{recodedChunks: s.recoded}
	iterator := firstSeries.Iterator(nil)
	s.Require().True(iterator.Next())
	retained := iterator.At()
	expected := []chunkSeriesTestSample{{timestamp: 100, value: 42}}
	secondSeries := &ChunkSeries{
		recodedChunks: []cppbridge.RecodedChunk{makeChunkSeriesTestChunk(s.T(), expected)},
	}

	// Act
	reused := secondSeries.Iterator(iterator)
	advanced := reused.Next()

	// Assert
	s.Same(iterator, reused)
	s.Require().True(advanced)
	s.Equal(s.expected[0], readChunkSeriesTestSamples(s.T(), retained.Chunk))
	s.Equal(expected, readChunkSeriesTestSamples(s.T(), reused.At().Chunk))
}

func (s *ChunkSeriesChunksIteratorSuite) TestRepeatedAtReturnsCurrentChunk() {
	// Arrange
	iterator := NewChunkSeriesChunksIterator(s.recoded)
	s.Require().True(iterator.Next())
	first := iterator.At()

	// Act
	repeated := iterator.At()

	// Assert
	s.Same(first.Chunk, repeated.Chunk)
	s.Equal(s.expected[0], readChunkSeriesTestSamples(s.T(), repeated.Chunk))
}

func (s *ChunkSeriesChunksIteratorSuite) TestResetToRetainsPreviousChunks() {
	// Arrange
	iterator := NewChunkSeriesChunksIterator(s.recoded)
	s.Require().True(iterator.Next())
	retained := iterator.At()
	expected := []chunkSeriesTestSample{{timestamp: 100, value: 42}}
	replacement := []cppbridge.RecodedChunk{makeChunkSeriesTestChunk(s.T(), expected)}

	// Act
	iterator.ResetTo(replacement)
	advanced := iterator.Next()

	// Assert
	s.Require().True(advanced)
	s.Equal(s.expected[0], readChunkSeriesTestSamples(s.T(), retained.Chunk))
	s.Equal(expected, readChunkSeriesTestSamples(s.T(), iterator.At().Chunk))
}

func (s *ChunkSeriesChunksIteratorSuite) TestRetainedChunksAfterExhaustion() {
	// Arrange
	iterator := NewChunkSeriesChunksIterator(s.recoded)
	var retained []chunks.Meta

	// Act
	for iterator.Next() {
		retained = append(retained, iterator.At())
	}

	// Assert
	s.Require().NoError(iterator.Err())
	s.Require().Len(retained, len(s.expected))

	for i, meta := range retained {
		s.Equal(s.expected[i], readChunkSeriesTestSamples(s.T(), meta.Chunk))
		s.Equal(s.recoded[i].MinT, meta.MinTime)
		s.Equal(s.recoded[i].MaxT, meta.MaxTime)
	}
}
