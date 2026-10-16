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
// The range check trusts the head time interval to be computed rather than assumed; that a storage
// with an unknown cached interval has its bounds computed on read is covered by
// cppbridge HeadSuite.TestTimeInterval.

package storage

import (
	"testing"

	"github.com/stretchr/testify/suite"

	pp_model "github.com/prometheus/prometheus/pp/go/model"
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
