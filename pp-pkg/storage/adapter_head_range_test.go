// Copyright 2026 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// This file pins down which heads a query touches: a head whose data cannot
// intersect the query range is not queried at all.
//
// The range check trusts the head time interval to be computed rather than assumed; that a non-empty
// storage with an unknown cached interval has its bounds computed on read is covered by
// cppbridge HeadSuite.TestTimeIntervalIsComputedWhenCacheIsUnknown.

package storage

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	pp_model "github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
)

// AdapterHeadRangeSuite reuses the Adapter harness of BatchStorageSuite to check
// the time interval of the active head against query ranges.
type AdapterHeadRangeSuite struct {
	BatchStorageSuite
}

func TestAdapterHeadRangeSuite(t *testing.T) {
	suite.Run(t, new(AdapterHeadRangeSuite))
}

// ingestAt appends a single sample of one series at tMs to the active head.
func (s *AdapterHeadRangeSuite) ingestAt(tMs int64) {
	batch := &testTimeSeriesBatch{
		timeSeries: []pp_model.TimeSeries{
			{
				LabelSet:  pp_model.NewLabelSetBuilder().Set("__name__", "metric").Set("job", "abc").Build(),
				Timestamp: uint64(tMs), // #nosec G115 // test data
				Value:     1,
			},
		},
	}

	_, err := s.adapter.AppendTimeSeries(s.ctx, batch, s.state, false)
	s.Require().NoError(err)
}

// TestEmptyHeadIsNotQueried covers a head whose bounds are known to be empty: the interval is
// computed from the data storage, not assumed, so an empty head is out of every query range.
// requireEveryHeadOutOfRange pins the premise of the zero-querier tests: no head of the adapter can
// hold data for the range, so Querier and ChunkQuerier build their merge over an empty list.
func (s *AdapterHeadRangeSuite) requireEveryHeadOutOfRange(mint, maxt int64) {
	proxy := s.manager.Proxy()
	s.Require().True(headIsOutOfQueryRange(proxy.Get(), mint, maxt), "the active head must be out of range")

	for _, head := range proxy.Heads() {
		s.Require().True(headIsOutOfQueryRange(head, mint, maxt), "head %s must be out of range", head.ID())
	}
}

func (s *AdapterHeadRangeSuite) TestEmptyHeadIsNotQueried() {
	// Arrange
	head := s.manager.Proxy().Get()

	// Act
	outOfRange := headIsOutOfQueryRange(head, 1_000, 2_000)

	// Assert
	s.True(outOfRange)
}

func (s *AdapterHeadRangeSuite) TestHeadIsQueriedForOverlappingRange() {
	// Arrange
	s.ingestAt(2_000)
	head := s.manager.Proxy().Get()

	// Act
	outOfRange := headIsOutOfQueryRange(head, 1_000, 3_000)

	// Assert
	s.False(outOfRange)
}

func (s *AdapterHeadRangeSuite) TestHeadIsNotQueriedForRangeAfterItsData() {
	// Arrange
	s.ingestAt(2_000)
	head := s.manager.Proxy().Get()

	// Act
	outOfRange := headIsOutOfQueryRange(head, 3_000, 4_000)

	// Assert
	s.True(outOfRange)
}

func (s *AdapterHeadRangeSuite) TestHeadIsNotQueriedForRangeBeforeItsData() {
	// Arrange
	s.ingestAt(2_000)
	head := s.manager.Proxy().Get()

	// Act
	outOfRange := headIsOutOfQueryRange(head, 500, 1_000)

	// Assert
	s.True(outOfRange)
}

// TestHeadIsQueriedForRangeOfLaterAppend guards the incremental maintenance of the head time
// interval: a stale interval would leave the later sample outside the bounds and skip the head
// that holds it.
func (s *AdapterHeadRangeSuite) TestHeadIsQueriedForRangeOfLaterAppend() {
	// Arrange
	s.ingestAt(2_000)
	head := s.manager.Proxy().Get()
	s.Require().False(headIsOutOfQueryRange(head, 1_000, 3_000))

	// Act
	s.ingestAt(10_000)

	// Assert
	s.False(headIsOutOfQueryRange(head, 9_000, 11_000))
}

func (s *AdapterHeadRangeSuite) TestQuerierUsesEachHeadsTimeBounds() {
	// Arrange
	s.ingestAt(2_000)
	proxy := s.manager.Proxy()
	oldHead := proxy.Get()
	newHead, err := s.manager.Builder().Build(oldHead.Generation()+1, 2)
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		s.NoError(newHead.Close())
		s.NoError(oldHead.Close())
	})
	s.Require().NoError(proxy.AddWithReplace(oldHead, 0))
	s.Require().NoError(proxy.Replace(s.ctx, newHead))
	oldHead.SetReadOnly()
	s.ingestAt(10_000)

	// Act
	q, err := s.adapter.Querier(1_000, 11_000)
	s.Require().NoError(err)

	// Assert
	// Inspect the stored bounds: the current optimization gate does not use them for range queries.
	queriers := reflect.ValueOf(q).Elem().FieldByName("queriers")
	s.Require().Equal(2, queriers.Len())
	s.Equal(int64(10_000), queriers.Index(0).Elem().Elem().FieldByName("headMinTSMS").Int())
	s.Equal(int64(2_000), queriers.Index(1).Elem().Elem().FieldByName("headMinTSMS").Int())

	seriesSet := q.Select(s.ctx, false, nil, labels.MustNewMatcher(labels.MatchEqual, "__name__", "metric"))
	actual := storagetest.TimeSeriesFromSeriesSet(seriesSet, true)
	s.Require().NoError(seriesSet.Err())
	s.Equal([]storagetest.TimeSeries{{
		Labels: labels.FromStrings("__name__", "metric", "job", "abc"),
		Samples: []cppbridge.Sample{
			{Timestamp: 2_000, Value: 1},
			{Timestamp: 10_000, Value: 1},
		},
	}}, actual)
	s.Require().NoError(q.Close())
}

// TestQuerierReadsActiveHeadForInstantAfterItsData mirrors the federation endpoint, which queries the
// zero-length range [now, now]: the last sample of the active head is always older than now, and the
// instant path must still return it.
func (s *AdapterHeadRangeSuite) TestQuerierReadsActiveHeadForInstantAfterItsData() {
	// Arrange
	s.ingestAt(2_000)

	// Act
	q, err := s.adapter.Querier(3_000, 3_000)
	s.Require().NoError(err)
	seriesSet := q.Select(s.ctx, false, nil, labels.MustNewMatcher(labels.MatchEqual, "__name__", "metric"))
	actual := storagetest.TimeSeriesFromSeriesSet(seriesSet, true)

	// Assert
	s.Require().NoError(seriesSet.Err())
	s.Equal([]storagetest.TimeSeries{{
		Labels:  labels.FromStrings("__name__", "metric", "job", "abc"),
		Samples: []cppbridge.Sample{{Timestamp: 2_000, Value: 1}},
	}}, actual)
	s.Require().NoError(q.Close())
}

func (s *AdapterHeadRangeSuite) TestQuerierSkipsActiveHeadForInstantBeforeItsData() {
	// Arrange
	s.ingestAt(2_000)

	// Act
	q, err := s.adapter.Querier(1_000, 1_000)
	s.Require().NoError(err)

	// Assert
	s.Zero(reflect.ValueOf(q).Elem().FieldByName("queriers").Len())
	s.Require().NoError(q.Close())
}

// TestQuerierWithoutAnyHeadInRange covers the empty querier list. When no head can hold data for the
// range, every head is skipped and the MultiQuerier is built over zero queriers; it must still answer
// every method with an empty result instead of failing.
func (s *AdapterHeadRangeSuite) TestQuerierWithoutAnyHeadInRange() {
	// Arrange
	s.ingestAt(2_000)
	s.requireEveryHeadOutOfRange(10_000, 20_000)

	// Act
	q, err := s.adapter.Querier(10_000, 20_000)
	s.Require().NoError(err)

	seriesSet := q.Select(s.ctx, false, nil, labels.MustNewMatcher(labels.MatchEqual, "__name__", "metric"))
	labelNames, _, labelNamesErr := q.LabelNames(s.ctx, nil)
	labelValues, _, labelValuesErr := q.LabelValues(s.ctx, "__name__", nil)
	closeErr := q.Close()

	// Assert
	s.False(seriesSet.Next())
	s.Require().NoError(seriesSet.Err())
	s.Require().NoError(labelNamesErr)
	s.Empty(labelNames)
	s.Require().NoError(labelValuesErr)
	s.Empty(labelValues)
	s.Require().NoError(closeErr)
}

// TestChunkQuerierWithoutAnyHeadInRange is TestQuerierWithoutAnyHeadInRange for the chunk querier,
// whose empty list goes to storage.NewMergeChunkQuerier instead.
func (s *AdapterHeadRangeSuite) TestChunkQuerierWithoutAnyHeadInRange() {
	// Arrange
	s.ingestAt(2_000)
	s.requireEveryHeadOutOfRange(10_000, 20_000)

	// Act
	q, err := s.adapter.ChunkQuerier(10_000, 20_000)
	s.Require().NoError(err)

	chunkSeriesSet := q.Select(s.ctx, false, nil, labels.MustNewMatcher(labels.MatchEqual, "__name__", "metric"))
	labelNames, _, labelNamesErr := q.LabelNames(s.ctx, nil)
	closeErr := q.Close()

	// Assert
	s.False(chunkSeriesSet.Next())
	s.Require().NoError(chunkSeriesSet.Err())
	s.Require().NoError(labelNamesErr)
	s.Empty(labelNames)
	s.Require().NoError(closeErr)
}
