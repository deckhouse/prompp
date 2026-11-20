package shard

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/head/task"
	"github.com/prometheus/prometheus/storage"
)

func TestLoadAndQuerySeriesDataDiscardsDeferredQueries(t *testing.T) {
	t.Parallel()

	// Arrange
	path := filepath.Join(t.TempDir(), "unloaded")
	file := NewAppendFileStorage(path)
	t.Cleanup(func() { require.NoError(t, file.Close()) })

	ds := NewDataStorage(false, true)
	ds.Encode(0, 0, 1)
	ds.Encode(0, 5, 2)
	ds.Encode(0, 10, 3)

	unloaded := NewUnloadedDataStorage(file)
	unloader := ds.CreateUnusedSeriesDataUnloader()
	header, err := unloaded.WriteSnapshot(unloader.CreateSnapshot())
	require.NoError(t, err)
	require.Positive(t, header.SnapshotSize)
	unloaded.WriteIndex(header)
	unloader.Unload()

	// The file has a version byte and the snapshot. Remove the snapshot's last byte.
	require.NoError(t, os.Truncate(path, int64(header.SnapshotSize)))

	s := NewShard(nil, ds, unloaded, nil, nil, 0)
	result := ds.Query(cppbridge.DataStorageQuery{
		StartTimestampMs: 0,
		EndTimestampMs:   10,
		LabelSetIDs:      []uint32{0},
	}, cppbridge.NoDownsampling, nil)
	t.Cleanup(result.SerializedData.Close)
	require.Equal(t, cppbridge.DataStorageQueryStatusNeedDataLoad, result.Status)
	require.NotZero(t, result.Querier)

	loadTask := task.NewTransactionGeneric((*Shard).LoadAndQuerySeriesData)
	loadTask.SetShardsNumber(1)
	waiter := s.loadAndQueryTask.Add(result.Querier, func() Task { return loadTask })
	handles := s.loadAndQueryTask.queriers

	// Act
	loadTask.ExecuteOnShard(s)
	err = waiter.Wait()

	// Assert
	require.ErrorIs(t, err, io.EOF)
	require.True(t, result.SerializedData.IsClosed(), "failed load must not publish a result")
	require.Zero(t, handles[0], "failed load must destroy the deferred query before Wait returns")
}

func TestMultiSeriesIteratorCloseAfterSerializedDataClose(t *testing.T) {
	t.Parallel()

	// Arrange
	ds := NewDataStorage(false, true)
	ds.Encode(0, 50, 10)
	ds.Encode(1, 80, 20)
	ds.Encode(0, 150, 20)
	ds.Encode(1, 180, 30)

	ids := []uint32{0, 1}
	hints := &storage.SelectHints{Start: 1, End: 200, Step: 100, LookbackDelta: 100, Func: "sum"}
	result := ds.Query(cppbridge.DataStorageQuery{
		StartTimestampMs: hints.Start,
		EndTimestampMs:   hints.End,
		LabelSetIDs:      ids,
	}, cppbridge.NoDownsampling, hints)
	t.Cleanup(result.SerializedData.Close)
	require.Equal(t, cppbridge.DataStorageQueryStatusSuccess, result.Status)

	it := cppbridge.NewDataStorageSerializedDataMultiSeriesIterator(result.SerializedData, ids)
	firstSample := cppbridge.Sample{Timestamp: it.Timestamp(), Value: it.Value()}

	// Act: destroy the iterator with unread data after freeing its borrowed buffer.
	result.SerializedData.Close()
	it.Close()

	// Assert
	require.Equal(t, cppbridge.Sample{Timestamp: 100, Value: 30}, firstSample)
	require.True(t, result.SerializedData.IsClosed())
}
