package catalog_test

import (
	"testing"

	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/stretchr/testify/suite"
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
