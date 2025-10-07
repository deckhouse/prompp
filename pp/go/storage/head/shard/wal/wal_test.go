package wal_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
)

type WalSuite struct {
	suite.Suite
}

func TestWalSuite(t *testing.T) {
	suite.Run(t, new(WalSuite))
}

func (s *WalSuite) TestCurrentSize() {
	expectedWalSize := int64(42)
	enc := &EncoderMock[*EncodedSegmentMock]{}
	segmentWriter := &SegmentWriterMock[*EncodedSegmentMock]{
		CurrentSizeFunc: func() int64 { return expectedWalSize },
		CloseFunc:       func() error { return nil },
	}
	maxSegmentSize := uint32(100)

	wl := wal.NewWal(enc, segmentWriter, maxSegmentSize)

	s.Equal(expectedWalSize, wl.CurrentSize())

	s.Require().NoError(wl.Close())
	s.Len(segmentWriter.CloseCalls(), 1)
}

func (s *WalSuite) TestClose() {
	enc := &EncoderMock[*EncodedSegmentMock]{}
	segmentWriter := &SegmentWriterMock[*EncodedSegmentMock]{
		CloseFunc: func() error { return nil },
	}
	maxSegmentSize := uint32(100)

	wl := wal.NewWal(enc, segmentWriter, maxSegmentSize)

	s.Require().NoError(wl.Close())
	s.Len(segmentWriter.CloseCalls(), 1)

	s.Require().NoError(wl.Close())
	s.Len(segmentWriter.CloseCalls(), 1)
}

func (s *WalSuite) TestCloseError() {
	expectedError := errors.New("test error")
	enc := &EncoderMock[*EncodedSegmentMock]{}
	segmentWriter := &SegmentWriterMock[*EncodedSegmentMock]{
		CloseFunc: func() error { return expectedError },
	}
	maxSegmentSize := uint32(100)

	wl := wal.NewWal(enc, segmentWriter, maxSegmentSize)

	s.Require().ErrorIs(wl.Close(), expectedError)
	s.Len(segmentWriter.CloseCalls(), 1)
}

func (s *WalSuite) TestCommit() {
	enc := &EncoderMock[*EncodedSegmentMock]{
		FinalizeFunc: func() (*EncodedSegmentMock, error) { return &EncodedSegmentMock{}, nil },
	}
	segmentWriter := &SegmentWriterMock[*EncodedSegmentMock]{
		WriteFunc: func(*EncodedSegmentMock) error { return nil },
	}
	maxSegmentSize := uint32(100)

	wl := wal.NewWal(enc, segmentWriter, maxSegmentSize)

	s.Require().NoError(wl.Commit())
	s.Len(enc.FinalizeCalls(), 1)
	s.Len(segmentWriter.WriteCalls(), 1)
}

func (s *WalSuite) TestCommitEncodeError() {
	expectedError := errors.New("test error")
	enc := &EncoderMock[*EncodedSegmentMock]{
		FinalizeFunc: func() (*EncodedSegmentMock, error) { return &EncodedSegmentMock{}, expectedError },
	}
	segmentWriter := &SegmentWriterMock[*EncodedSegmentMock]{
		WriteFunc: func(*EncodedSegmentMock) error { return nil },
	}
	maxSegmentSize := uint32(100)

	wl := wal.NewWal(enc, segmentWriter, maxSegmentSize)

	s.Require().ErrorIs(wl.Commit(), expectedError)
	s.Len(enc.FinalizeCalls(), 1)
	s.Empty(segmentWriter.WriteCalls())
}

func (s *WalSuite) TestCommitWriteError() {
	expectedError := errors.New("test error")
	enc := &EncoderMock[*EncodedSegmentMock]{
		FinalizeFunc: func() (*EncodedSegmentMock, error) { return &EncodedSegmentMock{}, nil },
	}
	segmentWriter := &SegmentWriterMock[*EncodedSegmentMock]{
		WriteFunc: func(*EncodedSegmentMock) error { return expectedError },
	}
	maxSegmentSize := uint32(100)

	wl := wal.NewWal(enc, segmentWriter, maxSegmentSize)

	s.Require().ErrorIs(wl.Commit(), expectedError)
	s.Len(enc.FinalizeCalls(), 1)
	s.Len(segmentWriter.WriteCalls(), 1)
}

func (s *WalSuite) TestFlush() {
	enc := &EncoderMock[*EncodedSegmentMock]{}
	segmentWriter := &SegmentWriterMock[*EncodedSegmentMock]{
		FlushFunc: func() error { return nil },
	}
	maxSegmentSize := uint32(100)

	wl := wal.NewWal(enc, segmentWriter, maxSegmentSize)

	s.Require().NoError(wl.Flush())
	s.Len(segmentWriter.FlushCalls(), 1)
}

func (s *WalSuite) TestFlushError() {
	expectedError := errors.New("test error")
	enc := &EncoderMock[*EncodedSegmentMock]{}
	segmentWriter := &SegmentWriterMock[*EncodedSegmentMock]{
		FlushFunc: func() error { return expectedError },
	}
	maxSegmentSize := uint32(100)

	wl := wal.NewWal(enc, segmentWriter, maxSegmentSize)

	s.Require().ErrorIs(wl.Flush(), expectedError)
	s.Len(segmentWriter.FlushCalls(), 1)
}

func (s *WalSuite) TestSync() {
	enc := &EncoderMock[*EncodedSegmentMock]{}
	segmentWriter := &SegmentWriterMock[*EncodedSegmentMock]{
		SyncFunc: func() error { return nil },
	}
	maxSegmentSize := uint32(100)

	wl := wal.NewWal(enc, segmentWriter, maxSegmentSize)

	s.Require().NoError(wl.Sync())
	s.Len(segmentWriter.SyncCalls(), 1)
}

func (s *WalSuite) TestSyncError() {
	expectedError := errors.New("test error")
	enc := &EncoderMock[*EncodedSegmentMock]{}
	segmentWriter := &SegmentWriterMock[*EncodedSegmentMock]{
		SyncFunc: func() error { return expectedError },
	}
	maxSegmentSize := uint32(100)

	wl := wal.NewWal(enc, segmentWriter, maxSegmentSize)

	s.Require().ErrorIs(wl.Sync(), expectedError)
	s.Len(segmentWriter.SyncCalls(), 1)
}

func (s *WalSuite) TestWrite() {
	enc := &EncoderMock[*EncodedSegmentMock]{
		EncodeFunc: func([]*cppbridge.InnerSeries) (uint32, error) { return 100, nil },
	}
	segmentWriter := &SegmentWriterMock[*EncodedSegmentMock]{
		CloseFunc: func() error { return nil },
	}

	maxSegmentSize := uint32(0)
	wl := wal.NewWal(enc, segmentWriter, maxSegmentSize)

	limitExhausted, err := wl.Write([]*cppbridge.InnerSeries{})
	s.Require().NoError(err)
	s.Len(enc.EncodeCalls(), 1)
	s.False(limitExhausted)

	s.Require().NoError(wl.Close())
	s.Len(segmentWriter.CloseCalls(), 1)
}

func (s *WalSuite) TestWriteLimitExhausted() {
	maxSegmentSize := uint32(100)
	enc := &EncoderMock[*EncodedSegmentMock]{
		EncodeFunc: func([]*cppbridge.InnerSeries) (uint32, error) { return maxSegmentSize, nil },
	}
	segmentWriter := &SegmentWriterMock[*EncodedSegmentMock]{
		CloseFunc: func() error { return nil },
	}

	wl := wal.NewWal(enc, segmentWriter, maxSegmentSize)

	limitExhausted, err := wl.Write([]*cppbridge.InnerSeries{})
	s.Require().NoError(err)
	s.Len(enc.EncodeCalls(), 1)
	s.True(limitExhausted)

	s.Require().NoError(wl.Close())
	s.Len(segmentWriter.CloseCalls(), 1)
}

func (s *WalSuite) TestWriteLimitNotExhausted() {
	maxSegmentSize := uint32(100)
	enc := &EncoderMock[*EncodedSegmentMock]{
		EncodeFunc: func([]*cppbridge.InnerSeries) (uint32, error) { return maxSegmentSize / 2, nil },
	}
	segmentWriter := &SegmentWriterMock[*EncodedSegmentMock]{
		CloseFunc: func() error { return nil },
	}

	wl := wal.NewWal(enc, segmentWriter, maxSegmentSize)

	limitExhausted, err := wl.Write([]*cppbridge.InnerSeries{})
	s.Require().NoError(err)
	s.Len(enc.EncodeCalls(), 1)
	s.False(limitExhausted)

	s.Require().NoError(wl.Close())
	s.Len(segmentWriter.CloseCalls(), 1)
}

func (s *WalSuite) TestWriteError() {
	maxSegmentSize := uint32(100)
	expectedError := errors.New("test error")
	enc := &EncoderMock[*EncodedSegmentMock]{
		EncodeFunc: func([]*cppbridge.InnerSeries) (uint32, error) { return maxSegmentSize / 2, expectedError },
	}
	segmentWriter := &SegmentWriterMock[*EncodedSegmentMock]{
		CloseFunc: func() error { return nil },
	}

	wl := wal.NewWal(enc, segmentWriter, maxSegmentSize)

	limitExhausted, err := wl.Write([]*cppbridge.InnerSeries{})
	s.Require().ErrorIs(err, expectedError)
	s.Len(enc.EncodeCalls(), 1)
	s.False(limitExhausted)

	s.Require().NoError(wl.Close())
	s.Len(segmentWriter.CloseCalls(), 1)
}

func (s *WalSuite) TestCorrupted() {
	wl := wal.NewCorruptedWal[*EncodedSegmentMock, *SegmentWriterMock[*EncodedSegmentMock]]()
	s.Equal(int64(0), wl.CurrentSize())

	limitExhausted, err := wl.Write([]*cppbridge.InnerSeries{})
	s.Require().ErrorIs(err, wal.ErrWalIsCorrupted)
	s.False(limitExhausted)

	err = wl.Commit()
	s.Require().ErrorIs(err, wal.ErrWalIsCorrupted)

	err = wl.Flush()
	s.Require().NoError(err)

	err = wl.Sync()
	s.Require().ErrorIs(err, wal.ErrWalIsCorrupted)

	s.Require().NoError(wl.Close())
}
