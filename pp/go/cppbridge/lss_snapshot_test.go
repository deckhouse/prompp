package cppbridge_test

import (
	"testing"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/stretchr/testify/suite"
)

type LabelSetSnapshotSuite struct {
	suite.Suite
}

func TestLabelSetSnapshotSuite(t *testing.T) {
	suite.Run(t, new(LabelSetSnapshotSuite))
}

func (s *LabelSetSnapshotSuite) TestLabels() {
	lsMap := map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	}

	lsIn := model.LabelSetFromMap(lsMap)

	lss := cppbridge.NewQueryableLssStorage()
	lsID := lss.FindOrEmplace(lsIn).LabelSetID
	snapshot := lss.CreateLabelSetSnapshot()

	lsLength := 0
	_ = snapshot.RangeLabelSet(lsID, func(l cppbridge.Label) error {
		lv, ok := lsMap[l.Name]
		s.Require().True(ok)
		s.Require().Equal(lv, l.Value)
		lsLength++

		return nil
	})

	s.Equal(lsIn.Len(), lsLength)
}
