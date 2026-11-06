package querier

import (
	"errors"
	"io"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/util/annotations"
)

// ChunkSeriesRemoteReadSuite checks chunk ownership across merging and streamed remote read.
type ChunkSeriesRemoteReadSuite struct {
	suite.Suite
	labelSet     labels.Labels
	blockSamples []chunkSeriesTestSample
	headSamples  [][]chunkSeriesTestSample
}

func TestChunkSeriesRemoteReadSuite(t *testing.T) {
	suite.Run(t, new(ChunkSeriesRemoteReadSuite))
}

func (s *ChunkSeriesRemoteReadSuite) SetupTest() {
	s.labelSet = labels.FromStrings(labels.MetricName, "chunk_series_remote_read")
	s.blockSamples = []chunkSeriesTestSample{{timestamp: 10, value: 1.25}}
	s.headSamples = [][]chunkSeriesTestSample{
		{{timestamp: 100, value: 11.5}, {timestamp: 130, value: 13.75}},
		{{timestamp: 200, value: 22.25}, {timestamp: 240, value: 24.5}, {timestamp: 290, value: 29.75}},
		{
			{timestamp: 400, value: 40.5},
			{timestamp: 460, value: 46.25},
			{timestamp: 530, value: 53.75},
			{timestamp: 610, value: 61.125},
		},
	}
}

func (s *ChunkSeriesRemoteReadSuite) TestEmptySet() {
	// Arrange
	response := httptest.NewRecorder()
	pool := &sync.Pool{}
	writer := remote.NewChunkedWriter(response, response)

	// Act
	warnings, err := remote.StreamChunkedReadResponses(writer, 0, storage.EmptyChunkSeriesSet(), nil, 1, pool)
	reader := remote.NewChunkedReader(response.Body, 1<<20, nil)
	_, readErr := reader.Next()

	// Assert
	s.Require().NoError(err)
	s.Empty(warnings)
	s.Require().ErrorIs(readErr, io.EOF)
	s.Empty(response.Body.Bytes())
}

func (s *ChunkSeriesRemoteReadSuite) TestStreams() {
	// Arrange
	cases := []chunkSeriesRemoteReadCase{
		{"block_two_heads_single_frame", true, 2, 1 << 20, 1},
		{"block_two_heads_multiple_frames", true, 2, 1, 3},
		{"block_three_heads_single_frame", true, 3, 1 << 20, 1},
		{"block_three_heads_multiple_frames", true, 3, 1, 4},
		{"head_only_single_frame", false, 3, 1 << 20, 1},
		{"head_only_multiple_frames", false, 3, 1, 3},
		{"single_head", false, 1, 1 << 20, 1},
		{"block_single_head_single_frame", true, 1, 1 << 20, 1},
		{"block_single_head_multiple_frames", true, 1, 1, 2},
	}

	// Act / Assert
	for _, testCase := range cases {
		s.Run(testCase.name, func() {
			s.checkStream(testCase)
		})
	}
}

func (s *ChunkSeriesRemoteReadSuite) checkStream(testCase chunkSeriesRemoteReadCase) {
	// Arrange
	recoded := make([]cppbridge.RecodedChunk, testCase.headCount)
	expectedChunks := make([][]chunkSeriesTestSample, 0, testCase.headCount+1)
	var blockSeries storage.ChunkSeries

	if testCase.withBlock {
		block := makeChunkSeriesTestChunk(s.T(), s.blockSamples)
		blockChunk, err := chunkenc.FromData(chunkenc.EncXOR, block.ChunkData)
		s.Require().NoError(err)
		meta := chunks.Meta{MinTime: block.MinT, MaxTime: block.MaxT, Chunk: blockChunk}
		blockSeries = &storage.ChunkSeriesEntry{
			Lset: s.labelSet,
			ChunkIteratorFn: func(chunks.Iterator) chunks.Iterator {
				return storage.NewListChunkSeriesIterator(meta)
			},
		}
		expectedChunks = append(expectedChunks, s.blockSamples)
	}

	for i := range recoded {
		recoded[i] = makeChunkSeriesTestChunk(s.T(), s.headSamples[i])
		expectedChunks = append(expectedChunks, s.headSamples[i])
	}

	var series storage.ChunkSeries = &ChunkSeries{labelSet: s.labelSet, recodedChunks: recoded}

	if testCase.withBlock {
		merger := storage.NewCompactingChunkSeriesMerger(storage.ChainedSeriesMerge)
		series = merger(blockSeries, series)
	}

	set := &chunkSeriesRemoteReadSet{series: series}
	response := httptest.NewRecorder()
	writer := remote.NewChunkedWriter(response, response)
	pool := &sync.Pool{}

	// Act
	warnings, err := remote.StreamChunkedReadResponses(writer, 7, set, nil, testCase.frameLimit, pool)

	// Assert
	s.Require().NoError(err)
	s.Empty(warnings)
	reader := remote.NewChunkedReader(response.Body, 1<<20, nil)
	var actualSamples, expectedSamples []chunkSeriesTestSample
	frameCount, chunkCount := 0, 0

	for _, samples := range expectedChunks {
		expectedSamples = append(expectedSamples, samples...)
	}

	for {
		var frame prompb.ChunkedReadResponse
		readErr := reader.NextProto(&frame)

		if errors.Is(readErr, io.EOF) {
			break
		}

		s.Require().NoError(readErr)
		frameCount++
		s.Equal(int64(7), frame.QueryIndex)
		s.Require().Len(frame.ChunkedSeries, 1)
		row := frame.ChunkedSeries[0]
		s.Equal(prompb.FromLabels(s.labelSet, nil), row.Labels)
		s.Require().NotEmpty(row.Chunks)

		for _, encoded := range row.Chunks {
			s.Require().Equal(prompb.Chunk_XOR, encoded.Type)
			chunk, decodeErr := chunkenc.FromData(chunkenc.EncXOR, encoded.Data)
			s.Require().NoError(decodeErr)
			samples := readChunkSeriesTestSamples(s.T(), chunk)
			s.Require().NotEmpty(samples)
			s.Equal(samples[0].timestamp, encoded.MinTimeMs)
			s.Equal(samples[len(samples)-1].timestamp, encoded.MaxTimeMs)
			s.Require().Less(chunkCount, len(expectedChunks))
			s.Equal(expectedChunks[chunkCount], samples)
			actualSamples = append(actualSamples, samples...)
			chunkCount++
		}
	}

	s.Equal(len(expectedChunks), chunkCount)
	s.Equal(expectedSamples, actualSamples)
	s.Equal(testCase.expectedFrames, frameCount)
}

// chunkSeriesRemoteReadCase describes a source combination and its expected stream framing.
type chunkSeriesRemoteReadCase struct {
	name           string
	withBlock      bool
	headCount      int
	frameLimit     int
	expectedFrames int
}

// chunkSeriesRemoteReadSet presents one real merged series to the remote read encoder.
type chunkSeriesRemoteReadSet struct {
	series storage.ChunkSeries
	done   bool
}

func (s *chunkSeriesRemoteReadSet) At() storage.ChunkSeries {
	return s.series
}

func (*chunkSeriesRemoteReadSet) Err() error {
	return nil
}

func (s *chunkSeriesRemoteReadSet) Next() bool {
	if s.done {
		return false
	}

	s.done = true

	return true
}

func (*chunkSeriesRemoteReadSet) Warnings() annotations.Annotations {
	return nil
}
