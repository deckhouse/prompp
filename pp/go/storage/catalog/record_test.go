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
