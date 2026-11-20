package querier_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-kit/log"
	"github.com/golang/snappy"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

// streamLifetimeQuerier exposes the real merge result selected by the remote read handler.
type streamLifetimeQuerier struct {
	storage.ChunkQuerier
	selected storage.ChunkSeriesSet
}

func (q *streamLifetimeQuerier) Select(
	ctx context.Context,
	sortSeries bool,
	hints *storage.SelectHints,
	matchers ...*labels.Matcher,
) storage.ChunkSeriesSet {
	q.selected = q.ChunkQuerier.Select(ctx, sortSeries, hints, matchers...)

	return q.selected
}

// streamLifetimeQueryable gives the remote read handler a prearranged chunk querier.
type streamLifetimeQueryable struct {
	storage.Queryable
	querier *streamLifetimeQuerier
}

func (q streamLifetimeQueryable) ChunkQuerier(int64, int64) (storage.ChunkQuerier, error) {
	return q.querier, nil
}

// streamLifetimeRecorder can observe or abort the first attempt to write a streamed frame.
type streamLifetimeRecorder struct {
	*httptest.ResponseRecorder
	onFirstWrite func() error
}

func (w *streamLifetimeRecorder) Write(data []byte) (int, error) {
	if w.onFirstWrite != nil {
		onFirstWrite := w.onFirstWrite
		w.onFirstWrite = nil
		if err := onFirstWrite(); err != nil {
			return 0, err
		}
	}

	return w.ResponseRecorder.Write(data)
}

func TestMergedChunkQuerierRemoteReadClose(t *testing.T) {
	t.Parallel()

	leftHead, rightHead := newMergeLifetimeHeads(t)
	for _, tc := range []struct {
		name  string
		abort bool
	}{
		{name: "completed stream"},
		{name: "aborted stream", abort: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			const frameLimit = 1 << 20
			writeErr := errors.New("remote read test write failed")
			left := querier.NewChunkQuerier(
				leftHead,
				querier.NewNoOpShardedDeduplicator,
				0,
				mergeLifetimeMaxt,
				nil,
			)
			right := querier.NewChunkQuerier(
				rightHead,
				querier.NewNoOpShardedDeduplicator,
				0,
				mergeLifetimeMaxt,
				nil,
			)
			merged := querier.NewMergedChunkQuerier([]storage.ChunkQuerier{left, right})
			t.Cleanup(func() { require.NoError(t, merged.Close()) })

			ctx := t.Context()
			match := labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, mergeLifetimeMetric)
			leftResult := left.Select(ctx, true, nil, match)
			rightResult := right.Select(ctx, true, nil, match)
			require.True(t, leftResult.Next())
			require.True(t, rightResult.Next())
			unstarted := merged.Select(ctx, true, nil, match)
			observed := &streamLifetimeQuerier{ChunkQuerier: merged}
			handler := remote.NewReadHandler(
				log.NewNopLogger(),
				nil,
				streamLifetimeQueryable{querier: observed},
				func() config.Config { return config.DefaultConfig },
				0,
				1,
				frameLimit,
			)

			query, err := remote.ToQuery(0, mergeLifetimeMaxt, []*labels.Matcher{match}, nil)
			require.NoError(t, err)
			request := &prompb.ReadRequest{
				Queries:               []*prompb.Query{query},
				AcceptedResponseTypes: []prompb.ReadRequest_ResponseType{prompb.ReadRequest_STREAMED_XOR_CHUNKS},
			}
			data, err := request.Marshal()
			require.NoError(t, err)
			httpRequest := httptest.NewRequestWithContext(
				ctx,
				http.MethodPost,
				"/api/v1/read",
				bytes.NewReader(snappy.Encode(nil, data)),
			)
			var retainedLabels labels.Labels
			var retainedChunk chunks.Meta
			response := &streamLifetimeRecorder{
				ResponseRecorder: httptest.NewRecorder(),
				onFirstWrite: func() error {
					series := observed.selected.At()
					retainedLabels = series.Labels()
					iter := series.Iterator(nil)
					require.True(t, iter.Next())
					retainedChunk = iter.At()
					if tc.abort {
						return writeErr
					}

					return nil
				},
			}

			// Act
			handler.ServeHTTP(response, httpRequest)

			// Assert
			require.NotNil(t, observed.selected)
			require.False(t, observed.selected.Next())
			require.Nil(t, observed.selected.At())
			require.False(t, unstarted.Next())
			require.False(t, leftResult.Next())
			require.False(t, rightResult.Next())
			require.Equal(t, mergeLifetimeLabels("a"), retainedLabels)
			requireLifetimeChunk(t, retainedChunk, 1)
			require.NoError(t, merged.Close())

			if tc.abort {
				require.Equal(t, http.StatusInternalServerError, response.Code)
				require.Contains(t, response.Body.String(), writeErr.Error())
				return
			}

			require.Equal(t, http.StatusOK, response.Code)
			require.True(t, response.Flushed)
			requireLifetimeFrames(t, remote.NewChunkedReader(response.Body, frameLimit, nil))
		})
	}
}

func requireLifetimeChunk(t *testing.T, meta chunks.Meta, firstValue float64) {
	t.Helper()

	require.NotNil(t, meta.Chunk)
	require.Equal(t, int64(1), meta.MinTime)
	require.Equal(t, int64(2), meta.MaxTime)
	iter := meta.Chunk.Iterator(nil)
	var samples []cppbridge.Sample
	for typ := iter.Next(); typ != chunkenc.ValNone; typ = iter.Next() {
		require.Equal(t, chunkenc.ValFloat, typ)
		timestamp, value := iter.At()
		samples = append(samples, cppbridge.Sample{Timestamp: timestamp, Value: value})
	}
	require.NoError(t, iter.Err())
	require.Equal(t, []cppbridge.Sample{
		{Timestamp: 1, Value: firstValue},
		{Timestamp: 2, Value: firstValue + 1},
	}, samples)
}

func requireLifetimeFrames(t *testing.T, reader *remote.ChunkedReader) {
	t.Helper()

	for i, id := range mergeLifetimeSeriesIDs {
		var frame prompb.ChunkedReadResponse
		require.NoError(t, reader.NextProto(&frame))
		require.Len(t, frame.ChunkedSeries, 1)
		row := frame.ChunkedSeries[0]
		require.Equal(t, prompb.FromLabels(mergeLifetimeLabels(string(id)), nil), row.Labels)
		require.Len(t, row.Chunks, 1)
		encoded := row.Chunks[0]
		require.Equal(t, prompb.Chunk_XOR, encoded.Type)
		chunk, err := chunkenc.FromData(chunkenc.EncXOR, encoded.Data)
		require.NoError(t, err)
		meta := chunks.Meta{MinTime: encoded.MinTimeMs, MaxTime: encoded.MaxTimeMs, Chunk: chunk}
		requireLifetimeChunk(t, meta, float64(i+1))
	}
	var frame prompb.ChunkedReadResponse
	require.ErrorIs(t, reader.NextProto(&frame), io.EOF)
}
