package shard_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
)

// fakeWal is a minimal [shard.Wal] implementation used to observe lifecycle
// calls made by [shard.Shard].
type fakeWal struct {
	closeCalls       int
	commitCalls      int
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

func (m *fakeWal) CurrentSize() int64 {
	m.currentSizeCalls++
	return m.currentSize
}

func (m *fakeWal) Flush() error {
	m.flushCalls++
	return m.flushErr
}

func (m *fakeWal) Sync() error {
	m.syncCalls++
	return m.syncErr
}

func (m *fakeWal) Write([]cppbridge.InnerSeries) (bool, error) {
	m.writeCalls++
	return false, m.writeErr
}

func (m *fakeWal) MaxWrittenItemIndex() uint32 {
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
	s.Require().ErrorIs(sd.WalFlush(), wal.ErrWalClosed)
	s.Require().ErrorIs(sd.WalSync(), wal.ErrWalClosed)

	ok, err := sd.WalWrite(nil)
	s.False(ok)
	s.Require().ErrorIs(err, wal.ErrWalClosed)

	// CurrentSize has no error channel; a closed WAL reports size 0.
	s.Zero(sd.WalCurrentSize())

	// None of the Wal* calls should reach the original wal.
	s.Equal(0, fw.commitCalls)
	s.Equal(0, fw.flushCalls)
	s.Equal(0, fw.syncCalls)
	s.Equal(0, fw.currentSizeCalls)
	s.Equal(0, fw.writeCalls)
}

func (s *ShardSuite) TestCloseAfterCloseWalDoesNotDoubleClose() {
	fw := &fakeWal{}
	sd := s.newShard(fw)

	s.Require().NoError(sd.CloseWal())
	s.Require().NoError(sd.Close())

	s.Equal(1, fw.closeCalls, "Shard.Close must not re-close the wal")
}
