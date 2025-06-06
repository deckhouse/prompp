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
	_ = lss.RangeLabelSet(lsID, false, func(l cppbridge.Label) error {
		lv, ok := lsMap[l.Name]
		s.Require().True(ok)
		s.Require().Equal(lv, l.Value)
		lsLength++

		return nil
	})

	s.Equal(lsIn.Len(), lsLength)
}

func (s *LSSSuite) TestCreateReadonlyLssFromEncodingBimap() {
	// Arrange
	lss := cppbridge.NewLssStorage()

	// Act
	snapshot := lss.CreateLabelSetSnapshot()

	// Assert
	s.Require().NotNil(snapshot.Pointer())
}

func (s *LSSSuite) TestCreateReadonlyLssFromQueryableEncodingBimap() {
	// Arrange
	lss := cppbridge.NewQueryableLssStorage()

	// Act
	snapshot := lss.CreateLabelSetSnapshot()

	// Assert
	s.Require().NotNil(snapshot.Pointer())
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

func (s *QueryableLSSSuite) TestFindOrEmplaceFromBuilderWithExistingLabelSet() {
	// Arrange
	labelSetSnapshot := s.lss.CreateLabelSetSnapshot()

	// Act
	snapshotWithAdd, lengthWithAdd, existingLsIdWithAdd := s.lss.FindOrEmplaceFromBuilder(
		[]cppbridge.Label{{Name: "che", Value: "bureck"}},
		nil,
		labelSetSnapshot,
		0,
	)
	snapshotWithDel, lengthWithDel, existingLsIdWithDel := s.lss.FindOrEmplaceFromBuilder(
		nil,
		[]string{"che"},
		labelSetSnapshot,
		1,
	)

	// Assert
	s.Equal(uint64(2), lengthWithAdd)
	s.Equal(uint64(1), lengthWithDel)
	s.Nil(snapshotWithAdd)
	s.Nil(snapshotWithDel)
	s.Equal(uint32(1), existingLsIdWithAdd)
	s.Equal(uint32(0), existingLsIdWithDel)
}

func (s *QueryableLSSSuite) TestFindOrEmplaceFromBuilderWithNewLabelSet() {
	// Arrange
	labelSetSnapshot := s.lss.CreateLabelSetSnapshot()

	// Act
	expectedLsId := len(s.labelSetIDs)
	snapshot, length, existingLsId := s.lss.FindOrEmplaceFromBuilder(
		[]cppbridge.Label{{Name: "new_lol", Value: "new_kek"}},
		nil,
		labelSetSnapshot,
		0,
	)

	// Assert
	s.Equal(uint64(2), length)
	s.NotNil(snapshot)
	s.Equal(uint32(expectedLsId), existingLsId)
}

func (s *QueryableLSSSuite) TestFindOrEmplaceFromBuilderWithNewLabelSetAnother() {
	// Arrange
	mls := s.labelSets[0]
	lss := cppbridge.NewQueryableLssStorage()
	lsid := lss.FindOrEmplace(mls).LabelSetID
	labelSetSnapshot := s.lss.CreateLabelSetSnapshot()

	// Act
	expectedLsId := len(s.labelSetIDs)
	snapshot, length, existingLsId := s.lss.FindOrEmplaceFromBuilder(
		[]cppbridge.Label{{Name: "new_lol", Value: "new_kek"}},
		nil,
		labelSetSnapshot,
		lsid,
	)

	// Assert
	s.Equal(uint64(2), length)
	s.NotNil(snapshot)
	s.Equal(uint32(expectedLsId), existingLsId)
}

func (s *QueryableLSSSuite) TestFindOrEmplaceLabelSet() {
	// Arrange
	mls := model.LabelSetFromMap(map[string]string{
		"__name__": "somename",
		"job":      "somejob",
	})
	builder := model.NewLabelSetSimpleBuilderSize(mls.Len())

	// Act
	snapshot, lsID := s.lss.FindOrEmplaceLabelSet(mls)
	s.Require().NotNil(snapshot)

	snapshot.RangeLabelSet(lsID, false, func(l cppbridge.Label) error {
		builder.Add(l.Name, l.Value)
		return nil
	})

	// Assert
	s.Equal(mls.String(), builder.Build().String())
}

func (s *QueryableLSSSuite) TestFind() {
	// Arrange
	mls := model.LabelSetFromMap(map[string]string{
		"__name__": "somename",
		"job":      "somejob",
	})

	// Act
	_, expectedLSID := s.lss.FindOrEmplaceLabelSet(mls)
	actualLSID, find := s.lss.Find(mls)

	// Assert
	s.Require().True(find)
	s.Equal(expectedLSID, actualLSID)
}

func (s *QueryableLSSSuite) TestFindNot() {
	// Arrange
	mls1 := model.LabelSetFromMap(map[string]string{
		"__name__": "somename",
		"job":      "somejob",
	})
	mls2 := model.LabelSetFromMap(map[string]string{
		"__name__": "somename",
		"job":      "somejob1",
	})
	_, _ = s.lss.FindOrEmplaceLabelSet(mls1)

	// Act
	_, find := s.lss.Find(mls2)

	// Assert
	s.Require().False(find)
}

func (s *QueryableLSSSuite) TestFindFromBuilder() {
	// Arrange
	mls := model.LabelSetFromMap(map[string]string{
		"__name__": "somename",
		"job":      "somejob",
	})

	// Act
	_, expectedLSID := s.lss.FindOrEmplaceLabelSet(mls)
	labelSetSnapshot := s.lss.CreateLabelSetSnapshot()
	length, actualLSID, find := s.lss.FindFromBuilder(
		nil,
		nil,
		labelSetSnapshot,
		expectedLSID,
	)

	// Assert
	s.Require().True(find)
	s.Equal(mls.Len(), int(length))
	s.Equal(expectedLSID, actualLSID)
}

func (s *QueryableLSSSuite) TestFindFromBuilderAnother() {
	// Arrange
	mls := s.labelSets[0]
	lss := cppbridge.NewQueryableLssStorage()
	lsid := lss.FindOrEmplace(mls).LabelSetID
	labelSetSnapshot := lss.CreateLabelSetSnapshot()

	// Act
	_, expectedLSID := s.lss.FindOrEmplaceLabelSet(mls)
	length, actualLSID, find := s.lss.FindFromBuilder(
		nil,
		nil,
		labelSetSnapshot,
		lsid,
	)

	// Assert
	s.Require().True(find)
	s.Equal(s.labelSets[0].Len(), int(length))
	s.Equal(expectedLSID, actualLSID)
}

func (s *QueryableLSSSuite) TestFindFromBuilderFromBuilderWithExistingLabelSet() {
	// Arrange
	labelSetSnapshot := s.lss.CreateLabelSetSnapshot()

	// Act
	lengthAdd, lsIDAdd, findAdd := s.lss.FindFromBuilder(
		[]cppbridge.Label{{Name: "che", Value: "bureck"}},
		nil,
		labelSetSnapshot,
		0,
	)
	lengthDel, lsIDDel, findDel := s.lss.FindFromBuilder(
		nil,
		[]string{"che"},
		labelSetSnapshot,
		1,
	)

	// Assert
	s.Require().True(findAdd)
	s.Equal(s.labelSets[1].Len(), int(lengthAdd))
	s.Equal(s.labelSetIDs[1], lsIDAdd)

	s.Require().True(findDel)
	s.Equal(s.labelSets[0].Len(), int(lengthDel))
	s.Equal(s.labelSetIDs[0], lsIDDel)
}

func (s *QueryableLSSSuite) TestFindFromBuilderNot() {
	// Arrange
	mls := model.LabelSetFromMap(map[string]string{
		"__name__": "somename",
		"job":      "somejob1",
	})
	lss := cppbridge.NewQueryableLssStorage()
	lsid := lss.FindOrEmplace(mls).LabelSetID
	labelSetSnapshot := lss.CreateLabelSetSnapshot()

	// Act
	_, _, find := s.lss.FindFromBuilder(
		nil,
		nil,
		labelSetSnapshot,
		lsid,
	)

	// Assert
	s.Require().False(find)
}

//
// TEST
//

func (s *QueryableLSSSuite) TestFindOrEmplace() {
	// Arrange
	mls := model.NewLabelSetBuilder().Set("__name__", "somename").Set("job", "somejob").Build()
	lss1 := cppbridge.NewLssStorage()
	lss2 := cppbridge.NewQueryableLssStorage()

	// Act
	res1 := lss1.FindOrEmplace(mls)
	res2 := lss2.FindOrEmplace(mls)

	s.T().Log(res1.LabelSetID, res1.LssHasReallocations)
	s.T().Log(res2.LabelSetID, res2.LssHasReallocations)
}

func (s *QueryableLSSSuite) TestFindOrEmplaceFromBuilderWithNewLabelSet2() {
	// Arrange
	mls := model.NewLabelSetBuilder().Set("__name__", "somename").Set("job", "somejob").Build()
	res := s.lss.FindOrEmplace(mls)
	labelSetSnapshot := s.lss.CreateLabelSetSnapshot()

	labelSetSnapshot.RangeLabelSet(res.LabelSetID, false, func(l cppbridge.Label) error {
		s.T().Log(l.Name, l.Value)
		return nil
	})

	// Act
	snapshot, _, existingLsId := s.lss.FindOrEmplaceFromBuilder(
		nil,
		// []cppbridge.Label{{Name: "new_lol", Value: "new_kek"}},
		[]string{"__name__"},
		labelSetSnapshot,
		res.LabelSetID,
	)

	s.T().Log(snapshot, existingLsId)

	if snapshot != nil {
		labelSetSnapshot = snapshot
	}

	labelSetSnapshot.RangeLabelSet(existingLsId, false, func(l cppbridge.Label) error {
		s.T().Log(l.Name, l.Value)
		return nil
	})

	// Assert
	// s.NotNil(snapshot)
	// s.Equal(uint32(expectedLsId), existingLsId)
}
