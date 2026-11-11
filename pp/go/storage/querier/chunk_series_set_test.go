package querier

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

func TestChunkSeriesSetRetainsSeriesAfterAdvancement(t *testing.T) {
	// Arrange
	lss := cppbridge.NewQueryableLssStorage()
	seriesIDs := []uint32{
		lss.FindOrEmplace(model.NewLabelSetBuilder().Set(labels.MetricName, "first").Build()).LabelSetID,
		lss.FindOrEmplace(model.NewLabelSetBuilder().Set(labels.MetricName, "second").Build()).LabelSetID,
	}
	expected := [][]chunkSeriesTestSample{
		make([]chunkSeriesTestSample, 2*cppbridge.MaxPointsInChunk+3),
		make([]chunkSeriesTestSample, cppbridge.MaxPointsInChunk+5),
	}
	dataStorage := cppbridge.NewDataStorage(false, false)

	for seriesIndex, samples := range expected {
		for i := range samples {
			sample := chunkSeriesTestSample{
				timestamp: int64((seriesIndex+1)*10_000 + i),
				value:     float64((seriesIndex+1)*1_000+i) + 0.25,
			}
			samples[i] = sample
			dataStorage.Encode(seriesIDs[seriesIndex], sample.timestamp, sample.value)
		}
	}

	interval := cppbridge.TimeInterval{
		MinT: expected[0][0].timestamp,
		MaxT: expected[1][len(expected[1])-1].timestamp,
	}
	query := dataStorage.Query(cppbridge.DataStorageQuery{
		StartTimestampMs: interval.MinT,
		EndTimestampMs:   interval.MaxT,
		LabelSetIDs:      cppbridge.GoLabelSetIDs(seriesIDs),
	}, cppbridge.NoDownsampling, unsafe.Pointer(&storage.SelectHints{}))
	require.Equal(t, cppbridge.DataStorageQueryStatusSuccess, query.Status)
	recoder := cppbridge.NewSerializedChunkRecoder(query.SerializedData, interval)
	set := NewChunkSeriesSet(lss.CreateLabelSetSnapshot(), recoder)
	require.True(t, set.Next())
	first := set.At()
	iterator := first.Iterator(nil)
	var retained []chunks.Meta

	for iterator.Next() {
		retained = append(retained, iterator.At())
	}

	require.NoError(t, iterator.Err())

	// Act
	advanced := set.Next()
	second := set.At()
	exhausted := !set.Next()

	// Assert
	require.True(t, advanced)
	require.True(t, exhausted)
	require.NoError(t, set.Err())
	require.Equal(t, "first", first.Labels().Get(labels.MetricName))
	require.Equal(t, "second", second.Labels().Get(labels.MetricName))
	require.Len(t, retained, 3)

	for i, meta := range retained {
		start := i * cppbridge.MaxPointsInChunk
		end := min(start+cppbridge.MaxPointsInChunk, len(expected[0]))
		samples := expected[0][start:end]
		require.Equal(t, samples[0].timestamp, meta.MinTime)
		require.Equal(t, samples[len(samples)-1].timestamp, meta.MaxTime)
		require.Equal(t, samples, readChunkSeriesTestSamples(t, meta.Chunk))
	}

	secondIterator := second.Iterator(nil)
	var secondSamples []chunkSeriesTestSample
	chunkCount := 0

	for secondIterator.Next() {
		chunkCount++
		secondSamples = append(secondSamples, readChunkSeriesTestSamples(t, secondIterator.At().Chunk)...)
	}

	require.NoError(t, secondIterator.Err())
	require.Equal(t, 2, chunkCount)
	require.Equal(t, expected[1], secondSamples)
}
