package shard_test

import (
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"testing"
	"time"
	"unsafe"
	"weak"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
)

// fakeWal is a minimal [shard.Wal] implementation used to observe lifecycle
// calls made by [shard.Shard].
type fakeWal struct {
	closeCalls       int
	commitCalls      int
	longCommitCalls  int
	flushCalls       int
	syncCalls        int
	writeCalls       int
	currentSizeCalls int

	closeErr  error
	commitErr error
	flushErr  error
	syncErr   error
	writeErr  error

	currentSize int64
}

func (m *fakeWal) Close() error {
	m.closeCalls++
	return m.closeErr
}

func (m *fakeWal) Commit() error {
	m.commitCalls++
	return m.commitErr
}

func (m *fakeWal) LongCommit() error {
	m.longCommitCalls++
	return m.commitErr
}

func (m *fakeWal) CurrentSize() int64 {
	m.currentSizeCalls++
	return m.currentSize
}

func (m *fakeWal) Flush() error {
	m.flushCalls++
	return m.flushErr
}

func (*fakeWal) IndependentCommit() bool {
	return false
}

func (m *fakeWal) Sync() error {
	m.syncCalls++
	return m.syncErr
}

func (m *fakeWal) Write([]cppbridge.InnerSeries) (bool, error) {
	m.writeCalls++
	return false, m.writeErr
}

func (m *fakeWal) WrittenSeriesIDSentinel() uint32 {
	return 0
}

type ShardSuite struct {
	suite.Suite
}

func TestShardSuite(t *testing.T) {
	suite.Run(t, new(ShardSuite))
}

func (s *ShardSuite) newShard(w shard.Wal) *shard.Shard {
	return shard.NewShard(nil, nil, nil, nil, w, 0)
}

func (s *ShardSuite) TestCloseWalClosesUnderlyingWal() {
	fw := &fakeWal{}
	sd := s.newShard(fw)

	s.Require().NoError(sd.CloseWal())
	s.Equal(1, fw.closeCalls)
}

func (s *ShardSuite) TestCloseWalIsIdempotent() {
	fw := &fakeWal{}
	sd := s.newShard(fw)

	s.Require().NoError(sd.CloseWal())
	s.Require().NoError(sd.CloseWal())
	s.Require().NoError(sd.CloseWal())

	s.Equal(1, fw.closeCalls, "underlying wal.Close must be called exactly once")
}

func (s *ShardSuite) TestCloseWalPropagatesError() {
	want := errors.New("boom")
	fw := &fakeWal{closeErr: want}
	sd := s.newShard(fw)

	s.Require().ErrorIs(sd.CloseWal(), want)

	// Even after an error the shard must refuse to close the wal again.
	s.Require().NoError(sd.CloseWal())
	s.Equal(1, fw.closeCalls)
}

func (s *ShardSuite) TestWalMethodsAfterCloseWalAreSafe() {
	fw := &fakeWal{currentSize: 42}
	sd := s.newShard(fw)
	s.Require().NoError(sd.CloseWal())

	// Data-handling methods report ErrWalClosed so accidental use-after-close
	// is noisy rather than silent.
	s.Require().ErrorIs(sd.WalCommit(), wal.ErrWalClosed)
	s.Require().ErrorIs(sd.WalLongCommit(), wal.ErrWalClosed)
	s.Require().ErrorIs(sd.WalFlush(), wal.ErrWalClosed)
	s.Require().ErrorIs(sd.WalSync(), wal.ErrWalClosed)

	ok, err := sd.WalWrite(nil)
	s.False(ok)
	s.Require().ErrorIs(err, wal.ErrWalClosed)

	// CurrentSize has no error channel; a closed WAL reports size 0.
	s.Zero(sd.WalCurrentSize())

	// None of the Wal* calls should reach the original wal.
	s.Equal(0, fw.commitCalls)
	s.Equal(0, fw.longCommitCalls)
	s.Equal(0, fw.flushCalls)
	s.Equal(0, fw.syncCalls)
	s.Equal(0, fw.currentSizeCalls)
	s.Equal(0, fw.writeCalls)
}

func (s *ShardSuite) TestWalLongCommitDelegatesToWalLongCommit() {
	want := errors.New("boom")
	fw := &fakeWal{commitErr: want}
	sd := s.newShard(fw)

	s.Require().ErrorIs(sd.WalLongCommit(), want)
	s.Equal(1, fw.longCommitCalls)
	s.Equal(0, fw.commitCalls, "WalLongCommit must not fall back to the regular Commit")
}

func (s *ShardSuite) TestCloseAfterCloseWalDoesNotDoubleClose() {
	fw := &fakeWal{}
	sd := s.newShard(fw)

	s.Require().NoError(sd.CloseWal())
	s.Require().NoError(sd.Close())

	s.Equal(1, fw.closeCalls, "Shard.Close must not re-close the wal")
}

func TestUnloadedDataLoaderCleanupAfterSnapshotReadError(t *testing.T) {
	// Arrange
	if cppbridge.GetMemInfo().Resident == 0 {
		t.Skip("requires jemalloc arena accounting")
	}

	path := filepath.Join(t.TempDir(), "unloaded")
	snapshots := shard.NewUnloadedDataStorage(shard.NewAppendFileStorage(path))
	t.Cleanup(func() { require.NoError(t, snapshots.Close()) })

	ds := cppbridge.NewDataStorage(false, true)
	loader := writeUnloadedSnapshotFixture(t, ds, snapshots)
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.NoError(t, os.Truncate(path, info.Size()-1))

	// Act
	arenaBytes := ds.AllocatedMemory()
	var loaded int
	err = snapshots.ForEachSnapshot(func(snapshot []byte, isLast bool) {
		require.False(t, isLast)
		loader.Load(snapshot, isLast)
		loaded++
	})

	// Assert
	require.ErrorIs(t, err, io.EOF)
	require.Equal(t, 1, loaded)
	require.Greater(t, ds.AllocatedMemory(), arenaBytes, "the first snapshot must populate native loader buffers")

	storageCleaned := make(chan struct{})
	runtime.AddCleanup(ds, func(done chan struct{}) { close(done) }, storageCleaned)
	loaderRef, storageRef := weak.Make(loader), weak.Make(ds)

	// Observe exactly one GC after dropping both owners, without an automatic GC
	// collecting the storage after loader cleanup but before we can probe it.
	func() {
		defer debug.SetGCPercent(debug.SetGCPercent(-1))
		defer debug.SetMemoryLimit(debug.SetMemoryLimit(math.MaxInt64))

		runtime.GC()
		runtime.KeepAlive(loader)
		runtime.KeepAlive(ds)
		loader, ds = nil, nil
		runtime.GC()

		require.Nil(t, loaderRef.Value(), "the loader must be eligible for cleanup")
		require.NotNil(t, storageRef.Value(), "loader cleanup must keep its arena alive")
	}()

	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for {
		runtime.GC()
		select {
		case <-storageCleaned:
			return
		case <-timeout.C:
			t.Fatal("the loader did not release its hold on data storage")
		default:
		}
	}
}

// writeUnloadedSnapshotFixture writes two native snapshots and prepares a loader interested in their series.
func writeUnloadedSnapshotFixture(
	t *testing.T,
	ds *cppbridge.DataStorage,
	snapshots *shard.UnloadedDataStorage,
) *cppbridge.UnloadedDataLoader {
	t.Helper()

	// Use enough series for the loader buffers to occupy additional arena pages.
	const seriesCount = 4096
	const samplesPerSnapshot = 5
	ids := make([]uint32, seriesCount)
	unloader := ds.CreateUnusedSeriesDataUnloader()

	for batch := range 2 {
		for i := range ids {
			ids[i] = uint32(i)
			for sample := range samplesPerSnapshot {
				timestamp := int64(batch*samplesPerSnapshot + sample + 1)
				ds.Encode(ids[i], timestamp, float64(timestamp))
			}
		}

		snapshot := unloader.CreateSnapshot()
		header, err := snapshots.WriteSnapshot(snapshot)
		require.NoError(t, err)
		require.NotZero(t, header.SnapshotSize)
		snapshots.WriteIndex(header)
		unloader.Unload()
	}

	samples := make([]querier.InstantSeries, len(ids))
	var pinner runtime.Pinner
	pinner.Pin(&ids[0])
	pinner.Pin(&samples[0])
	defer pinner.Unpin()

	query := ds.InstantQuery(2, ids, uintptr(unsafe.Pointer(&samples[0]))) // #nosec G103 -- shared C++ output buffer.
	require.Equal(t, cppbridge.DataStorageQueryStatusNeedDataLoad, query.Status)
	require.NotZero(t, query.Querier)

	queriers := []uintptr{query.Querier}
	loader := ds.CreateLoader(queriers)
	// The loader copied the requested series IDs. Dispose of the native querier
	// while the data is still marked unloaded, so it won't read the trimmed chunks.
	ds.QueryFinal(queriers)

	return loader
}
