package cppbridge_test

import (
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
)

type LabelsCppSuite struct {
	suite.Suite
}

func TestLabelsCppSuite(t *testing.T) {
	suite.Run(t, new(LabelsCppSuite))
}

func (s *LabelsCppSuite) TestLen() {
	lsIn := model.LabelSetFromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lss := cppbridge.NewQueryableLssStorage()
	ls := cppbridge.NewLabelsCpp(lss, lss.FindOrEmplace(lsIn), uint16(lsIn.Len()))

	s.Equal(lsIn.Len(), ls.Len())
}

func (s *LabelsCppSuite) TestLenNullIn() {
	lsIn := model.LabelSetFromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lss := cppbridge.NewQueryableLssStorage()
	ls := cppbridge.NewLabelsCpp(lss, lss.FindOrEmplace(lsIn), 0)

	s.Equal(lsIn.Len(), ls.Len())
}

func (s *LabelsCppSuite) TestIsZeroFalseLSS() {
	lss := cppbridge.NewQueryableLssStorage()
	ls := cppbridge.NewLabelsCpp(lss, lss.FindOrEmplace(model.LabelSetFromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})), 3)

	s.False(ls.IsZero())
}

func (s *LabelsCppSuite) TestIsZero() {
	ls := cppbridge.NewLabelsCpp(nil, 3, 0)

	s.True(ls.IsZero())
}

func (s *LabelsCppSuite) TestLabels() {
	lsMap := map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	}

	lsIn := model.LabelSetFromMap(lsMap)

	lss := cppbridge.NewQueryableLssStorage()
	ls := cppbridge.NewLabelsCpp(lss, lss.FindOrEmplace(lsIn), uint16(lsIn.Len()))
	lsOut := labels.NewLabelsWithLSS(ls.LSS(), ls.ID(), ls.Length())

	s.Equal(lsIn.Len(), lsOut.Len())

	lsOut.Range(func(l labels.Label) {
		lv, ok := lsMap[l.Name]
		s.Require().True(ok)
		s.Require().Equal(lv, l.Value)
	})
}
