package cppbridge_test

import (
	"context"
	"testing"

	"github.com/prometheus/prometheus/pp/go/model"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/stretchr/testify/suite"
)

type LSSSuite struct {
	suite.Suite
	baseCtx context.Context
}

func TestLSSSuite(t *testing.T) {
	suite.Run(t, new(LSSSuite))
}

func (s *LSSSuite) SetupTest() {
	s.baseCtx = context.Background()
}

func (s *LSSSuite) TestLSS() {
	lss := cppbridge.NewLssStorage()

	s.Equal(uint64(0), lss.AllocatedMemory())
	cp := lss.Pointer()
	s.Require().NotEqual(0, cp)
}

func (s *LSSSuite) TestOrderedLSS() {
	lss := cppbridge.NewOrderedLssStorage()

	s.Equal(uint64(0), lss.AllocatedMemory())
	cp := lss.Pointer()
	s.Require().NotEqual(0, cp)
}

func (s *LSSSuite) TestQueryableLSS() {
	lss := cppbridge.NewQueryableLssStorage()

	s.NotEqual(uint64(0), lss.AllocatedMemory())
	cp := lss.Pointer()
	s.Require().NotEqual(0, cp)
}

func (s *LSSSuite) TestCreateReadonlyLssFromEncodingBimap() {
	// Arrange
	lss := cppbridge.NewLssStorage()

	// Act
	readonlyLss := lss.CreateReadonlyLss()

	// Assert
	s.Require().NotNil(readonlyLss.Pointer())
}

func (s *LSSSuite) TestCreateReadonlyLssFromQueryableEncodingBimap() {
	// Arrange
	lss := cppbridge.NewQueryableLssStorage()

	// Act
	readonlyLss := lss.CreateReadonlyLss()

	// Assert
	s.Require().NotNil(readonlyLss.Pointer())
}

type QueryableLSSSuite struct {
	suite.Suite
	baseCtx     context.Context
	lss         *cppbridge.LabelSetStorage
	labelSets   []model.LabelSet
	labelSetIDs []uint32
}

func TestQueryableLSSSuite(t *testing.T) {
	suite.Run(t, new(QueryableLSSSuite))
}

func (s *QueryableLSSSuite) SetupTest() {
	s.baseCtx = context.Background()
	s.lss = cppbridge.NewQueryableLssStorage()

	s.labelSets = []model.LabelSet{
		model.NewLabelSetBuilder().Set("lol", "kek").Build(),
		model.NewLabelSetBuilder().Set("lol", "kek").Set("che", "bureck").Build(),
		model.NewLabelSetBuilder().Set("lol", "kek").Set("zhe", "bureck").Build(),
		model.NewLabelSetBuilder().Set("foo", "bar").Build(),
		model.NewLabelSetBuilder().Set("foo", "baz").Build(),
	}

	s.labelSetIDs = make([]uint32, 0, len(s.labelSets))
	for _, labelSet := range s.labelSets {
		s.labelSetIDs = append(s.labelSetIDs, s.lss.FindOrEmplace(labelSet))
	}
}

func (s *QueryableLSSSuite) TestQuery() {
	// match with sorting
	{
		labelMatchers := []model.LabelMatcher{
			{Name: "lol", Value: "kek", MatcherType: model.MatcherTypeExactMatch},
		}
		queryResult := s.lss.Query(labelMatchers, cppbridge.LSSQuerySourceOther)
		s.Require().Equal(cppbridge.LSSQueryStatusMatch, queryResult.Status())
		s.Require().Len(queryResult.IDs(), 3)
		s.Require().Equal(s.labelSetIDs[1], queryResult.IDs()[0])
		s.Require().Equal(s.labelSetIDs[0], queryResult.IDs()[1])
		s.Require().Equal(s.labelSetIDs[2], queryResult.IDs()[2])
		s.Require().Equal(uint16(2), queryResult.LabelSetLengths()[0])
		s.Require().Equal(uint16(1), queryResult.LabelSetLengths()[1])
		s.Require().Equal(uint16(2), queryResult.LabelSetLengths()[2])
	}

	// no positive matchers
	{
		labelMatchers := []model.LabelMatcher{
			{Name: "kek", Value: "lol", MatcherType: model.MatcherTypeExactNotMatch},
		}
		queryResult := s.lss.Query(labelMatchers, cppbridge.LSSQuerySourceOther)
		s.Require().Equal(cppbridge.LSSQueryStatusNoPositiveMatchers, queryResult.Status())
	}

	// no match
	{
		labelMatchers := []model.LabelMatcher{
			{Name: "kek", Value: "lol", MatcherType: model.MatcherTypeExactMatch},
		}
		queryResult := s.lss.Query(labelMatchers, cppbridge.LSSQuerySourceOther)
		s.Require().Equal(cppbridge.LSSQueryStatusNoMatch, queryResult.Status())
	}

	// invalid regexp
	{
		labelMatchers := []model.LabelMatcher{
			{Name: "kek", Value: ".[", MatcherType: model.MatcherTypeRegexpMatch},
		}
		queryResult := s.lss.Query(labelMatchers, cppbridge.LSSQuerySourceOther)
		s.Require().Equal(cppbridge.LSSQueryStatusRegexpError, queryResult.Status())
	}
}

func (s *QueryableLSSSuite) TestGetLabelSets() {
	// Arrange

	// Act
	fetchedLabelSets := s.lss.GetLabelSets(s.labelSetIDs)

	// Assert
	s.Equal(labelSetToCppBridgeLabels(s.labelSets), fetchedLabelSets.LabelsSets())
}

func labelSetToCppBridgeLabels(labelSets []model.LabelSet) []cppbridge.Labels {
	result := make([]cppbridge.Labels, 0, len(labelSets))
	for _, labelSet := range labelSets {
		cppLabels := make(cppbridge.Labels, labelSet.Len())
		for i := 0; i < labelSet.Len(); i++ {
			cppLabels[i].Name = labelSet.Key(i)
			cppLabels[i].Value = labelSet.Value(i)
		}
		result = append(result, cppLabels)
	}

	return result
}

type queryLabelNameCase struct {
	matchers       []model.LabelMatcher
	expectedStatus uint32
	expectedNames  []string
}

var queryLabelNamesCases = []queryLabelNameCase{
	{
		matchers:       []model.LabelMatcher{},
		expectedStatus: cppbridge.LSSQueryStatusMatch,
		expectedNames:  []string{"che", "foo", "lol", "zhe"},
	},
	{
		matchers:       []model.LabelMatcher{{Name: "lol", Value: ".+", MatcherType: model.MatcherTypeRegexpMatch}},
		expectedStatus: cppbridge.LSSQueryStatusMatch,
		expectedNames:  []string{"che", "lol", "zhe"},
	},
}

func (s *QueryableLSSSuite) TestQueryLabelNames() {
	for _, testCase := range queryLabelNamesCases {
		s.testQueryLabelNamesImpl(testCase)
	}
}

func (s *QueryableLSSSuite) testQueryLabelNamesImpl(test_case queryLabelNameCase) {
	// Arrange

	// Act
	result := s.lss.QueryLabelNames(test_case.matchers)

	// Assert
	s.Equal(test_case.expectedStatus, result.Status())
	s.Equal(test_case.expectedNames, result.Names())
}

type queryLabelValuesCase struct {
	labelName      string
	matchers       []model.LabelMatcher
	expectedStatus uint32
	expectedValues []string
}

var queryLabelValuesCases = []queryLabelValuesCase{
	{
		labelName:      "foo",
		matchers:       []model.LabelMatcher{},
		expectedStatus: cppbridge.LSSQueryStatusMatch,
		expectedValues: []string{"bar", "baz"},
	},
	{
		labelName:      "foo",
		matchers:       []model.LabelMatcher{{Name: "foo", Value: ".+", MatcherType: model.MatcherTypeRegexpMatch}},
		expectedStatus: cppbridge.LSSQueryStatusMatch,
		expectedValues: []string{"bar", "baz"},
	},
}

func (s *QueryableLSSSuite) TestQueryLabelValues() {
	for _, testCase := range queryLabelValuesCases {
		s.testQueryLabelValuesImpl(testCase)
	}
}

func (s *QueryableLSSSuite) testQueryLabelValuesImpl(testCase queryLabelValuesCase) {
	// Arrange

	// Act
	result := s.lss.QueryLabelValues(testCase.labelName, testCase.matchers)

	// Assert
	s.Equal(testCase.expectedStatus, result.Status())
	s.Equal(testCase.expectedValues, result.Values())
}

func (s *QueryableLSSSuite) TestCopyAddedSeries() {
	// Arrange
	emptyLabelsSets := make([]cppbridge.Labels, len(s.labelSetIDs))
	lssCopy := cppbridge.NewQueryableLssStorage()
	lssCopyOfCopy := cppbridge.NewQueryableLssStorage()

	// Act
	s.lss.CopyAddedSeries(lssCopy)
	lssCopy.CopyAddedSeries(lssCopyOfCopy)

	// Assert
	s.Equal(labelSetToCppBridgeLabels(s.labelSets), lssCopy.GetLabelSets(s.labelSetIDs).LabelsSets())
	s.Equal(emptyLabelsSets, lssCopyOfCopy.GetLabelSets(s.labelSetIDs).LabelsSets())
}
