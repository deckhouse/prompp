package catalog_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/storage/catalog"
)

type RecordSuite struct {
	suite.Suite
}

func TestRecordSuite(t *testing.T) {
	suite.Run(t, new(RecordSuite))
}

func (s *RecordSuite) TestReferenceCounterIncDecValue() {
	r := catalog.NewEmptyRecord()
	s.Require().Equal(int64(0), r.ReferenceCount())
	release := r.Acquire()
	s.Require().Equal(int64(1), r.ReferenceCount())
	release()
	s.Require().Equal(int64(0), r.ReferenceCount())
	release()
	s.Require().Equal(int64(0), r.ReferenceCount())
}

func (s *RecordSuite) TestGetShardBySegmentIDEmpty() {
	r := catalog.NewEmptyRecord()

	s.Equal(uint16(math.MaxUint16), r.GetShardBySegmentID(0))
	s.Equal(uint16(math.MaxUint16), r.GetShardBySegmentID(3600))
}

func (s *RecordSuite) TestSetSegmentIDByShard() {
	r := catalog.NewEmptyRecord()

	expectedShardID := uint16(2)
	r.SetSegmentIDByShard(0, expectedShardID)
	s.Equal(expectedShardID, r.GetShardBySegmentID(0))
}

func (s *RecordSuite) TestSetSegmentIDByShardResize() {
	r := catalog.NewEmptyRecord()

	expectedShardID := uint16(2)
	r.SetSegmentIDByShard(1440, expectedShardID)
	s.Equal(expectedShardID, r.GetShardBySegmentID(1440))

	r.SetSegmentIDByShard(2047, 2)
	s.Equal(expectedShardID, r.GetShardBySegmentID(2047))
}

func (s *RecordSuite) TestClearSegmentsByShard() {
	r := catalog.NewEmptyRecord()

	expectedShardID := uint16(2)
	r.SetSegmentIDByShard(1440, expectedShardID)
	s.Equal(expectedShardID, r.GetShardBySegmentID(1440))

	r.ClearSegmentsByShard()
	s.Equal(uint16(math.MaxUint16), r.GetShardBySegmentID(1440))
}

func (s *RecordSuite) TestIsMissingSegmentsByShardEmpty() {
	r := catalog.NewEmptyRecord()

	s.False(r.IsMissingSegmentsByShard())
}

func (s *RecordSuite) TestIsMissingSegmentsByShardFalse() {
	r := catalog.NewEmptyRecord()

	r.SetSegmentIDByShard(0, 2)
	r.SetSegmentIDByShard(1, 2)

	s.False(r.IsMissingSegmentsByShard())
}

func (s *RecordSuite) TestIsMissingSegmentsByShardTrue() {
	r := catalog.NewEmptyRecord()

	r.SetSegmentIDByShard(0, 2)
	r.SetSegmentIDByShard(2, 2)

	s.True(r.IsMissingSegmentsByShard())
}

func (s *RecordSuite) TestIsMissingSegmentsByShardTrue_2() {
	r := catalog.NewEmptyRecord()

	r.SetSegmentIDByShard(0, 2)
	r.SetSegmentIDByShard(42, 2)

	s.True(r.IsMissingSegmentsByShard())
}

func (s *RecordSuite) TestNextSegmentID() {
	r := catalog.NewEmptyRecord()

	for i := range uint32(1000) {
		s.Require().Equal(i, r.NextSegmentID())
	}
}

func (s *RecordSuite) TestSetLastSegmentID() {
	r := catalog.NewEmptyRecord()
	sid := uint32(42)

	r.SetLastSegmentID(sid)

	s.Require().Equal(sid+1, r.NextSegmentID())
}

func (s *RecordSuite) TestSetLastSegmentIDLess() {
	r := catalog.NewEmptyRecord()
	sid := uint32(42)

	r.SetLastSegmentID(sid)
	r.SetLastSegmentID(sid - 1)

	s.Require().Equal(sid+1, r.NextSegmentID())
}
