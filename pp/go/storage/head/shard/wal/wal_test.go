package wal_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
	"github.com/prometheus/prometheus/pp/go/util/locker"
)

type WalSuite struct {
	suite.Suite

	locker locker.RLockable
}

func TestWalSuite(t *testing.T) {
	suite.Run(t, new(WalSuite))
}

func (s *WalSuite) SetupSuite() {
	s.locker = locker.NoopLocker{}
}

func (s *WalSuite) TestCurrentSize() {
	expectedWalSize := int64(42)
	enc := &EncoderMock[*EncodedSegmentMock]{}
	segmentWriter := &SegmentWriterMock[*EncodedSegmentMock]{
		CurrentSizeFunc: func() int64 { return expectedWalSize },
		CloseFunc:       func() error { return nil },
	}
	maxSegmentSize := uint32(100)

	wl := wal.NewWal(enc, segmentWriter, s.locker, maxSegmentSize, 0, nil)

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

	wl := wal.NewWal(enc, segmentWriter, s.locker, maxSegmentSize, 0, nil)

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

	wl := wal.NewWal(enc, segmentWriter, s.locker, maxSegmentSize, 0, nil)

	s.Require().ErrorIs(wl.Close(), expectedError)
	s.Len(segmentWriter.CloseCalls(), 1)
}

type testWal = wal.Wal[*EncodedSegmentMock, *SegmentWriterMock[*EncodedSegmentMock]]

// commitVariant is one of the [wal.Wal] commit methods with the [wal.Encoder] finalize method it must use.
type commitVariant struct {
	name string
	// commit calls the commit method under test.
	commit func(*testWal) error
	// newEncoder returns an encoder whose expected finalize method is mocked by finalize,
	// and the other one is left nil, so a call to it panics.
	newEncoder func(finalize func() (*EncodedSegmentMock, error)) *EncoderMock[*EncodedSegmentMock]
	// finalizeCalls returns the number of calls of the expected finalize method.
	finalizeCalls func(*EncoderMock[*EncodedSegmentMock]) int
}

var commitVariants = []commitVariant{
	{
		name:   "Commit",
		commit: (*testWal).Commit,
		newEncoder: func(finalize func() (*EncodedSegmentMock, error)) *EncoderMock[*EncodedSegmentMock] {
			return &EncoderMock[*EncodedSegmentMock]{FinalizeFunc: finalize}
		},
		finalizeCalls: func(enc *EncoderMock[*EncodedSegmentMock]) int { return len(enc.FinalizeCalls()) },
	},
	{
		name:   "LongCommit",
		commit: (*testWal).LongCommit,
		newEncoder: func(finalize func() (*EncodedSegmentMock, error)) *EncoderMock[*EncodedSegmentMock] {
			return &EncoderMock[*EncodedSegmentMock]{LongFinalizeFunc: finalize}
		},
		finalizeCalls: func(enc *EncoderMock[*EncodedSegmentMock]) int { return len(enc.LongFinalizeCalls()) },
	},
}

func (s *WalSuite) TestCommit() {
	for _, variant := range commitVariants {
		s.Run(variant.name, func() {
			enc := variant.newEncoder(func() (*EncodedSegmentMock, error) {
				return &EncodedSegmentMock{
					SamplesFunc: func() uint32 { return 42 },
				}, nil
			})
			segmentWriter := &SegmentWriterMock[*EncodedSegmentMock]{
				WriteFunc: func(*EncodedSegmentMock) error { return nil },
			}
			maxSegmentSize := uint32(100)

			wl := wal.NewWal(enc, segmentWriter, s.locker, maxSegmentSize, 0, nil)

			s.Require().NoError(variant.commit(wl))
			s.Equal(1, variant.finalizeCalls(enc))
			s.Len(segmentWriter.WriteCalls(), 1)
		})
	}
}

func (s *WalSuite) TestCommitEncodeError() {
	for _, variant := range commitVariants {
		s.Run(variant.name, func() {
			expectedError := errors.New("test error")
			enc := variant.newEncoder(func() (*EncodedSegmentMock, error) { return &EncodedSegmentMock{}, expectedError })
			segmentWriter := &SegmentWriterMock[*EncodedSegmentMock]{
				WriteFunc: func(*EncodedSegmentMock) error { return nil },
			}
			maxSegmentSize := uint32(100)

			wl := wal.NewWal(enc, segmentWriter, s.locker, maxSegmentSize, 0, nil)

			s.Require().ErrorIs(variant.commit(wl), expectedError)
			s.Equal(1, variant.finalizeCalls(enc))
			s.Empty(segmentWriter.WriteCalls())
		})
	}
}

func (s *WalSuite) TestCommitWriteError() {
	for _, variant := range commitVariants {
		s.Run(variant.name, func() {
			expectedError := errors.New("test error")
			enc := variant.newEncoder(func() (*EncodedSegmentMock, error) {
				return &EncodedSegmentMock{
					SamplesFunc: func() uint32 { return 42 },
				}, nil
			})
			segmentWriter := &SegmentWriterMock[*EncodedSegmentMock]{
				WriteFunc: func(*EncodedSegmentMock) error { return expectedError },
			}
			maxSegmentSize := uint32(100)

			wl := wal.NewWal(enc, segmentWriter, s.locker, maxSegmentSize, 0, nil)

			s.Require().ErrorIs(variant.commit(wl), expectedError)
			s.Equal(1, variant.finalizeCalls(enc))
			s.Len(segmentWriter.WriteCalls(), 1)
		})
	}
}

func (s *WalSuite) TestCommitResetsLimitExhausted() {
	for _, variant := range commitVariants {
		s.Run(variant.name, func() {
			maxSegmentSize := uint32(100)
			enc := variant.newEncoder(func() (*EncodedSegmentMock, error) {
				return &EncodedSegmentMock{
					SamplesFunc: func() uint32 { return maxSegmentSize },
				}, nil
			})
			enc.EncodeFunc = func([]cppbridge.InnerSeries) (uint32, error) { return maxSegmentSize, nil }
			segmentWriter := &SegmentWriterMock[*EncodedSegmentMock]{
				WriteFunc: func(*EncodedSegmentMock) error { return nil },
			}

			wl := wal.NewWal(enc, segmentWriter, s.locker, maxSegmentSize, 0, nil)

			limitExhausted, err := wl.Write([]cppbridge.InnerSeries{})
			s.Require().NoError(err)
			s.True(limitExhausted)

			limitExhausted, err = wl.Write([]cppbridge.InnerSeries{})
			s.Require().NoError(err)
			s.False(limitExhausted, "limit exhaustion must be reported once per segment")

			s.Require().NoError(variant.commit(wl))

			limitExhausted, err = wl.Write([]cppbridge.InnerSeries{})
			s.Require().NoError(err)
			s.True(limitExhausted, "commit must reset limit exhaustion for the next segment")
		})
	}
}

func (s *WalSuite) TestFlush() {
	enc := &EncoderMock[*EncodedSegmentMock]{}
	segmentWriter := &SegmentWriterMock[*EncodedSegmentMock]{
		FlushFunc: func() error { return nil },
	}
	maxSegmentSize := uint32(100)

	wl := wal.NewWal(enc, segmentWriter, s.locker, maxSegmentSize, 0, nil)

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

	wl := wal.NewWal(enc, segmentWriter, s.locker, maxSegmentSize, 0, nil)

	s.Require().ErrorIs(wl.Flush(), expectedError)
	s.Len(segmentWriter.FlushCalls(), 1)
}

func (s *WalSuite) TestSync() {
	enc := &EncoderMock[*EncodedSegmentMock]{}
	segmentWriter := &SegmentWriterMock[*EncodedSegmentMock]{
		SyncFunc: func() error { return nil },
	}
	maxSegmentSize := uint32(100)

	wl := wal.NewWal(enc, segmentWriter, s.locker, maxSegmentSize, 0, nil)

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

	wl := wal.NewWal(enc, segmentWriter, s.locker, maxSegmentSize, 0, nil)

	s.Require().ErrorIs(wl.Sync(), expectedError)
	s.Len(segmentWriter.SyncCalls(), 1)
}

func (s *WalSuite) TestWrite() {
	enc := &EncoderMock[*EncodedSegmentMock]{
		EncodeFunc: func([]cppbridge.InnerSeries) (uint32, error) { return 100, nil },
	}
	segmentWriter := &SegmentWriterMock[*EncodedSegmentMock]{
		CloseFunc: func() error { return nil },
	}

	maxSegmentSize := uint32(0)
	wl := wal.NewWal(enc, segmentWriter, s.locker, maxSegmentSize, 0, nil)

	limitExhausted, err := wl.Write([]cppbridge.InnerSeries{})
	s.Require().NoError(err)
	s.Len(enc.EncodeCalls(), 1)
	s.False(limitExhausted)

	s.Require().NoError(wl.Close())
	s.Len(segmentWriter.CloseCalls(), 1)
}

func (s *WalSuite) TestWriteLimitExhausted() {
	maxSegmentSize := uint32(100)
	enc := &EncoderMock[*EncodedSegmentMock]{
		EncodeFunc: func([]cppbridge.InnerSeries) (uint32, error) { return maxSegmentSize, nil },
	}
	segmentWriter := &SegmentWriterMock[*EncodedSegmentMock]{
		CloseFunc: func() error { return nil },
	}

	wl := wal.NewWal(enc, segmentWriter, s.locker, maxSegmentSize, 0, nil)

	limitExhausted, err := wl.Write([]cppbridge.InnerSeries{})
	s.Require().NoError(err)
	s.Len(enc.EncodeCalls(), 1)
	s.True(limitExhausted)

	s.Require().NoError(wl.Close())
	s.Len(segmentWriter.CloseCalls(), 1)
}

func (s *WalSuite) TestWriteLimitNotExhausted() {
	maxSegmentSize := uint32(100)
	enc := &EncoderMock[*EncodedSegmentMock]{
		EncodeFunc: func([]cppbridge.InnerSeries) (uint32, error) { return maxSegmentSize / 2, nil },
	}
	segmentWriter := &SegmentWriterMock[*EncodedSegmentMock]{
		CloseFunc: func() error { return nil },
	}

	wl := wal.NewWal(enc, segmentWriter, s.locker, maxSegmentSize, 0, nil)

	limitExhausted, err := wl.Write([]cppbridge.InnerSeries{})
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
		EncodeFunc: func([]cppbridge.InnerSeries) (uint32, error) { return maxSegmentSize / 2, expectedError },
	}
	segmentWriter := &SegmentWriterMock[*EncodedSegmentMock]{
		CloseFunc: func() error { return nil },
	}

	wl := wal.NewWal(enc, segmentWriter, s.locker, maxSegmentSize, 0, nil)

	limitExhausted, err := wl.Write([]cppbridge.InnerSeries{})
	s.Require().ErrorIs(err, expectedError)
	s.Len(enc.EncodeCalls(), 1)
	s.False(limitExhausted)

	s.Require().NoError(wl.Close())
	s.Len(segmentWriter.CloseCalls(), 1)
}

func (s *WalSuite) TestCorrupted() {
	wl := wal.NewCorruptedWal[*EncodedSegmentMock, *SegmentWriterMock[*EncodedSegmentMock]]()
	s.Equal(int64(0), wl.CurrentSize())

	limitExhausted, err := wl.Write([]cppbridge.InnerSeries{})
	s.Require().ErrorIs(err, wal.ErrWalIsCorrupted)
	s.False(limitExhausted)

	err = wl.Commit()
	s.Require().ErrorIs(err, wal.ErrWalIsCorrupted)

	err = wl.LongCommit()
	s.Require().ErrorIs(err, wal.ErrWalIsCorrupted)

	err = wl.Flush()
	s.Require().NoError(err)

	err = wl.Sync()
	s.Require().ErrorIs(err, wal.ErrWalIsCorrupted)

	s.Require().NoError(wl.Close())
}
