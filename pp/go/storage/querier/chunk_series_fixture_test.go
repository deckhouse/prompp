package querier

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// chunkSeriesTestSample holds an expected float sample with a millisecond timestamp.
type chunkSeriesTestSample struct {
	timestamp int64
	value     float64
}

// makeChunkSeriesTestChunk encodes samples into an independently owned recoded XOR chunk.
func makeChunkSeriesTestChunk(t testing.TB, samples []chunkSeriesTestSample) cppbridge.RecodedChunk {
	t.Helper()
	require.NotEmpty(t, samples)

	chunk := chunkenc.NewXORChunk()
	appender, err := chunk.Appender()
	require.NoError(t, err)

	for _, sample := range samples {
		appender.Append(sample.timestamp, sample.value)
	}

	return cppbridge.RecodedChunk{
		TimeInterval: cppbridge.TimeInterval{MinT: samples[0].timestamp, MaxT: samples[len(samples)-1].timestamp},
		SamplesCount: uint8(len(samples)),
		ChunkData:    chunk.Bytes(),
	}
}

// newChunkSeriesTestSeries builds a series with immutable wrappers over the fixture payloads.
func newChunkSeriesTestSeries(recodedChunks []cppbridge.RecodedChunk) *ChunkSeries {
	series := &ChunkSeries{chunks: make([]chunkSeriesChunk, len(recodedChunks))}

	for i, recodedChunk := range recodedChunks {
		series.chunks[i] = newChunkSeriesChunk(recodedChunk)
	}

	return series
}

// readChunkSeriesTestSamples decodes a real XOR chunk into timestamp/value pairs.
func readChunkSeriesTestSamples(t testing.TB, chunk chunkenc.Chunk) []chunkSeriesTestSample {
	t.Helper()

	var samples []chunkSeriesTestSample
	iterator := chunk.Iterator(nil)

	for valueType := iterator.Next(); valueType != chunkenc.ValNone; valueType = iterator.Next() {
		require.Equal(t, chunkenc.ValFloat, valueType)
		timestamp, value := iterator.At()
		samples = append(samples, chunkSeriesTestSample{timestamp: timestamp, value: value})
	}

	require.NoError(t, iterator.Err())

	return samples
}
