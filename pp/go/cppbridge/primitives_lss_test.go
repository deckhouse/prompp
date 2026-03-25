package cppbridge_test

import (
	"context"
	"runtime"
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

func (s *LSSSuite) TestQueryableLSS() {
	lss := cppbridge.NewQueryableLssStorage()

	s.NotEqual(uint64(0), lss.AllocatedMemory())
	cp := lss.Pointer()
	s.Require().NotEqual(0, cp)
}

func (s *LSSSuite) TestCreateSnapshotFromEncodingBimap() {
	// Arrange
	lss := cppbridge.NewLssStorage()

	// Act
	labelSetSnapshot := lss.CreateLabelSetSnapshot()

	// Assert
	s.Require().NotNil(labelSetSnapshot.Pointer())
}

func (s *LSSSuite) TestCreateSnapshotFromQueryableEncodingBimap() {
	// Arrange
	lss := cppbridge.NewQueryableLssStorage()

	// Act
	labelSetSnapshot := lss.CreateLabelSetSnapshot()

	// Assert
	s.Require().NotNil(labelSetSnapshot.Pointer())
}

func (s *LSSSuite) TestLabels() {
	lsMap := map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	}

	lsIn := model.LabelSetFromMap(lsMap)

	lss := cppbridge.NewQueryableLssStorage()
	lsID := lss.FindOrEmplace(lsIn).LabelSetID

	lsLength := 0
	_ = lss.RangeLabelSet(lsID, func(l cppbridge.Label) error {
		lv, ok := lsMap[l.Name]
		s.Require().True(ok)
		s.Require().Equal(lv, l.Value)
		lsLength++

		return nil
	})

	s.Equal(lsIn.Len(), lsLength)
}

type bytesTestCase struct {
	labelSet model.LabelSet
	expected []byte
}

func (s *LSSSuite) TestBytes() {
	testCases := []bytesTestCase{
		{
			labelSet: model.NewLabelSetBuilder().Set("key", "value").Build(),
			expected: []byte("\xFEkey\xFFvalue"),
		},
		{
			labelSet: model.NewLabelSetBuilder().Set("key1", "value1").Set("key2", "value2").Build(),
			expected: []byte("\xFEkey1\xFFvalue1\xFFkey2\xFFvalue2"),
		},
	}

	var bytes []byte
	for _, testCase := range testCases {
		s.testBytesImpl(testCase, &bytes)
	}
}

func (s *LSSSuite) testBytesImpl(testCase bytesTestCase, bytes *[]byte) {
	// Arrange
	lss := cppbridge.NewLssStorage()
	lss.FindOrEmplace(testCase.labelSet)

	// Act
	*bytes = cppbridge.LabelSetBytes(lss.Pointer(), 0, *bytes)
	// TODO: public interface must work without outside keep alives
	runtime.KeepAlive(lss)

	// Assert
	s.Equal(testCase.expected, *bytes)
}

type bytesWithFilteredNamesTestCase struct {
	labelSet model.LabelSet
	names    []string
	expected []byte
}

func (s *LSSSuite) TestBytesWithLabels() {
	testCases := []bytesWithFilteredNamesTestCase{
		{
			labelSet: model.NewLabelSetBuilder().Set("key", "value").Build(),
			names:    []string{"key", "key1", "key2"},
			expected: []byte("\xFEkey\xFFvalue"),
		},
		{
			labelSet: model.NewLabelSetBuilder().Set("key", "value").Build(),
			names:    []string{"non_existing_key"},
			expected: []byte("\xFE"),
		},
		{
			labelSet: model.NewLabelSetBuilder().Set("key1", "value1").Set("key2", "value2").Build(),
			names:    []string{"key1", "key2"},
			expected: []byte("\xFEkey1\xFFvalue1\xFFkey2\xFFvalue2"),
		},
	}

	var bytes []byte
	for _, testCase := range testCases {
		s.testBytesWithLabelsImpl(testCase, &bytes)
	}
}

func (s *LSSSuite) testBytesWithLabelsImpl(testCase bytesWithFilteredNamesTestCase, bytes *[]byte) {
	// Arrange
	lss := cppbridge.NewLssStorage()
	lss.FindOrEmplace(testCase.labelSet)

	// Act
	*bytes = cppbridge.LabelSetBytesWithLabels(lss.Pointer(), 0, *bytes, testCase.names...)
	// TODO: public interface must work without outside keep alives
	runtime.KeepAlive(lss)

	// Assert
	s.Equal(testCase.expected, *bytes)
}

func (s *LSSSuite) TestBytesWithoutLabels() {
	testCases := []bytesWithFilteredNamesTestCase{
		{
			labelSet: model.NewLabelSetBuilder().Set("key1", "value1").Set("key2", "value2").Build(),
			names:    []string{"key1", "key2"},
			expected: []byte("\xFE"),
		},
		{
			labelSet: model.NewLabelSetBuilder().Set("key1", "value1").Set("key2", "value2").Build(),
			names:    []string{"key1"},
			expected: []byte("\xFEkey2\xFFvalue2"),
		},
		{
			labelSet: model.NewLabelSetBuilder().Set("key1", "value1").Set("key2", "value2").Build(),
			names:    []string{"key2"},
			expected: []byte("\xFEkey1\xFFvalue1"),
		},
		{
			labelSet: model.NewLabelSetBuilder().Set("key", "value").Build(),
			names:    []string{"key", "key1", "key2"},
			expected: []byte("\xFE"),
		},
		{
			labelSet: model.NewLabelSetBuilder().Set("key", "value").Build(),
			names:    []string{"non_existing_key"},
			expected: []byte("\xFEkey\xFFvalue"),
		},
	}

	var bytes []byte
	for _, testCase := range testCases {
		s.testBytesWithoutLabelsImpl(testCase, &bytes)
	}
}

func (s *LSSSuite) testBytesWithoutLabelsImpl(testCase bytesWithFilteredNamesTestCase, bytes *[]byte) {
	// Arrange
	lss := cppbridge.NewLssStorage()
	lss.FindOrEmplace(testCase.labelSet)

	// Act
	*bytes = cppbridge.LabelSetBytesWithoutLabels(lss.Pointer(), 0, *bytes, testCase.names...)
	// TODO: public interface must work without outside keep alives
	runtime.KeepAlive(lss)

	// Assert
	s.Equal(testCase.expected, *bytes)
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
		s.labelSetIDs = append(s.labelSetIDs, s.lss.FindOrEmplace(labelSet).LabelSetID)
	}
}

func (s *QueryableLSSSuite) TestQuery() {
	// match with sorting
	{
		labelMatchers := []model.LabelMatcher{
			{Name: "lol", Value: "kek", MatcherType: model.MatcherTypeExactMatch},
		}
		selector, status := s.lss.QuerySelector(labelMatchers)
		s.Require().Equal(cppbridge.LSSQueryStatusMatch, status)
		snapshot := s.lss.CreateLabelSetSnapshot()
		queryResult := snapshot.Query(selector)
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
		_, status := s.lss.QuerySelector(labelMatchers)
		s.Require().Equal(cppbridge.LSSQueryStatusNoPositiveMatchers, status)
	}

	// no match
	{
		labelMatchers := []model.LabelMatcher{
			{Name: "kek", Value: "lol", MatcherType: model.MatcherTypeExactMatch},
		}
		_, status := s.lss.QuerySelector(labelMatchers)
		s.Require().Equal(cppbridge.LSSQueryStatusNoMatch, status)
	}

	// invalid regexp
	{
		labelMatchers := []model.LabelMatcher{
			{Name: "kek", Value: ".[", MatcherType: model.MatcherTypeRegexpMatch},
		}
		_, status := s.lss.QuerySelector(labelMatchers)
		s.Require().Equal(cppbridge.LSSQueryStatusRegexpError, status)
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

func (s *QueryableLSSSuite) TestFindOrEmplaceBuilderWithExistingLabelSet() {
	// Arrange
	labelSetSnapshot := s.lss.CreateLabelSetSnapshot()

	// Act
	existingLsIdWithAdd := s.lss.FindOrEmplaceBuilder(cppbridge.CppLabelSetBuilder{
		ReadonlyLss: labelSetSnapshot.Pointer(),
		LsId:        0,
		SortedAdd:   []cppbridge.Label{{Name: "che", Value: "bureck"}},
		SortedDel:   nil,
	}).LabelSetID
	existingLsIdWithDel := s.lss.FindOrEmplaceBuilder(cppbridge.CppLabelSetBuilder{
		ReadonlyLss: labelSetSnapshot.Pointer(),
		LsId:        1,
		SortedAdd:   nil,
		SortedDel:   []string{"che"},
	}).LabelSetID

	// TODO: public object should encapsulate keep alives
	runtime.KeepAlive(labelSetSnapshot)

	// Assert
	s.Equal(uint32(1), existingLsIdWithAdd)
	s.Equal(uint32(0), existingLsIdWithDel)
}

func (s *QueryableLSSSuite) TestFindOrEmplaceBuilderWithNewLabelSet() {
	// Arrange
	labelSetSnapshot := s.lss.CreateLabelSetSnapshot()

	// Act
	expectedLsId := len(s.labelSetIDs)
	existingLsId := s.lss.FindOrEmplaceBuilder(cppbridge.CppLabelSetBuilder{
		ReadonlyLss: labelSetSnapshot.Pointer(),
		LsId:        0,
		SortedAdd:   []cppbridge.Label{{Name: "new_lol", Value: "new_kek"}},
		SortedDel:   nil,
	}).LabelSetID
	// TODO: public object should encapsulate keep alives
	runtime.KeepAlive(labelSetSnapshot)

	// Assert
	s.Equal(uint32(expectedLsId), existingLsId)
}

func (s *QueryableLSSSuite) TestFindOrEmplaceBuilderWithoutReadonlyLss() {
	// Arrange

	// Act
	expectedLsId := len(s.labelSetIDs)
	existingLsId := s.lss.FindOrEmplaceBuilder(cppbridge.CppLabelSetBuilder{
		ReadonlyLss: uintptr(0),
		LsId:        0,
		SortedAdd:   []cppbridge.Label{{Name: "new_lol", Value: "new_kek"}},
		SortedDel:   nil,
	}).LabelSetID

	// Assert
	s.Equal(uint32(expectedLsId), existingLsId)
}

func (s *QueryableLSSSuite) TestCopyAddedSeriesFromSnapshot() {
	// Arrange
	emptyLabelsSets := make([]cppbridge.Labels, len(s.labelSetIDs))
	lssCopy := cppbridge.NewQueryableLssStorage()
	lssCopyOfCopy := cppbridge.NewQueryableLssStorage()

	// Act
	snapshot := s.lss.CreateLabelSetSnapshot()
	bitsetSeries := s.lss.BitsetSeries()
	snapshot.CopyAddedSeries(bitsetSeries, lssCopy)

	snapshotCopy := lssCopy.CreateLabelSetSnapshot()
	bitsetSeriesCopy := lssCopy.BitsetSeries()
	snapshotCopy.CopyAddedSeries(bitsetSeriesCopy, lssCopyOfCopy)

	// Assert
	// !!!ATTENTION!!! When copying the added series, the order in which the series are added is preserved.
	s.Equal(labelSetToCppBridgeLabels(s.labelSets), lssCopy.GetLabelSets(s.labelSetIDs).LabelsSets())
	s.Equal(emptyLabelsSets, lssCopyOfCopy.GetLabelSets(s.labelSetIDs).LabelsSets())
}
