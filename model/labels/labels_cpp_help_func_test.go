//go:build cpplabels

package labels_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
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

func (s *HelpFuncSuite) TestEqual() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lsB := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	s.True(labels.Equal(lsA, lsB))
}

func (s *HelpFuncSuite) TestEqualDropMetricName() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lsB := labels.FromMap(map[string]string{
		"lol": "kek",
		"che": "bureck",
	})

	s.True(labels.Equal(lsA.DropMetricName(), lsB))
}

func (s *HelpFuncSuite) TestEqualDropMetricName_2() {
	lsA := labels.FromMap(map[string]string{
		"lol": "kek",
		"che": "bureck",
	})

	lsB := labels.FromMap(map[string]string{
		"lol": "kek",
		"che": "bureck",
	})

	s.True(labels.Equal(lsA.DropMetricName(), lsB))
}

func (s *HelpFuncSuite) TestEqualEmpty() {
	lsA := labels.EmptyLabels()

	lsB := labels.EmptyLabels()

	s.True(labels.Equal(lsA, lsB))
}

func (s *HelpFuncSuite) TestEqualEmptyDropMetricName() {
	lsA := labels.FromStrings("__name__", "ubername")

	lsB := labels.EmptyLabels()

	s.True(labels.Equal(lsA.DropMetricName(), lsB))
}

func (s *HelpFuncSuite) TestEqualOneLSS() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lss := cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage())
	lsid := lss.FindOrEmplace(model.LabelSetFromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	}))
	lsB := labels.NewLabelsWithLSS(lss.Snapshot(), nil, lsid, 0)

	s.True(labels.Equal(lsA, lsB))
	s.True(labels.Equal(lsB, lsA))
}

func (s *HelpFuncSuite) TestEqualOneLSSDropMetricName() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lss := cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage())
	lsid := lss.FindOrEmplace(model.LabelSetFromMap(map[string]string{
		"lol": "kek",
		"che": "bureck",
	}))
	lsB := labels.NewLabelsWithLSS(lss.Snapshot(), nil, lsid, 0)

	s.True(labels.Equal(lsA.DropMetricName(), lsB))
	s.True(labels.Equal(lsB, lsA.DropMetricName()))
}

func (s *HelpFuncSuite) TestEqualTwoLSS() {
	lssA := cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage())
	lsInA := model.LabelSetFromMap(map[string]string{
		"__aaa__":  "11111",
		"__name__": "ubername",
		"che":      "bureck",
		"zimya":    "reck",
	})
	lsidA := lssA.FindOrEmplace(lsInA)
	lsA := labels.NewLabelsWithLSS(lssA.Snapshot(), nil, lsidA, uint16(lsInA.Len()))

	lssB := cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage())
	lsInB := model.LabelSetFromMap(map[string]string{
		"__aaa__":  "11111",
		"__name__": "ubername",
		"che":      "bureck",
		"zimya":    "reck",
	})
	lsidB := lssB.FindOrEmplace(lsInB)
	lsB := labels.NewLabelsWithLSS(lssB.Snapshot(), nil, lsidB, uint16(lsInB.Len()))

	s.True(labels.Equal(lsA, lsB))
	s.True(labels.Equal(lsB, lsA))
}

func (s *HelpFuncSuite) TestNotEqualOnLen() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lsB := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"imya":     "reck",
	})

	s.False(labels.Equal(lsA, lsB))
}

func (s *HelpFuncSuite) TestNotEqualOnLenDropMetricName() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lsB := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"imya":     "reck",
	})

	s.False(labels.Equal(lsA.DropMetricName(), lsB))
}

func (s *HelpFuncSuite) TestNotEqualOnLenAnyLSS() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lss := cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage())
	lsidB := lss.FindOrEmplace(model.LabelSetFromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"imya":     "reck",
	}))
	lsB := labels.NewLabelsWithLSS(lss.Snapshot(), nil, lsidB, 0)

	s.False(labels.Equal(lsA, lsB))
}

func (s *HelpFuncSuite) TestNotEqualOnLenAnyLSSDropMetricName() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lss := cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage())
	lsidB := lss.FindOrEmplace(model.LabelSetFromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"imya":     "reck",
	}))
	lsB := labels.NewLabelsWithLSS(lss.Snapshot(), nil, lsidB, 0)

	s.False(labels.Equal(lsA.DropMetricName(), lsB))
}

func (s *HelpFuncSuite) TestNotEqualOnLabel() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lsB := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"imya":     "reck",
	})

	s.False(labels.Equal(lsA, lsB))
}

func (s *HelpFuncSuite) TestNotEqualOnLabelDropMetricName() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lsB := labels.FromMap(map[string]string{
		"lol":  "kek",
		"imya": "reck",
	})

	s.False(labels.Equal(lsA.DropMetricName(), lsB))
}

func (s *HelpFuncSuite) TestNotEqualOnLabelAnyLSS() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lss := cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage())
	lsidB := lss.FindOrEmplace(model.LabelSetFromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"imya":     "reck",
	}))
	lsB := labels.NewLabelsWithLSS(lss.Snapshot(), nil, lsidB, 0)

	s.False(labels.Equal(lsA, lsB))
}

func (s *HelpFuncSuite) TestNotEqualOnLabelAnyLSSDropMetricName() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lss := cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage())
	lsidB := lss.FindOrEmplace(model.LabelSetFromMap(map[string]string{
		"lol":  "kek",
		"imya": "reck",
	}))
	lsB := labels.NewLabelsWithLSS(lss.Snapshot(), nil, lsidB, 0)

	s.False(labels.Equal(lsA.DropMetricName(), lsB))
}

func (s *HelpFuncSuite) TestNotEqualOnEmpty() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lsB := labels.EmptyLabels()

	s.False(labels.Equal(lsA, lsB))
}

func (s *HelpFuncSuite) TestCompare() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lsB := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	s.Equal(0, labels.Compare(lsA, lsB))
}

func (s *HelpFuncSuite) TestCompareDropMetricName() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lsB := labels.FromMap(map[string]string{
		"lol": "kek",
		"che": "bureck",
	})

	s.Equal(0, labels.Compare(lsA.DropMetricName(), lsB))
}

func (s *HelpFuncSuite) TestCompareDropMetricName_2() {
	lsA := labels.FromMap(map[string]string{
		"lol": "kek",
		"che": "bureck",
	})

	lsB := labels.FromMap(map[string]string{
		"lol": "kek",
		"che": "bureck",
	})

	s.Equal(0, labels.Compare(lsA.DropMetricName(), lsB))
}

func (s *HelpFuncSuite) TestCompareEmpty() {
	lsA := labels.EmptyLabels()

	lsB := labels.EmptyLabels()

	s.Equal(0, labels.Compare(lsA, lsB))
}

func (s *HelpFuncSuite) TestCompareEmptyDropMetricName() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
	})

	lsB := labels.EmptyLabels()

	s.Equal(0, labels.Compare(lsA.DropMetricName(), lsB))
}

func (s *HelpFuncSuite) TestCompareAnyLSS() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lss := cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage())
	lsidB := lss.FindOrEmplace(model.LabelSetFromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	}))
	lsB := labels.NewLabelsWithLSS(lss.Snapshot(), nil, lsidB, 0)

	s.Equal(0, labels.Compare(lsA, lsB))
}

func (s *HelpFuncSuite) TestCompareAnyLSSDropMetricName() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lss := cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage())
	lsidB := lss.FindOrEmplace(model.LabelSetFromMap(map[string]string{
		"lol": "kek",
		"che": "bureck",
	}))
	lsB := labels.NewLabelsWithLSS(lss.Snapshot(), nil, lsidB, 0)

	s.Equal(0, labels.Compare(lsA.DropMetricName(), lsB))
}

func (s *HelpFuncSuite) TestCompareLength() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"ximya":    "reck",
		"zimya":    "reck",
	})

	lsB := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	s.Equal(1, labels.Compare(lsA, lsB))
	s.Equal(-1, labels.Compare(lsB, lsA))
}

func (s *HelpFuncSuite) TestCompareLengthDropMetricName() {
	lsA := labels.FromMap(map[string]string{
		"lol":   "kek",
		"che":   "bureck",
		"ximya": "reck",
		"zimya": "reck",
	})

	lsB := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	s.Equal(1, labels.Compare(lsA, lsB.DropMetricName()))
	s.Equal(-1, labels.Compare(lsB.DropMetricName(), lsA))
}

func (s *HelpFuncSuite) TestCompareNameLength() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lolk":     "kek",
		"che":      "bureck",
	})

	lsB := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	s.Equal(1, labels.Compare(lsA, lsB))
	s.Equal(-1, labels.Compare(lsB, lsA))
}

func (s *HelpFuncSuite) TestCompareNameLengthDropMetricName() {
	lsA := labels.FromMap(map[string]string{
		"lolk": "kek",
		"che":  "bureck",
	})

	lsB := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	s.Equal(1, labels.Compare(lsA, lsB.DropMetricName()))
	s.Equal(-1, labels.Compare(lsB.DropMetricName(), lsA))
}

func (s *HelpFuncSuite) TestCompareAnyLSSNameLength() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lolk":     "kek",
		"che":      "bureck",
	})

	lss := cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage())
	lsidB := lss.FindOrEmplace(model.LabelSetFromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	}))
	lsB := labels.NewLabelsWithLSS(lss.Snapshot(), nil, lsidB, 0)

	s.Equal(1, labels.Compare(lsA, lsB))
	s.Equal(-1, labels.Compare(lsB, lsA))
}

func (s *HelpFuncSuite) TestCompareAnyLSSNameLengthDropMetricName() {
	lsA := labels.FromMap(map[string]string{
		"lolk": "kek",
		"che":  "bureck",
	})

	lss := cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage())
	lsidB := lss.FindOrEmplace(model.LabelSetFromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	}))
	lsB := labels.NewLabelsWithLSS(lss.Snapshot(), nil, lsidB, 0)

	s.Equal(1, labels.Compare(lsA, lsB.DropMetricName()))
	s.Equal(-1, labels.Compare(lsB.DropMetricName(), lsA))
}

func (s *HelpFuncSuite) TestCompareNameString() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lsB := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lok":      "kek",
		"che":      "bureck",
	})

	s.Equal(1, labels.Compare(lsA, lsB))
	s.Equal(-1, labels.Compare(lsB, lsA))
}

func (s *HelpFuncSuite) TestCompareNameStringDropMetricName() {
	lsA := labels.FromMap(map[string]string{
		"lol": "kek",
		"che": "bureck",
	})

	lsB := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lok":      "kek",
		"che":      "bureck",
	})

	s.Equal(1, labels.Compare(lsA, lsB.DropMetricName()))
	s.Equal(-1, labels.Compare(lsB.DropMetricName(), lsA))
}

func (s *HelpFuncSuite) TestCompareALSSNameString() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lss := cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage())
	lsidB := lss.FindOrEmplace(model.LabelSetFromMap(map[string]string{
		"__name__": "ubername",
		"lok":      "kek",
		"che":      "bureck",
	}))
	lsB := labels.NewLabelsWithLSS(lss.Snapshot(), nil, lsidB, 3)

	s.Equal(1, labels.Compare(lsA, lsB))
	s.Equal(-1, labels.Compare(lsB, lsA))
}

func (s *HelpFuncSuite) TestCompareALSSNameStringDropMetricName() {
	lsA := labels.FromMap(map[string]string{
		"lol": "kek",
		"che": "bureck",
	})

	lss := cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage())
	lsidB := lss.FindOrEmplace(model.LabelSetFromMap(map[string]string{
		"__name__": "ubername",
		"lok":      "kek",
		"che":      "bureck",
	}))
	lsB := labels.NewLabelsWithLSS(lss.Snapshot(), nil, lsidB, 3)

	s.Equal(1, labels.Compare(lsA, lsB.DropMetricName()))
	s.Equal(-1, labels.Compare(lsB.DropMetricName(), lsA))
}

func (s *HelpFuncSuite) TestCompareValueLength() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kkek",
		"che":      "bureck",
	})

	lsB := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	s.Equal(1, labels.Compare(lsA, lsB))
	s.Equal(-1, labels.Compare(lsB, lsA))
}

func (s *HelpFuncSuite) TestCompareValueLengthDropMetricName() {
	lsA := labels.FromMap(map[string]string{
		"lol": "kkek",
		"che": "bureck",
	})

	lsB := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	s.Equal(1, labels.Compare(lsA, lsB.DropMetricName()))
	s.Equal(-1, labels.Compare(lsB.DropMetricName(), lsA))
}

func (s *HelpFuncSuite) TestCompareLSSValueLength() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kkek",
		"che":      "bureck",
	})

	lss := cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage())
	lsidB := lss.FindOrEmplace(model.LabelSetFromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	}))
	lsB := labels.NewLabelsWithLSS(lss.Snapshot(), nil, lsidB, 0)

	s.Equal(1, labels.Compare(lsA, lsB))
	s.Equal(-1, labels.Compare(lsB, lsA))
}

func (s *HelpFuncSuite) TestCompareLSSValueLengthDropMetricName() {
	lsA := labels.FromMap(map[string]string{
		"lol": "kkek",
		"che": "bureck",
	})

	lss := cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage())
	lsidB := lss.FindOrEmplace(model.LabelSetFromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	}))
	lsB := labels.NewLabelsWithLSS(lss.Snapshot(), nil, lsidB, 0)

	s.Equal(1, labels.Compare(lsA, lsB.DropMetricName()))
	s.Equal(-1, labels.Compare(lsB.DropMetricName(), lsA))
}

func (s *HelpFuncSuite) TestCompareValueString() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lsB := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kak",
		"che":      "bureck",
	})

	s.Equal(1, labels.Compare(lsA, lsB))
	s.Equal(-1, labels.Compare(lsB, lsA))
}

func (s *HelpFuncSuite) TestCompareValueStringDropMetricName() {
	lsA := labels.FromMap(map[string]string{
		"lol": "kek",
		"che": "bureck",
	})

	lsB := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kak",
		"che":      "bureck",
	})

	s.Equal(1, labels.Compare(lsA, lsB.DropMetricName()))
	s.Equal(-1, labels.Compare(lsB.DropMetricName(), lsA))
}

func (s *HelpFuncSuite) TestCompareLSSValueString() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lss := cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage())
	lsidB := lss.FindOrEmplace(model.LabelSetFromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kak",
		"che":      "bureck",
	}))
	lsB := labels.NewLabelsWithLSS(lss.Snapshot(), nil, lsidB, 0)

	s.Equal(1, labels.Compare(lsA, lsB))
	s.Equal(-1, labels.Compare(lsB, lsA))
}

func (s *HelpFuncSuite) TestCompareLSSValueStringDropMetricName() {
	lsA := labels.FromMap(map[string]string{
		"lol": "kek",
		"che": "bureck",
	})

	lss := cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage())
	lsidB := lss.FindOrEmplace(model.LabelSetFromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kak",
		"che":      "bureck",
	}))
	lsB := labels.NewLabelsWithLSS(lss.Snapshot(), nil, lsidB, 0)

	s.Equal(1, labels.Compare(lsA, lsB.DropMetricName()))
	s.Equal(-1, labels.Compare(lsB.DropMetricName(), lsA))
}

func (s *HelpFuncSuite) TestCompareOnLabelAnyLSS() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"imya":     "reck",
	})

	lss := cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage())
	lsidB := lss.FindOrEmplace(model.LabelSetFromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	}))
	lsB := labels.NewLabelsWithLSS(lss.Snapshot(), nil, lsidB, 0)

	s.Equal(1, labels.Compare(lsA, lsB))
	s.Equal(-1, labels.Compare(lsB, lsA))
}

func (s *HelpFuncSuite) TestCompareOnLabelAnyLSSDropMetricName() {
	lsA := labels.FromMap(map[string]string{
		"lol":  "kek",
		"imya": "reck",
	})

	lss := cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage())
	lsidB := lss.FindOrEmplace(model.LabelSetFromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	}))
	lsB := labels.NewLabelsWithLSS(lss.Snapshot(), nil, lsidB, 0)

	s.Equal(1, labels.Compare(lsA, lsB.DropMetricName()))
	s.Equal(-1, labels.Compare(lsB.DropMetricName(), lsA))
}

func (s *HelpFuncSuite) TestCompareOnEmpty() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lsB := labels.EmptyLabels()

	s.Equal(1, labels.Compare(lsA, lsB))
	s.Equal(-1, labels.Compare(lsB, lsA))
}

func (s *HelpFuncSuite) TestCompareOnEmptyDropMetricName() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lsB := labels.EmptyLabels()

	s.Equal(1, labels.Compare(lsA.DropMetricName(), lsB))
	s.Equal(-1, labels.Compare(lsB, lsA.DropMetricName()))
}

func (s *HelpFuncSuite) TestCompareOnEmptyDropMetricName_2() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lsB := labels.FromMap(map[string]string{
		"__name__": "ubername",
	})

	s.Equal(1, labels.Compare(lsA.DropMetricName(), lsB.DropMetricName()))
	s.Equal(-1, labels.Compare(lsB.DropMetricName(), lsA.DropMetricName()))
}

func BenchmarkLabels_Equal(b *testing.B) {
	lsA := labels.FromMap(map[string]string{
		"lol": "kek",
		"che": "bureck",
	})

	lsB := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	for i := 0; i < b.N; i++ {
		_ = labels.Equal(lsA, lsB.DropMetricName())
	}
}
