//go:build cpplabels

package labels_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
)

//
// ScratchBuilder
//

type ScratchBuilderSuite struct {
	suite.Suite
}

func TestScratchBuilderSuite(t *testing.T) {
	suite.Run(t, new(ScratchBuilderSuite))
}

func (s *ScratchBuilderSuite) TestAdd() {
	lsA := labels.FromMap(map[string]string{
		"__aaa__":  "11111",
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})

	builder := labels.NewScratchBuilder(lsA.Len())
	lsA.Range(func(l labels.Label) {
		builder.Add(l.Name, l.Value)
	})

	s.True(labels.Equal(lsA, builder.Labels()))
}

func (s *ScratchBuilderSuite) TestAssign() {
	lsA := labels.FromMap(map[string]string{
		"__aaa__":  "11111",
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})

	builder := labels.NewScratchBuilder(lsA.Len())
	builder.Assign(lsA)

	s.True(labels.Equal(lsA, builder.Labels()))
}

func (s *ScratchBuilderSuite) TestOverwrite() {
	lsA := labels.FromMap(map[string]string{
		"__aaa__":  "11111",
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})
	builder := labels.NewScratchBuilder(lsA.Len())
	builder.Assign(lsA)

	lsB := labels.EmptyLabels()

	builder.Overwrite(&lsB)

	s.True(labels.Equal(lsA, builder.Labels()))
	s.True(labels.Equal(lsA, lsB))
}

func (s *ScratchBuilderSuite) TestReset() {
	lsA := labels.FromMap(map[string]string{
		"__aaa__":  "11111",
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})
	builder := labels.NewScratchBuilder(lsA.Len())
	builder.Assign(lsA)
	builder.Sort()
	builder.Reset()

	s.False(labels.Equal(lsA, builder.Labels()))
}

func (s *ScratchBuilderSuite) TestUnsafeAddBytes() {
	lsA := labels.FromMap(map[string]string{
		"__aaa__":  "11111",
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})

	builder := labels.NewScratchBuilder(lsA.Len())
	lsA.Range(func(l labels.Label) {
		builder.UnsafeAddBytes([]byte(l.Name), []byte(l.Value))
	})

	s.True(labels.Equal(lsA, builder.Labels()))
}
