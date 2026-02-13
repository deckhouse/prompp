package catalog_test

import (
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

func TestXxx(t *testing.T) {
	r := catalog.NewEmptyRecord()

	t.Log(r.GetShardBySegmentID(0))
	t.Log(r.GetShardBySegmentID(3600))

	r.SetSegmentIDByShard(0, 2)
	t.Log(r.GetShardBySegmentID(0))

	r.SetSegmentIDByShard(1440, 2)
	t.Log(r.GetShardBySegmentID(1440))

	r.SetSegmentIDByShard(2047, 2)
	t.Log(r.GetShardBySegmentID(2047))
}
