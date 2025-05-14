package cppbridge_test

import (
	"testing"

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

// func (s *LabelsCppSuite) TestLabels() {
// 	lsMap := map[string]string{
// 		"__name__": "ubername",
// 		"lol":      "kek",
// 		"che":      "bureck",
// 	}

// 	lsIn := model.LabelSetFromMap(lsMap)

// 	lss := cppbridge.NewQueryableLssStorage()
// 	ls := cppbridge.NewLabelsCpp(lss, lss.FindOrEmplace(lsIn), uint16(lsIn.Len()))
// 	lsOut := ls.Labels()

// 	s.Equal(lsIn.Len(), lsOut.Len())

// 	lsOut.Range(func(l labels.Label) {
// 		lv, ok := lsMap[l.Name]
// 		s.Require().True(ok)
// 		s.Require().Equal(lv, l.Value)
// 	})
// }

//
// help func
//

type HelpFuncSuite struct {
	suite.Suite
}

func TestHelpFuncSuite(t *testing.T) {
	suite.Run(t, new(HelpFuncSuite))
}

func (s *HelpFuncSuite) TestEqualLabelSetsOneLSS() {
	lssA := cppbridge.NewQueryableLssStorage()
	mls := model.NewLabelSetBuilder().Set(
		"__name__", "ubername",
	).Set(
		"lol", "kek",
	).Set(
		"che", "bureck",
	).Build()

	lsIDA := lssA.FindOrEmplace(mls)

	s.True(cppbridge.EqualLabelSets(lssA, lssA, lsIDA, lsIDA))
}

func (s *HelpFuncSuite) TestEqualLabelSetsDiffLSS() {
	mls := model.NewLabelSetBuilder().Set(
		"__name__", "ubername",
	).Set(
		"lol", "kek",
	).Set(
		"che", "bureck",
	).Build()

	lssA := cppbridge.NewQueryableLssStorage()
	lsIDA := lssA.FindOrEmplace(mls)

	lssB := cppbridge.NewQueryableLssStorage()
	lsIDB := lssB.FindOrEmplace(mls)

	s.True(cppbridge.EqualLabelSets(lssA, lssB, lsIDA, lsIDB))
}
