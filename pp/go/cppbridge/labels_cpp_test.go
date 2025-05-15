package cppbridge_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
)

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

	s.True(cppbridge.EqualLabelSets(lssA, lssA, lsIDA, lsIDA, false, false))
}

func (s *HelpFuncSuite) TestEqualLabelSetsOneLSSDrop() {
	lssA := cppbridge.NewQueryableLssStorage()
	mls := model.NewLabelSetBuilder().Set(
		"__name__", "ubername",
	).Set(
		"lol", "kek",
	).Set(
		"che", "bureck",
	).Build()

	lsIDA := lssA.FindOrEmplace(mls)

	s.False(cppbridge.EqualLabelSets(lssA, lssA, lsIDA, lsIDA, false, true))
}

func (s *HelpFuncSuite) TestEqualLabelSetsOneLSSDrop_2() {
	mlsA := model.NewLabelSetBuilder().Set(
		"__name__", "ubername",
	).Set(
		"lol", "kek",
	).Set(
		"che", "bureck",
	).Build()

	mlsB := model.NewLabelSetBuilder().Set(
		"lol", "kek",
	).Set(
		"che", "bureck",
	).Build()

	lssA := cppbridge.NewQueryableLssStorage()
	lsIDA := lssA.FindOrEmplace(mlsA)

	lsIDB := lssA.FindOrEmplace(mlsB)

	s.True(cppbridge.EqualLabelSets(lssA, lssA, lsIDA, lsIDB, true, false))
}

func (s *HelpFuncSuite) TestEqualLabelSetsOneLSSFalse() {
	mlsA := model.NewLabelSetBuilder().Set(
		"__name__", "ubername",
	).Set(
		"lol", "kek",
	).Set(
		"che", "bureck",
	).Build()

	mlsB := model.NewLabelSetBuilder().Set(
		"__name__", "ubername",
	).Set(
		"lol", "kep",
	).Set(
		"che", "bureck",
	).Build()

	lssA := cppbridge.NewQueryableLssStorage()
	lsIDA := lssA.FindOrEmplace(mlsA)

	lsIDB := lssA.FindOrEmplace(mlsB)

	s.False(cppbridge.EqualLabelSets(lssA, lssA, lsIDA, lsIDB, false, false))
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

	s.True(cppbridge.EqualLabelSets(lssA, lssB, lsIDA, lsIDB, false, false))
}

func (s *HelpFuncSuite) TestEqualLabelSetsDiffLSSFalse_2() {
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

	s.False(cppbridge.EqualLabelSets(lssA, lssB, lsIDA, lsIDB, true, false))
}

func (s *HelpFuncSuite) TestEqualLabelSetsDiffLSSFalse() {
	mlsA := model.NewLabelSetBuilder().Set(
		"__name__", "ubername",
	).Set(
		"lol", "kek",
	).Set(
		"che", "bureck",
	).Build()

	mlsB := model.NewLabelSetBuilder().Set(
		"__name__", "ubername",
	).Set(
		"lol", "kep",
	).Set(
		"che", "bureck",
	).Build()

	lssA := cppbridge.NewQueryableLssStorage()
	lsIDA := lssA.FindOrEmplace(mlsA)

	lssB := cppbridge.NewQueryableLssStorage()
	lsIDB := lssB.FindOrEmplace(mlsB)

	s.False(cppbridge.EqualLabelSets(lssA, lssB, lsIDA, lsIDB, false, false))
}

func (s *HelpFuncSuite) TestEqualLabelSetsDiffLSSDrop() {
	mlsA := model.NewLabelSetBuilder().Set(
		"__name__", "ubername",
	).Set(
		"lol", "kek",
	).Set(
		"che", "bureck",
	).Build()

	mlsB := model.NewLabelSetBuilder().Set(
		"__name__", "ubername",
	).Set(
		"lol", "kek",
	).Set(
		"che", "bureck",
	).Build()

	lssA := cppbridge.NewQueryableLssStorage()
	lsIDA := lssA.FindOrEmplace(mlsA)

	lssB := cppbridge.NewQueryableLssStorage()
	lsIDB := lssB.FindOrEmplace(mlsB)

	s.True(cppbridge.EqualLabelSets(lssA, lssB, lsIDA, lsIDB, true, true))
}
