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

	resA := lssA.FindOrEmplace(mls)
	snapshotA := lssA.CreateLabelSetSnapshot()

	s.True(cppbridge.EqualLabelSets(snapshotA, snapshotA, resA.LabelSetID, resA.LabelSetID, false, false))
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

	resA := lssA.FindOrEmplace(mls)
	snapshotA := lssA.CreateLabelSetSnapshot()

	s.False(cppbridge.EqualLabelSets(snapshotA, snapshotA, resA.LabelSetID, resA.LabelSetID, false, true))
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
	resA := lssA.FindOrEmplace(mlsA)
	resB := lssA.FindOrEmplace(mlsB)
	snapshotA := lssA.CreateLabelSetSnapshot()

	s.True(cppbridge.EqualLabelSets(snapshotA, snapshotA, resA.LabelSetID, resB.LabelSetID, true, false))
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
	resA := lssA.FindOrEmplace(mlsA)
	resB := lssA.FindOrEmplace(mlsB)
	snapshotA := lssA.CreateLabelSetSnapshot()

	s.False(cppbridge.EqualLabelSets(snapshotA, snapshotA, resA.LabelSetID, resB.LabelSetID, false, false))
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
	resA := lssA.FindOrEmplace(mls)
	snapshotA := lssA.CreateLabelSetSnapshot()

	lssB := cppbridge.NewQueryableLssStorage()
	resB := lssB.FindOrEmplace(mls)
	snapshotB := lssB.CreateLabelSetSnapshot()

	s.True(cppbridge.EqualLabelSets(snapshotA, snapshotB, resA.LabelSetID, resB.LabelSetID, false, false))
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
	resA := lssA.FindOrEmplace(mls)
	snapshotA := lssA.CreateLabelSetSnapshot()

	lssB := cppbridge.NewQueryableLssStorage()
	resB := lssB.FindOrEmplace(mls)
	snapshotB := lssB.CreateLabelSetSnapshot()

	s.False(cppbridge.EqualLabelSets(snapshotA, snapshotB, resA.LabelSetID, resB.LabelSetID, true, false))
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
	resA := lssA.FindOrEmplace(mlsA)
	snapshotA := lssA.CreateLabelSetSnapshot()

	lssB := cppbridge.NewQueryableLssStorage()
	resB := lssB.FindOrEmplace(mlsB)
	snapshotB := lssB.CreateLabelSetSnapshot()

	s.False(cppbridge.EqualLabelSets(snapshotA, snapshotB, resA.LabelSetID, resB.LabelSetID, false, false))
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
	resA := lssA.FindOrEmplace(mlsA)
	snapshotA := lssA.CreateLabelSetSnapshot()

	lssB := cppbridge.NewQueryableLssStorage()
	resB := lssB.FindOrEmplace(mlsB)
	snapshotB := lssB.CreateLabelSetSnapshot()

	s.True(cppbridge.EqualLabelSets(snapshotA, snapshotB, resA.LabelSetID, resB.LabelSetID, true, true))
}
