package querier

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

// SelectFuncOptimizationSuite checks replacement, combinations, and validation of optimization modes.
type SelectFuncOptimizationSuite struct {
	suite.Suite

	previous queryOptimizeType
}

func TestSelectFuncOptimizationSuite(t *testing.T) {
	suite.Run(t, new(SelectFuncOptimizationSuite))
}

func (s *SelectFuncOptimizationSuite) SetupTest() {
	s.previous = selectFuncOptimize
	selectFuncOptimize = noneOptimizeType
}

func (s *SelectFuncOptimizationSuite) TearDownTest() {
	selectFuncOptimize = s.previous
}

func (s *SelectFuncOptimizationSuite) TestCombination() {
	s.Require().NoError(SetSelectFuncOptimize("drop_point|cross"))

	s.Equal(dropPointOptimizeType|crossSeriesOptimizeType, selectFuncOptimize)
}

func (s *SelectFuncOptimizationSuite) TestInvalidOptionsKeepPreviousSelection() {
	s.Require().NoError(SetSelectFuncOptimize("new_point"))

	for _, opt := range []string{"", " ", "unknown", "drop_point|unknown", "drop_point|", "|cross"} {
		s.Require().Error(SetSelectFuncOptimize(opt), "option %q", opt)
		s.Equal(newPointOptimizeType, selectFuncOptimize, "option %q", opt)
	}
}

func (s *SelectFuncOptimizationSuite) TestOptionsApplyInOrder() {
	s.Require().NoError(SetSelectFuncOptimize("all|none|cross"))

	s.Equal(crossSeriesOptimizeType, selectFuncOptimize)
}

func (s *SelectFuncOptimizationSuite) TestSingleModesReplacePreviousSelection() {
	cases := []struct {
		option   string
		expected queryOptimizeType
	}{
		{option: "none", expected: noneOptimizeType},
		{option: "drop_point", expected: dropPointOptimizeType},
		{option: "new_point", expected: newPointOptimizeType},
		{option: "cross", expected: crossSeriesOptimizeType},
		{option: "all", expected: allOptimizeType},
	}

	for _, c := range cases {
		s.Require().NoError(SetSelectFuncOptimize("all"))
		s.Require().NoError(SetSelectFuncOptimize(c.option))
		s.Equal(c.expected, selectFuncOptimize, "option %q", c.option)
	}
}

func (s *SelectFuncOptimizationSuite) TestWhitespaceAroundOptions() {
	s.Require().NoError(SetSelectFuncOptimize(" \t drop_point \t | \n cross "))

	s.Equal(dropPointOptimizeType|crossSeriesOptimizeType, selectFuncOptimize)
}
