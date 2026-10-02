package wal_test

import (
	"errors"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
)

func (s *WalSuite) TestStatsEachOperationAccounted() {
	// Arrange
	registry := prometheus.NewRegistry()
	wl := wal.NewWal(s.newStatsEncoder(nil), s.newStatsSegmentWriter(nil), s.locker, 100, 0, registry)

	// Act
	_, writeErr := wl.Write(nil)
	commitErr := wl.Commit()
	longCommitErr := wl.LongCommit()
	flushErr := wl.Flush()
	syncErr := wl.Sync()

	// Assert
	s.Require().NoError(errors.Join(writeErr, commitErr, longCommitErr, flushErr, syncErr))
	s.Equal(map[string]float64{
		"write":       1,
		"commit":      1,
		"long_commit": 1,
		"flush":       1,
		"sync":        1,
	}, s.stageExecutions(registry))
}

func (s *WalSuite) TestStatsFailedOperationsNotAccounted() {
	// Arrange
	registry := prometheus.NewRegistry()
	expectedError := errors.New("test error")
	wl := wal.NewWal(
		s.newStatsEncoder(expectedError),
		s.newStatsSegmentWriter(expectedError),
		s.locker,
		100,
		0,
		registry,
	)

	// Act
	_, writeErr := wl.Write(nil)
	commitErr := wl.Commit()
	longCommitErr := wl.LongCommit()
	flushErr := wl.Flush()
	syncErr := wl.Sync()

	// Assert
	s.Require().ErrorIs(writeErr, expectedError)
	s.Require().ErrorIs(commitErr, expectedError)
	s.Require().ErrorIs(longCommitErr, expectedError)
	s.Require().ErrorIs(flushErr, expectedError)
	s.Require().ErrorIs(syncErr, expectedError)
	s.Equal(map[string]float64{
		"write":       0,
		"commit":      0,
		"long_commit": 0,
		"flush":       0,
		"sync":        0,
	}, s.stageExecutions(registry))
}

func (s *WalSuite) TestStatsSharedByRegisterer() {
	// Arrange
	registry := prometheus.NewRegistry()
	first := wal.NewWal(s.newStatsEncoder(nil), s.newStatsSegmentWriter(nil), s.locker, 100, 0, registry)
	second := wal.NewWal(s.newStatsEncoder(nil), s.newStatsSegmentWriter(nil), s.locker, 100, 1, registry)

	// Act
	_, firstErr := first.Write(nil)
	_, secondErr := second.Write(nil)

	// Assert
	s.Require().NoError(errors.Join(firstErr, secondErr))
	s.InDelta(2, s.stageExecutions(registry)["write"], 0)
}

// newStatsEncoder returns an encoder whose methods return err.
func (*WalSuite) newStatsEncoder(err error) *EncoderMock[*EncodedSegmentMock] {
	finalize := func() (*EncodedSegmentMock, error) {
		return &EncodedSegmentMock{SamplesFunc: func() uint32 { return 1 }}, err
	}

	return &EncoderMock[*EncodedSegmentMock]{
		EncodeFunc:       func([]cppbridge.InnerSeries) (uint32, error) { return 1, err },
		FinalizeFunc:     finalize,
		LongFinalizeFunc: finalize,
	}
}

// newStatsSegmentWriter returns a segment writer whose methods return err.
func (*WalSuite) newStatsSegmentWriter(err error) *SegmentWriterMock[*EncodedSegmentMock] {
	return &SegmentWriterMock[*EncodedSegmentMock]{
		WriteFunc: func(*EncodedSegmentMock) error { return err },
		FlushFunc: func() error { return err },
		SyncFunc:  func() error { return err },
	}
}

// stageExecutions returns the number of executions by stage of the wal operations in the registry.
func (s *WalSuite) stageExecutions(registry *prometheus.Registry) map[string]float64 {
	families, err := registry.Gather()
	s.Require().NoError(err)

	result := map[string]float64{}
	for _, family := range families {
		if family.GetName() != "prompp_shard_wal_stage_executions_total" {
			continue
		}

		for _, metric := range family.GetMetric() {
			result[metric.GetLabel()[0].GetValue()] = metric.GetCounter().GetValue()
		}
	}

	return result
}
