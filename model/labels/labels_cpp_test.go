//go:build cpplabels

package labels_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
)

type LabelsSuite struct {
	suite.Suite
}

func TestLabelsSuite(t *testing.T) {
	suite.Run(t, new(LabelsSuite))
}

func (s *LabelsSuite) TestBytes() {
	expected := []byte{
		254, 95, 95, 110, 97, 109, 101, 95, 95, 255, 117, 98,
		101, 114, 110, 97, 109, 101, 255, 99, 104, 101, 255, 98,
		117, 114, 101, 99, 107, 255, 108, 111, 108, 255, 107, 101,
		107, 255, 122, 105, 109, 121, 97, 255, 114, 101, 99, 107,
	}

	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})

	s.Equal(expected, lsA.Bytes(nil))
}

func (s *LabelsSuite) TestBytesDropMetricName() {
	expected := []byte{
		254, 99, 104, 101, 255, 98, 117, 114, 101, 99,
		107, 255, 108, 111, 108, 255, 107, 101, 107, 255,
		122, 105, 109, 121, 97, 255, 114, 101, 99, 107,
	}

	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})

	s.Equal(expected, lsA.DropMetricName().Bytes(nil))
}

func (s *LabelsSuite) TestBytesWithLabels() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
	})

	lsB := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})

	s.Equal(lsA.Bytes(nil), lsB.BytesWithLabels(nil, "__name__", "lol"))
}

func (s *LabelsSuite) TestBytesWithLabelsDropMetricName() {
	lsA := labels.FromMap(map[string]string{
		"lol": "kek",
	})

	lsB := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})

	s.Equal(lsA.Bytes(nil), lsB.DropMetricName().BytesWithLabels(nil, "__name__", "lol"))
}

func (s *LabelsSuite) TestBytesWithLabelsEmpty() {
	lsA := labels.FromStrings()

	lsB := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})

	s.Equal(lsA.Bytes(nil), lsB.BytesWithLabels(nil))
}

func (s *LabelsSuite) TestBytesWithLabelsEmptyDropMetricName() {
	lsA := labels.FromStrings()

	lsB := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})

	s.Equal(lsA.Bytes(nil), lsB.DropMetricName().BytesWithLabels(nil, "__name__"))
}

func (s *LabelsSuite) TestBytesWithoutLabels() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
	})

	lsB := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})

	s.Equal(lsA.Bytes(nil), lsB.BytesWithoutLabels(nil, "che", "zimya"))
}

func (s *LabelsSuite) TestBytesWithoutLabelsDropMetricName() {
	lsA := labels.FromMap(map[string]string{
		"lol": "kek",
	})

	lsB := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})

	s.Equal(lsA.Bytes(nil), lsB.DropMetricName().BytesWithoutLabels(nil, "che", "zimya"))
}

func (s *LabelsSuite) TestBytesWithoutLabels_2() {
	lsA := labels.FromMap(map[string]string{
		"lol": "kek",
	})

	lsB := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})

	s.Equal(lsA.Bytes(nil), lsB.BytesWithoutLabels(nil, "__name__", "che", "zimya"))
}

func (s *LabelsSuite) TestBytesWithoutLabelsEmpty() {
	lsA := labels.FromStrings()

	lsB := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})

	s.Equal(lsA.Bytes(nil), lsB.BytesWithoutLabels(nil, "__name__", "che", "lol", "zimya"))
}

func (s *LabelsSuite) TestBytesWithoutLabelsEmptyDropMetricName() {
	lsA := labels.FromStrings()

	lsB := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})

	s.Equal(lsA.Bytes(nil), lsB.DropMetricName().BytesWithoutLabels(nil, "che", "lol", "zimya"))
}

func (s *LabelsSuite) TestBytesWithoutLabelsEmptyNames() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})

	s.Equal(lsA.Bytes(nil), lsA.BytesWithoutLabels(nil))
}

func (s *LabelsSuite) TestBytesWithoutLabelsEmptyNamesDropMetricName() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})

	s.Equal(lsA.DropMetricName().Bytes(nil), lsA.DropMetricName().BytesWithoutLabels(nil))
}

func (s *LabelsSuite) TestCopy() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})

	lsB := lsA.Copy()

	s.Equal(lsA.Bytes(nil), lsB.Bytes(nil))
	s.True(labels.Equal(lsA, lsB))
}

func (s *LabelsSuite) TestCopyDropMetricName() {
	lsExpected := labels.FromMap(map[string]string{
		"lol":   "kek",
		"che":   "bureck",
		"zimya": "reck",
	})

	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})

	lsB := lsA.Copy()

	s.Equal(lsExpected.Bytes(nil), lsB.DropMetricName().Bytes(nil))
	s.NotEqual(lsA.Bytes(nil), lsB.DropMetricName().Bytes(nil))
	s.True(labels.Equal(lsExpected, lsB.DropMetricName()))
	s.False(labels.Equal(lsA, lsB.DropMetricName()))
}

func (s *LabelsSuite) TestCopyEmpty() {
	lsA := labels.FromStrings()

	lsB := lsA.Copy()

	s.Equal(lsA.Bytes(nil), lsB.Bytes(nil))
	s.True(labels.Equal(lsA, lsB))
}

func (s *LabelsSuite) TestCopyEmptyDropMetricName() {
	lsExpected := labels.FromStrings()

	lsA := labels.FromStrings("__name__", "ubername")

	lsB := lsA.Copy()

	s.Equal(lsExpected.Bytes(nil), lsB.DropMetricName().Bytes(nil))
	s.NotEqual(lsA.Bytes(nil), lsB.DropMetricName().Bytes(nil))
	s.True(labels.Equal(lsExpected, lsB.DropMetricName()))
	s.False(lsA.IsEmpty())
}

func (s *LabelsSuite) TestCopyFrom() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})

	lsB := labels.FromStrings()
	lsB.CopyFrom(lsA)

	s.Equal(lsA.Bytes(nil), lsB.Bytes(nil))
	s.True(labels.Equal(lsA, lsB))
}

func (s *LabelsSuite) TestCopyFromDropMetricName() {
	lsExpected := labels.FromMap(map[string]string{
		"lol":   "kek",
		"che":   "bureck",
		"zimya": "reck",
	})

	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})

	lsB := labels.FromStrings()
	lsB.CopyFrom(lsA.DropMetricName())

	s.Equal(lsExpected.Bytes(nil), lsB.Bytes(nil))
	s.NotEqual(lsA.Bytes(nil), lsB.Bytes(nil))
	s.True(labels.Equal(lsExpected, lsB))
	s.False(labels.Equal(lsA, lsB))
}

func (s *LabelsSuite) TestCopyFromEmpty() {
	lsA := labels.FromStrings()

	lsB := labels.FromStrings()
	lsB.CopyFrom(lsA)

	s.Equal(lsA.Bytes(nil), lsB.Bytes(nil))
	s.True(labels.Equal(lsA, lsB))
}

func (s *LabelsSuite) TestCopyFromEmptyDropMetricName() {
	lsExpected := labels.FromStrings()

	lsA := labels.FromStrings("__name__", "ubername")

	lsB := labels.FromStrings()
	lsB.CopyFrom(lsA.DropMetricName())

	s.Equal(lsExpected.Bytes(nil), lsB.Bytes(nil))
	s.NotEqual(lsA.Bytes(nil), lsB.Bytes(nil))
	s.True(labels.Equal(lsExpected, lsB))
	s.False(labels.Equal(lsA, lsB))
}

func (s *LabelsSuite) TestDropMetricName() {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})

	lsB := labels.FromMap(map[string]string{
		"lol":   "kek",
		"che":   "bureck",
		"zimya": "reck",
	})

	s.Equal(lsB.Bytes(nil), lsA.DropMetricName().Bytes(nil))
	s.True(labels.Equal(lsA.DropMetricName(), lsB))
}

func (s *LabelsSuite) TestDropMetricName_2() {
	lsA := labels.FromMap(map[string]string{
		"lol":   "kek",
		"che":   "bureck",
		"zimya": "reck",
	})

	lsB := labels.FromMap(map[string]string{
		"lol":   "kek",
		"che":   "bureck",
		"zimya": "reck",
	})

	s.Equal(lsB.Bytes(nil), lsA.DropMetricName().Bytes(nil))
	s.True(labels.Equal(lsA.DropMetricName(), lsB))
}

func (s *LabelsSuite) TestDropMetricName_3() {
	original := labels.FromMap(map[string]string{
		"__aaa__":  "11111",
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})

	lsB := labels.FromMap(map[string]string{
		"__aaa__":  "11111",
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})

	lsC := labels.FromMap(map[string]string{
		"__aaa__": "11111",
		"lol":     "kek",
		"che":     "bureck",
		"zimya":   "reck",
	})

	s.Equal(lsC.Bytes(nil), lsB.DropMetricName().Bytes(nil))
	s.True(labels.Equal(lsB.DropMetricName(), lsC))

	s.Equal(original.Bytes(nil), lsB.Bytes(nil))
	s.True(labels.Equal(original, lsB))
}

func (s *LabelsSuite) TestGetDropMetricName() {
	lsMap := map[string]string{
		"__aaa__":  "11111",
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	}

	lsA := labels.FromMap(lsMap)

	for ln, lv := range lsMap {
		s.Equal(lv, lsA.Get(ln))
	}

	s.Equal("", lsA.DropMetricName().Get("__name__"))
}

func (s *LabelsSuite) TestHasDropMetricName() {
	lsMap := map[string]string{
		"__aaa__":  "11111",
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	}

	lsA := labels.FromMap(lsMap)

	for ln := range lsMap {
		s.True(lsA.Has(ln))
	}

	s.False(lsA.DropMetricName().Has("__name__"))
}

func (s *LabelsSuite) TestHasDuplicateLabelNamesFalseDropMetricName() {
	lsA := labels.FromStrings(
		"__aaa__", "11111",
		"__name__", "ubername",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)
	_, has := lsA.DropMetricName().HasDuplicateLabelNames()

	s.False(has)
}

func (s *LabelsSuite) TestHasDuplicateLabelNamesFalseDropMetricName_2() {
	expected := `{__aaa__="11111", che="bureck", lol="kek", zimya="reck"}`
	lsA := labels.FromStrings(
		"__aaa__", "11111",
		"__name__", "ubername",
		"__name__", "ubername2",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)
	name, has := lsA.DropMetricName().HasDuplicateLabelNames()

	s.Equal(expected, lsA.DropMetricName().String())
	s.False(has)
	s.Equal("", name)
}

func (s *LabelsSuite) TestHasDuplicateLabelNamesTrueDropMetricName() {
	lsA := labels.FromStrings(
		"__aaa__", "11111",
		"__name__", "ubername",
		"lol", "kek",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)
	name, has := lsA.DropMetricName().HasDuplicateLabelNames()

	s.True(has)
	s.Equal("lol", name)
}

func (s *LabelsSuite) TestHashDropMetricName() {
	lsA := labels.FromStrings(
		"__name__", "ubername",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	lsB := labels.FromStrings(
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	s.Equal(lsA.DropMetricName().Hash(), lsB.Hash())
}

func (s *LabelsSuite) TestHashDropMetricName_2() {
	lsA := labels.FromStrings(
		"__name__", "ubername",
		"__name__", "ubername2",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	lsB := labels.FromStrings(
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	s.Equal(lsA.DropMetricName().Hash(), lsB.Hash())
}

func (s *LabelsSuite) TestHashDropMetricName_3() {
	lsA := labels.FromStrings(
		"__name__", "ubername",
	)

	lsB := labels.FromStrings()

	s.Equal(lsA.DropMetricName().Hash(), lsB.Hash())
}

func (s *LabelsSuite) TestHashForLabelsDropMetricName() {
	lsA := labels.FromStrings(
		"__name__", "ubername",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	lsB := labels.FromStrings(
		"__aaa__", "11111",
		"__name__", "ubername",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	hash1 := lsA.DropMetricName().Hash()
	hash2, _ := lsB.HashForLabels(nil, "che", "lol", "zimya")

	s.Equal(hash1, hash2)
}

func (s *LabelsSuite) TestHashForLabelsDropMetricName_2() {
	lsA := labels.FromStrings(
		"__name__", "ubername",
		"__name__", "ubername2",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	lsB := labels.FromStrings(
		"__aaa__", "11111",
		"__name__", "ubername",
		"__name__", "ubername2",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	hash1 := lsA.DropMetricName().Hash()
	hash2, _ := lsB.HashForLabels(nil, "che", "lol", "zimya")

	s.Equal(hash1, hash2)
}

func (s *LabelsSuite) TestHashForLabelsDropMetricName_3() {
	lsA := labels.FromStrings(
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	lsB := labels.FromStrings(
		"__name__", "ubername",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	hash1 := lsA.DropMetricName().Hash()
	hash2, _ := lsB.DropMetricName().HashForLabels(nil, "__name__", "che", "lol", "zimya")

	s.Equal(hash1, hash2)
}

func (s *LabelsSuite) TestHashWithoutLabelsDropMetricName() {
	lsA := labels.FromStrings(
		"__name__", "ubername",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	lsB := labels.FromStrings(
		"__aaa__", "11111",
		"__name__", "ubername",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	hash1 := lsA.DropMetricName().Hash()
	hash2, _ := lsB.HashWithoutLabels(nil, "__aaa__")

	s.Equal(hash1, hash2)
}

func (s *LabelsSuite) TestHashWithoutLabelsDropMetricName_2() {
	lsA := labels.FromStrings(
		"__aaa__", "11111",
		"__name__", "ubername",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	lsB := labels.FromStrings(
		"__aaa__", "11111",
		"__name__", "ubername",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	hash1 := lsA.DropMetricName().Hash()
	hash2, _ := lsB.HashWithoutLabels(nil)

	s.Equal(hash1, hash2)
}

func (s *LabelsSuite) TestIsEmptyTrueDropMetricName() {
	lsA := labels.FromStrings("__name__", "ubername")

	s.False(lsA.IsEmpty())
	lsDMN := lsA.DropMetricName()
	s.True(lsDMN.IsEmpty())
}

func (s *LabelsSuite) TestLenDropMetricName() {
	lsIn := model.LabelSetFromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lss := cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage())
	lsid := lss.FindOrEmplace(lsIn)
	ls := labels.NewLabelsWithLSS(lss.Snapshot(), nil, lsid, 0)

	lsDMN := ls.DropMetricName()
	s.Equal(lsIn.Len()-1, lsDMN.Len())
}

func (s *LabelsSuite) TestLenDropMetricName_2() {
	lsIn := model.LabelSetFromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lss := cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage())
	lsid := lss.FindOrEmplace(lsIn)
	ls := labels.NewLabelsWithLSS(lss.Snapshot(), nil, lsid, uint16(lsIn.Len()))

	lsDMN := ls.DropMetricName()
	s.Equal(lsIn.Len()-1, lsDMN.Len())
}

func (s *LabelsSuite) TestLenDropMetricName_3() {
	lsIn := model.LabelSetFromMap(map[string]string{
		"lol": "kek",
		"che": "bureck",
	})

	lss := cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage())
	lsid := lss.FindOrEmplace(lsIn)
	ls := labels.NewLabelsWithLSS(lss.Snapshot(), nil, lsid, uint16(lsIn.Len()))

	lsDMN := ls.DropMetricName()
	s.Equal(lsIn.Len(), lsDMN.Len())
}

func (s *LabelsSuite) TestLenEmptyDropMetricName() {
	lsA := labels.EmptyLabels()

	lsDMN := lsA.DropMetricName()
	s.Equal(0, lsDMN.Len())
}

func (s *LabelsSuite) TestMatchLabelsTrueDropMetricName() {
	lsA := labels.FromStrings(
		"__aaa__", "11111",
		"__name__", "ubername",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	lsB := labels.FromStrings(
		"lol", "kek",
		"che", "bureck",
	)

	lsC := labels.FromStrings(
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	s.Equal(lsA.DropMetricName().MatchLabels(true, "__name__", "che", "lol").String(), lsB.String())
	s.Equal(lsA.DropMetricName().MatchLabels(true, "che", "lol", "zimya").String(), lsC.String())

	s.True(labels.Equal(lsA.DropMetricName().MatchLabels(true, "__name__", "che", "lol"), lsB))
	s.True(labels.Equal(lsA.DropMetricName().MatchLabels(true, "che", "lol", "zimya"), lsC))
}

func (s *LabelsSuite) TestMatchLabelsTrueDropMetricName_2() {
	lsA := labels.FromStrings(
		"__aaa__", "11111",
		"__name__", "ubername",
		"__name__", "ubername2",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	lsB := labels.FromStrings(
		"lol", "kek",
		"che", "bureck",
	)

	lsC := labels.FromStrings(
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	s.Equal(lsA.DropMetricName().MatchLabels(true, "__name__", "che", "lol").String(), lsB.String())
	s.Equal(lsA.DropMetricName().MatchLabels(true, "che", "lol", "zimya").String(), lsC.String())

	s.True(labels.Equal(lsA.DropMetricName().MatchLabels(true, "__name__", "che", "lol"), lsB))
	s.True(labels.Equal(lsA.DropMetricName().MatchLabels(true, "che", "lol", "zimya"), lsC))
}

func (s *LabelsSuite) TestMatchLabelsFalseDropMetricName() {
	lsA := labels.FromStrings(
		"__aaa__", "11111",
		"__name__", "ubername",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	lsB := labels.FromStrings(
		"__aaa__", "11111",
		"zimya", "reck",
	)

	lsC := labels.FromStrings(
		"__aaa__", "11111",
	)

	s.Equal(lsA.DropMetricName().MatchLabels(false, "__name__", "che", "lol").String(), lsB.String())
	s.Equal(lsA.DropMetricName().MatchLabels(false, "che", "lol", "zimya").String(), lsC.String())

	s.True(labels.Equal(lsA.DropMetricName().MatchLabels(false, "__name__", "che", "lol"), lsB))
	s.True(labels.Equal(lsA.DropMetricName().MatchLabels(false, "che", "lol", "zimya"), lsC))
}

func (s *LabelsSuite) TestMatchLabelsFalseDropMetricName_2() {
	lsA := labels.FromStrings(
		"__aaa__", "11111",
		"__name__", "ubername",
		"__name__", "ubername2",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	lsB := labels.FromStrings(
		"__aaa__", "11111",
		"zimya", "reck",
	)

	lsC := labels.FromStrings(
		"__aaa__", "11111",
	)

	s.Equal(lsA.DropMetricName().MatchLabels(false, "__name__", "che", "lol").String(), lsB.String())
	s.Equal(lsA.DropMetricName().MatchLabels(false, "che", "lol", "zimya").String(), lsC.String())

	s.True(labels.Equal(lsA.DropMetricName().MatchLabels(false, "__name__", "che", "lol"), lsB))
	s.True(labels.Equal(lsA.DropMetricName().MatchLabels(false, "che", "lol", "zimya"), lsC))
}

func (s *LabelsSuite) TestRangeDropMetricName() {
	lsMapA := map[string]string{
		"__aaa__":  "11111",
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	}
	lsA := labels.FromMap(lsMapA)

	lsMapB := make(map[string]string, len(lsMapA))
	lsA.DropMetricName().Range(func(l labels.Label) {
		lsMapB[l.Name] = l.Value
	})

	delete(lsMapA, "__name__")

	s.Equal(lsMapA, lsMapB)
}

func (s *LabelsSuite) TestValidateDropMetricName() {
	lsMapA := map[string]string{
		"__aaa__":  "11111",
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	}
	lsA := labels.FromMap(lsMapA)

	errEnd := errors.New("end")
	delete(lsMapA, "__name__")
	length := len(lsMapA)

	lsMapB := make(map[string]string, length)
	err := lsA.DropMetricName().Validate(func(l labels.Label) error {
		lsMapB[l.Name] = l.Value

		length--
		if length == 0 {
			return errEnd
		}
		return nil
	})

	s.Equal(lsMapA, lsMapB)
	s.ErrorIs(err, errEnd)
}

func (s *LabelsSuite) TestWithoutEmptyDropMetricName() {
	lsA := labels.FromStrings(
		"__aaa__", "11111",
		"che", "bureck",
		"zimya", "reck",
	)

	lsB := labels.FromStrings(
		"__aaa__", "11111",
		"__name__", "ubername",
		"lol", "",
		"che", "bureck",
		"zimya", "reck",
	)

	s.Equal(lsA.Bytes(nil), lsB.DropMetricName().WithoutEmpty().Bytes(nil))
	s.True(labels.Equal(lsA, lsB.DropMetricName().WithoutEmpty()))
}

func (s *LabelsSuite) TestIsZeroDropMetricName() {
	lsA := labels.FromStrings(
		"__name__", "ubername",
	)

	lsDMN := lsA.DropMetricName()
	s.True(lsDMN.IsZero())
}

func (s *LabelsSuite) TestGet() {
	lsMap := map[string]string{
		"__aaa__":  "11111",
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	}

	lsA := labels.FromMap(lsMap)

	for ln, lv := range lsMap {
		s.Equal(lv, lsA.Get(ln))
	}

	s.Equal("", lsA.Get("boo"))
}

func (s *LabelsSuite) TestHas() {
	lsMap := map[string]string{
		"__aaa__":  "11111",
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	}

	lsA := labels.FromMap(lsMap)

	for ln := range lsMap {
		s.True(lsA.Has(ln))
	}

	s.False(lsA.Has("boo"))
}

func (s *LabelsSuite) TestHasDuplicateLabelNamesFalse() {
	lsA := labels.FromStrings(
		"__aaa__", "11111",
		"__name__", "ubername",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)
	_, has := lsA.HasDuplicateLabelNames()

	s.False(has)
}

func (s *LabelsSuite) TestHasDuplicateLabelNamesTrue() {
	lsA := labels.FromStrings(
		"__aaa__", "11111",
		"__name__", "ubername",
		"lol", "kek",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)
	name, has := lsA.HasDuplicateLabelNames()

	s.True(has)
	s.Equal("lol", name)
}

func (s *LabelsSuite) TestHash() {
	lsA := labels.FromStrings(
		"__aaa__", "11111",
		"__name__", "ubername",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	lsB := labels.FromStrings(
		"__name__", "ubername",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	hash1, hash2, hash3 := lsA.Hash(), lsA.Hash(), lsB.Hash()

	s.Equal(hash1, hash2)
	s.NotEqual(hash3, hash1)
}

func (s *LabelsSuite) TestHashForLabels() {
	lsA := labels.FromStrings(
		"__name__", "ubername",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	lsB := labels.FromStrings(
		"__aaa__", "11111",
		"__name__", "ubername",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	hash1 := lsA.Hash()
	hash2, _ := lsB.HashForLabels(nil, "__name__", "che", "lol", "zimya")

	s.Equal(hash1, hash2)
}

func (s *LabelsSuite) TestHashWithoutLabels() {
	lsA := labels.FromStrings(
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	lsB := labels.FromStrings(
		"__aaa__", "11111",
		"__name__", "ubername",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	hash1 := lsA.Hash()
	hash2, _ := lsB.HashWithoutLabels(nil, "__aaa__")

	s.Equal(hash1, hash2)
}

func (s *LabelsSuite) TestHashWithoutLabels_2() {
	lsA := labels.FromStrings(
		"__aaa__", "11111",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	lsB := labels.FromStrings(
		"__aaa__", "11111",
		"__name__", "ubername",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	hash1 := lsA.Hash()
	hash2, _ := lsB.HashWithoutLabels(nil)

	s.Equal(hash1, hash2)
}

func (s *LabelsSuite) TestIsEmptyTrue() {
	lsA := labels.FromStrings()
	lsB := labels.EmptyLabels()

	s.True(lsA.IsEmpty())
	s.True(lsB.IsEmpty())
}

func (s *LabelsSuite) TestIsEmptyFalse() {
	lsA := labels.FromStrings(
		"__aaa__", "11111",
		"__name__", "ubername",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	s.False(lsA.IsEmpty())
}

func (s *LabelsSuite) TestLen() {
	lsIn := model.LabelSetFromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lss := cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage())
	lsid := lss.FindOrEmplace(lsIn)
	ls := labels.NewLabelsWithLSS(lss.Snapshot(), nil, lsid, 0)

	s.Equal(lsIn.Len(), ls.Len())
}

func (s *LabelsSuite) TestLen_2() {
	lsIn := model.LabelSetFromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	})

	lss := cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage())
	lsid := lss.FindOrEmplace(lsIn)
	ls := labels.NewLabelsWithLSS(lss.Snapshot(), nil, lsid, uint16(lsIn.Len()))

	s.Equal(lsIn.Len(), ls.Len())
}

func (s *LabelsSuite) TestLenEmpty() {
	lsA := labels.FromStrings()
	lsB := labels.EmptyLabels()

	s.Equal(0, lsA.Len())
	s.Equal(0, lsB.Len())
}

func (s *LabelsSuite) TestMatchLabelsTrue() {
	lsA := labels.FromStrings(
		"__aaa__", "11111",
		"__name__", "ubername",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	lsB := labels.FromStrings(
		"__name__", "ubername",
		"lol", "kek",
		"che", "bureck",
	)

	lsC := labels.FromStrings(
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	s.True(labels.Equal(lsA.MatchLabels(true, "__name__", "che", "lol"), lsB))
	s.True(labels.Equal(lsA.MatchLabels(true, "che", "lol", "zimya"), lsC))
}

func (s *LabelsSuite) TestMatchLabelsFalse() {
	lsA := labels.FromStrings(
		"__aaa__", "11111",
		"__name__", "ubername",
		"lol", "kek",
		"che", "bureck",
		"zimya", "reck",
	)

	lsB := labels.FromStrings(
		"__aaa__", "11111",
		"zimya", "reck",
	)

	lsC := labels.FromStrings(
		"__aaa__", "11111",
	)

	s.True(labels.Equal(lsA.MatchLabels(false, "__name__", "che", "lol"), lsB))
	s.True(labels.Equal(lsA.MatchLabels(false, "che", "lol", "zimya"), lsC))
}

func (s *LabelsSuite) TestRange() {
	lsMapA := map[string]string{
		"__aaa__":  "11111",
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	}
	lsA := labels.FromMap(lsMapA)

	lsMapB := make(map[string]string, len(lsMapA))
	lsA.Range(func(l labels.Label) {
		lsMapB[l.Name] = l.Value
	})

	s.Equal(lsMapA, lsMapB)
}

func (s *LabelsSuite) TestRenewSnapshot() {
	lsMapA := map[string]string{
		"__aaa__":  "11111",
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	}

	lsA := labels.FromMap(lsMapA)
	lsaCopy := lsA.Copy()

	s.True(lsA == lsaCopy)

	for i := 0; i < 100; i++ {
		labels.FromMap(map[string]string{
			"__aaa__":  fmt.Sprintf("11111%d", i),
			"__name__": fmt.Sprintf("ubernameB%d", i),
			"lol":      fmt.Sprintf("kekB%d", i),
			"che":      fmt.Sprintf("bureckB%d", i),
			"zimya":    fmt.Sprintf("reckB%d", i),
		})
	}

	lsA.RenewSnapshot()

	s.False(lsA == lsaCopy)
}

func (s *LabelsSuite) TestValidate() {
	lsMapA := map[string]string{
		"__aaa__":  "11111",
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	}
	lsA := labels.FromMap(lsMapA)

	errEnd := errors.New("end")
	length := len(lsMapA)
	lsMapB := make(map[string]string, length)
	err := lsA.Validate(func(l labels.Label) error {
		lsMapB[l.Name] = l.Value

		length--
		if length == 0 {
			return errEnd
		}
		return nil
	})

	s.Equal(lsMapA, lsMapB)
	s.ErrorIs(err, errEnd)
}

func (s *LabelsSuite) TestWithoutEmpty() {
	lsA := labels.FromStrings(
		"__aaa__", "11111",
		"__name__", "ubername",
		"che", "bureck",
		"zimya", "reck",
	)

	lsB := labels.FromStrings(
		"__aaa__", "11111",
		"__name__", "ubername",
		"lol", "",
		"che", "bureck",
		"zimya", "reck",
	)

	s.Equal(lsA.Bytes(nil), lsB.WithoutEmpty().Bytes(nil))
	s.True(labels.Equal(lsA, lsB.WithoutEmpty()))
}

func (s *LabelsSuite) TestIsZero() {
	lsA := labels.EmptyLabels()

	s.True(lsA.IsZero())
}

func (s *LabelsSuite) TestIsZeroFalse() {
	lsA := labels.FromStrings(
		"__aaa__", "11111",
		"__name__", "ubername",
		"che", "bureck",
		"zimya", "reck",
	)

	s.False(lsA.IsZero())
}

func (s *LabelsSuite) TestIsZeroFalseLSS() {
	lss := cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage())
	lsid := lss.FindOrEmplace(model.LabelSetFromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	}))
	lsA := labels.NewLabelsWithLSS(lss.Snapshot(), nil, lsid, 3)

	s.False(lsA.IsZero())
}

func BenchmarkLabels_Bytes(b *testing.B) {
	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})

	buf := make([]byte, 48)
	for i := 0; i < b.N; i++ {
		buf = lsA.Bytes(buf)
	}
}

func BenchmarkLabels_Range(b *testing.B) {
	lsA := labels.FromMap(map[string]string{
		"__aaa__":  "11111",
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})

	for i := 0; i < b.N; i++ {
		lsA.Range(func(l labels.Label) {
			_ = l
		})
	}
}

func BenchmarkLabels_RangeDropMetricName(b *testing.B) {
	lsA := labels.FromMap(map[string]string{
		"__aaa__":  "11111",
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})

	for i := 0; i < b.N; i++ {
		lsA.DropMetricName().Range(func(l labels.Label) {
			_ = l
		})
	}
}
