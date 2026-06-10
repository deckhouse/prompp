package cppbridge_test

import (
	"context"
	"math"
	"runtime"
	"slices"
	"testing"
	"unique"

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

func (s *QueryableLSSSuite) TestGetLabelNameIDs() {
	// Arrange

	// Act
	out := s.lss.GetLabelNameIDs([]string{"lol", "foo", "nope", "lol"})

	// Assert
	s.Equal([]uint32{0, 3, math.MaxUint32, 0}, out)
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
		SnapshotPtr: labelSetSnapshot.Pointer(),
		LsId:        0,
		SortedAdd:   []cppbridge.Label{{Name: "che", Value: "bureck"}},
		SortedDel:   nil,
	}).LabelSetID
	existingLsIdWithDel := s.lss.FindOrEmplaceBuilder(cppbridge.CppLabelSetBuilder{
		SnapshotPtr: labelSetSnapshot.Pointer(),
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
		SnapshotPtr: labelSetSnapshot.Pointer(),
		LsId:        0,
		SortedAdd:   []cppbridge.Label{{Name: "new_lol", Value: "new_kek"}},
		SortedDel:   nil,
	}).LabelSetID
	// TODO: public object should encapsulate keep alives
	runtime.KeepAlive(labelSetSnapshot)

	// Assert
	s.Equal(uint32(expectedLsId), existingLsId)
}

func (s *QueryableLSSSuite) TestFindOrEmplaceBuilderWithoutSnapshot() {
	// Arrange

	// Act
	expectedLsId := len(s.labelSetIDs)
	existingLsId := s.lss.FindOrEmplaceBuilder(cppbridge.CppLabelSetBuilder{
		SnapshotPtr: uintptr(0),
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

//
// RotateLSSSuite
//

type RotateLSSSuite struct {
	suite.Suite

	lss         *cppbridge.LabelSetStorage
	labelSets   []model.LabelSet
	labelSetIDs []uint32
	matchers    []model.LabelMatcher
}

func TestRotateLSSSuite(t *testing.T) {
	suite.Run(t, new(RotateLSSSuite))
}

func (s *RotateLSSSuite) SetupTest() {
	s.lss = cppbridge.NewQueryableLssStorage()

	s.labelSets = []model.LabelSet{
		model.LabelSetFromPairs("__name__", "kek"),
		model.LabelSetFromPairs("__name__", "kek", "che", "bureck"),
		model.LabelSetFromPairs("__name__", "kek", "zhe", "bureck"),
		model.LabelSetFromPairs("__name__", "foo_container", "foo", "bar"),
		model.LabelSetFromPairs("__name__", "foo_container", "foo", "baz", "zt", "uuid"),
		model.LabelSetFromPairs("__name__", "zram", "dev", "/dev/zram0"),
		model.LabelSetFromPairs("__name__", "zram", "dev", "/dev/zram1"),
	}

	s.labelSetIDs = make([]uint32, 0, len(s.labelSets))
	for _, labelSet := range s.labelSets {
		s.labelSetIDs = append(s.labelSetIDs, s.lss.FindOrEmplace(labelSet).LabelSetID)
	}

	s.matchers = []model.LabelMatcher{
		{Name: "__name__", Value: "kek", MatcherType: model.MatcherTypeExactMatch},
	}
}

func (s *RotateLSSSuite) TestCopyOnRotate() {
	// Arrange
	newLSS := cppbridge.NewQueryableLssStorage()
	shrinkBoundary := slices.Max(s.labelSetIDs) + 1

	// Act
	result := s.rotate(shrinkBoundary, s.lss, newLSS)

	// Assert
	// !!!ATTENTION!!! When copying the added series, the order in which the series are added is preserved.
	expectedLabelSets := labelSetToCppBridgeLabels(s.labelSets)
	s.Equal(expectedLabelSets, newLSS.GetLabelSets(s.labelSetIDs).LabelsSets())

	// QueryLabelNames
	namesNew := newLSS.QueryLabelNames([]model.LabelMatcher{})
	namesOld := s.lss.QueryLabelNames([]model.LabelMatcher{})
	s.Equal(namesNew.Names(), namesOld.Names())

	// QueryLabelValues
	valuesNew := newLSS.QueryLabelValues("foo", []model.LabelMatcher{})
	valuesOld := s.lss.QueryLabelValues("foo", []model.LabelMatcher{})
	s.Equal(valuesNew.Values(), valuesOld.Values())

	selector := s.mustQuerySelector(s.matchers)
	snapshot := s.lss.CreateLabelSetSnapshot()
	s.Equal(labelSetToCppBridgeLabels(s.labelSets[:3]), s.convertQueryResultToLabelSets(snapshot, selector))

	runtime.KeepAlive(newLSS)
	runtime.KeepAlive(snapshot)
	runtime.KeepAlive(result)
}

func (s *RotateLSSSuite) TestOldLSSSnapshotFinalizer() {
	// Arrange
	newLSS := cppbridge.NewQueryableLssStorage()
	shrinkBoundary := slices.Max(s.labelSetIDs) + 1

	// Act
	_ = s.rotate(shrinkBoundary, s.lss, newLSS)
	runtime.GC()
	runtime.GC()

	// Assert
	selector := s.mustQuerySelector(s.matchers)
	snapshot := s.lss.CreateLabelSetSnapshot()
	runtime.GC()
	runtime.GC()

	s.Equal(labelSetToCppBridgeLabels(s.labelSets[:3]), s.convertQueryResultToLabelSets(snapshot, selector))

	runtime.KeepAlive(snapshot)
}

func (s *RotateLSSSuite) TestCopyOnRotateCheckQueryOnOldSnapshot() {
	// Arrange
	newLSS := cppbridge.NewQueryableLssStorage()
	shrinkBoundary := slices.Max(s.labelSetIDs) + 1

	// Act
	s.T().Log("Rotate LSS")
	snapshot := s.lss.CreateLabelSetSnapshot()
	s.lss.SetPendingShrinkBoundary(shrinkBoundary)
	dstSrcLsIdsMapping := snapshot.CopyAddedSeries(s.lss.BitsetSeries(), newLSS)
	mappedSnapshot := newLSS.CreateLabelSetSnapshot()

	selector, status := s.lss.QuerySelector(s.matchers)
	s.Require().Equal(cppbridge.LSSQueryStatusMatch, status)

	s.lss.FinalizeCopyAndShrink(mappedSnapshot, dstSrcLsIdsMapping)

	// Assert
	// !!!ATTENTION!!! When copying the added series, the order in which the series are added is preserved.
	expectedLabelSets := labelSetToCppBridgeLabels(s.labelSets)
	s.Equal(expectedLabelSets, newLSS.GetLabelSets(s.labelSetIDs).LabelsSets())

	s.T().Log("Checking query on snapshot")
	s.Equal(labelSetToCppBridgeLabels(s.labelSets[:3]), s.convertQueryResultToLabelSets(snapshot, selector))

	runtime.KeepAlive(newLSS)
	runtime.KeepAlive(snapshot)
	runtime.KeepAlive(dstSrcLsIdsMapping)
	runtime.KeepAlive(mappedSnapshot)
}

func (s *RotateLSSSuite) TestCheckQueryOnFreezeLSS() {
	// Arrange
	shrinkBoundary := slices.Max(s.labelSetIDs) + 1

	// Act
	selector, status := s.lss.QuerySelector(s.matchers)
	s.Require().Equal(cppbridge.LSSQueryStatusMatch, status)

	s.lss.SetPendingShrinkBoundary(shrinkBoundary)
	snapshot := s.lss.CreateLabelSetSnapshot()

	// Assert
	s.T().Log("Checking query on snapshot")
	s.Equal(labelSetToCppBridgeLabels(s.labelSets[:3]), s.convertQueryResultToLabelSets(snapshot, selector))

	runtime.KeepAlive(snapshot)
}

func (s *RotateLSSSuite) TestCheckQueryOnFreezeLSS2() {
	// Arrange
	shrinkBoundary := slices.Max(s.labelSetIDs) + 1

	// Act
	s.lss.SetPendingShrinkBoundary(shrinkBoundary)

	selector, status := s.lss.QuerySelector(s.matchers)
	s.Require().Equal(cppbridge.LSSQueryStatusMatch, status)

	snapshot := s.lss.CreateLabelSetSnapshot()

	// Assert
	s.T().Log("Checking query on snapshot")
	s.Equal(labelSetToCppBridgeLabels(s.labelSets[:3]), s.convertQueryResultToLabelSets(snapshot, selector))

	runtime.KeepAlive(snapshot)
}

func (s *RotateLSSSuite) TestCopyOnRotateEmplaceNewLS() {
	// Arrange
	newLSS := cppbridge.NewQueryableLssStorage()
	shrinkBoundary := slices.Max(s.labelSetIDs) + 1

	// Act
	result := s.rotate(shrinkBoundary, s.lss, newLSS)
	lsNew := model.LabelSetFromPairs("__name__", "kek1")
	lsIDNew := s.lss.FindOrEmplace(lsNew).LabelSetID

	// Assert
	// !!!ATTENTION!!! When copying the added series, the order in which the series are added is preserved.
	expectedLabelSets := labelSetToCppBridgeLabels(s.labelSets)
	s.Equal(expectedLabelSets, newLSS.GetLabelSets(s.labelSetIDs).LabelsSets())
	s.Equal(shrinkBoundary, lsIDNew)
	s.Equal(
		labelSetToCppBridgeLabels(append(s.labelSets, lsNew)),
		s.lss.GetLabelSets(append(s.labelSetIDs, lsIDNew)).LabelsSets(),
	)

	selector := s.mustQuerySelector(s.matchers)
	snapshot := s.lss.CreateLabelSetSnapshot()
	s.Equal(labelSetToCppBridgeLabels(s.labelSets[:3]), s.convertQueryResultToLabelSets(snapshot, selector))

	s.T().Log("Checking query on snapshot with new ls")
	selector = s.mustQuerySelector(
		[]model.LabelMatcher{{Name: "__name__", Value: "ke.*", MatcherType: model.MatcherTypeRegexpMatch}},
	)
	s.Equal(
		labelSetToCppBridgeLabels(append(s.labelSets[:3], lsNew)),
		s.convertQueryResultToLabelSets(snapshot, selector),
	)

	runtime.KeepAlive(newLSS)
	runtime.KeepAlive(snapshot)
	runtime.KeepAlive(result)
}

func (s *RotateLSSSuite) TestCopyOnRotateEmplaceExistingLS() {
	// Arrange
	newLSS := cppbridge.NewQueryableLssStorage()
	shrinkBoundary := slices.Max(s.labelSetIDs) + 1

	// Act
	result := s.rotate(shrinkBoundary, s.lss, newLSS)
	lsIDExisting := s.lss.FindOrEmplace(s.labelSets[0]).LabelSetID

	// Assert
	s.Equal(s.labelSetIDs[0], lsIDExisting)

	selector := s.mustQuerySelector(s.matchers)
	snapshot := s.lss.CreateLabelSetSnapshot()
	s.Equal(labelSetToCppBridgeLabels(s.labelSets[:3]), s.convertQueryResultToLabelSets(snapshot, selector))

	runtime.KeepAlive(snapshot)
	runtime.KeepAlive(result)
}

func (s *RotateLSSSuite) TestCopyOnRotatePart() {
	// Arrange
	lname := "foo"
	rLSS := s.makeRotatedLSS(lname)
	newLSS := cppbridge.NewQueryableLssStorage()
	shrinkBoundary := slices.Max(rLSS.oldLabelSetIDs) + 1

	// Act
	result := s.rotate(shrinkBoundary, rLSS.oldLSS, newLSS)

	// Assert
	expectedLabelSets := labelSetToCppBridgeLabels(rLSS.expectedLSes)
	s.Equal(expectedLabelSets, newLSS.GetLabelSets(rLSS.newLabelSetIDs).LabelsSets())
	s.Equal(expectedLabelSets, rLSS.oldLSS.GetLabelSets(rLSS.oldLabelSetIDs).LabelsSets())

	// QueryLabelNames
	oldNames := rLSS.oldLSS.QueryLabelNames([]model.LabelMatcher{})
	s.Equal(rLSS.oldNames, oldNames.Names())

	newNames := newLSS.QueryLabelNames([]model.LabelMatcher{})
	s.Equal(rLSS.newNames, newNames.Names())

	// QueryLabelValues
	oldValues := rLSS.oldLSS.QueryLabelValues("foo", []model.LabelMatcher{})
	s.Equal(rLSS.oldValues, oldValues.Values())

	newValues := newLSS.QueryLabelValues("foo", []model.LabelMatcher{})
	s.Equal(rLSS.newValues, newValues.Values())

	selector := s.mustQuerySelector(s.matchers)
	snapshot := s.lss.CreateLabelSetSnapshot()
	s.Equal(labelSetToCppBridgeLabels(s.labelSets[:3]), s.convertQueryResultToLabelSets(snapshot, selector))

	runtime.KeepAlive(snapshot)
	runtime.KeepAlive(result)
}

func (s *RotateLSSSuite) TestCopyOnRotateEmplaceNewLSPart() {
	// Arrange
	rLSS := s.makeRotatedLSS("")
	newLSS := cppbridge.NewQueryableLssStorage()
	shrinkBoundary := slices.Max(rLSS.oldLabelSetIDs) + 1

	// Act
	result := s.rotate(shrinkBoundary, rLSS.oldLSS, newLSS)
	lsNew := model.LabelSetFromPairs("__name__", "kek1")
	lsIDNew := rLSS.oldLSS.FindOrEmplace(lsNew).LabelSetID

	// Assert
	// !!!ATTENTION!!! When copying the added series, the order in which the series are added is preserved.
	expectedLabelSets := labelSetToCppBridgeLabels(rLSS.expectedLSes)
	s.Equal(expectedLabelSets, newLSS.GetLabelSets(rLSS.newLabelSetIDs).LabelsSets())
	s.Equal(expectedLabelSets, rLSS.oldLSS.GetLabelSets(rLSS.oldLabelSetIDs).LabelsSets())
	s.Equal(slices.Max(s.labelSetIDs)+1, lsIDNew)
	s.Equal(
		labelSetToCppBridgeLabels(append(rLSS.expectedLSes, lsNew)),
		rLSS.oldLSS.GetLabelSets(append(rLSS.oldLabelSetIDs, lsIDNew)).LabelsSets(),
	)

	selector := s.mustQuerySelector(s.matchers)
	snapshot := s.lss.CreateLabelSetSnapshot()
	s.Equal(labelSetToCppBridgeLabels(s.labelSets[:3]), s.convertQueryResultToLabelSets(snapshot, selector))

	runtime.KeepAlive(snapshot)
	runtime.KeepAlive(result)
}

func (s *RotateLSSSuite) TestCopyOnRotateEmplaceExistingLSPart() {
	// Arrange
	rLSS := s.makeRotatedLSS("")
	newLSS := cppbridge.NewQueryableLssStorage()
	shrinkBoundary := slices.Max(rLSS.oldLabelSetIDs) + 1

	// Act
	result := s.rotate(shrinkBoundary, rLSS.oldLSS, newLSS)
	lsIDExisting := rLSS.oldLSS.FindOrEmplace(rLSS.expectedLSes[0]).LabelSetID

	// Assert
	// !!!ATTENTION!!! When copying the added series, the order in which the series are added is preserved.
	expectedLabelSets := labelSetToCppBridgeLabels(rLSS.expectedLSes)
	s.Equal(expectedLabelSets, newLSS.GetLabelSets(rLSS.newLabelSetIDs).LabelsSets())
	s.Equal(expectedLabelSets, rLSS.oldLSS.GetLabelSets(rLSS.oldLabelSetIDs).LabelsSets())
	s.Equal(rLSS.oldLabelSetIDs[0], lsIDExisting)

	selector := s.mustQuerySelector(s.matchers)
	snapshot := s.lss.CreateLabelSetSnapshot()
	s.Equal(labelSetToCppBridgeLabels(s.labelSets[:3]), s.convertQueryResultToLabelSets(snapshot, selector))

	runtime.KeepAlive(snapshot)
	runtime.KeepAlive(result)
}

func (s *RotateLSSSuite) TestCopyOnRotateEmplaceAfterBoundaryLS() {
	// Arrange
	rLSS := s.makeRotatedLSS("")
	newLSS := cppbridge.NewQueryableLssStorage()
	shrinkBoundary := slices.Max(rLSS.oldLabelSetIDs) + 1

	// Act
	result := s.rotate(shrinkBoundary, rLSS.oldLSS, newLSS)
	// the new ls ID will be returned because the labelset(6 - even-numbered) is copied to the oldlass,
	// but the bit is not set in the bitset, and after rotation, the labelset will be discarded
	lsIDNew := rLSS.oldLSS.FindOrEmplace(s.labelSets[len(s.labelSets)-1]).LabelSetID

	// Assert
	// !!!ATTENTION!!! When copying the added series, the order in which the series are added is preserved.
	expectedLabelSets := labelSetToCppBridgeLabels(rLSS.expectedLSes)
	s.Equal(expectedLabelSets, newLSS.GetLabelSets(rLSS.newLabelSetIDs).LabelsSets())
	s.Equal(expectedLabelSets, rLSS.oldLSS.GetLabelSets(rLSS.oldLabelSetIDs).LabelsSets())
	s.Equal(s.labelSetIDs[len(s.labelSetIDs)-1]+1, lsIDNew)

	selector := s.mustQuerySelector(s.matchers)
	snapshot := s.lss.CreateLabelSetSnapshot()
	s.Equal(labelSetToCppBridgeLabels(s.labelSets[:3]), s.convertQueryResultToLabelSets(snapshot, selector))

	runtime.KeepAlive(snapshot)
	runtime.KeepAlive(result)
}

func (s *RotateLSSSuite) TestCopyOnRotateShrinkAndEmplacePart() {
	// Arrange
	rLSS := s.makeRotatedLSS("")
	newLSS := cppbridge.NewQueryableLssStorage()
	shrinkBoundary := slices.Max(rLSS.oldLabelSetIDs) - 1

	// Act
	snapshot := rLSS.oldLSS.CreateLabelSetSnapshot()
	rLSS.oldLSS.SetPendingShrinkBoundary(shrinkBoundary)
	dstSrcLsIdsMapping := snapshot.CopyAddedSeries(rLSS.oldLSS.BitsetSeries(), newLSS)
	mappedSnapshot := newLSS.CreateLabelSetSnapshot()

	// add ls before finalize copy and shrink
	lsNew1 := model.LabelSetFromPairs("__name__", "kek1")
	lsIDNew1 := rLSS.oldLSS.FindOrEmplace(lsNew1).LabelSetID

	rLSS.oldLSS.FinalizeCopyAndShrink(mappedSnapshot, dstSrcLsIdsMapping)

	lsNew2 := model.LabelSetFromPairs("__name__", "kek2")
	lsIDNew2 := rLSS.oldLSS.FindOrEmplace(lsNew2).LabelSetID
	lsIDExisting := rLSS.oldLSS.FindOrEmplace(rLSS.expectedLSes[0]).LabelSetID

	// Assert
	// !!!ATTENTION!!! When copying the added series, the order in which the series are added is preserved.
	expectedLabelSets := labelSetToCppBridgeLabels(rLSS.expectedLSes)
	s.Equal(expectedLabelSets, newLSS.GetLabelSets(rLSS.newLabelSetIDs).LabelsSets())
	s.Equal(expectedLabelSets, rLSS.oldLSS.GetLabelSets(rLSS.oldLabelSetIDs).LabelsSets())
	s.Equal(rLSS.oldLabelSetIDs[0], lsIDExisting)
	s.Equal(slices.Max(s.labelSetIDs)+1, lsIDNew1)
	s.Equal(slices.Max(s.labelSetIDs)+2, lsIDNew2)
	s.Equal(
		labelSetToCppBridgeLabels(append(rLSS.expectedLSes, lsNew1, lsNew2)),
		rLSS.oldLSS.GetLabelSets(append(rLSS.oldLabelSetIDs, lsIDNew1, lsIDNew2)).LabelsSets(),
	)

	selector := s.mustQuerySelector(s.matchers)
	snapshot = s.lss.CreateLabelSetSnapshot()
	s.Equal(labelSetToCppBridgeLabels(s.labelSets[:3]), s.convertQueryResultToLabelSets(snapshot, selector))

	runtime.KeepAlive(snapshot)
	runtime.KeepAlive(dstSrcLsIdsMapping)
	runtime.KeepAlive(mappedSnapshot)
}

// rotateResult is a helper struct for testing rotated LSS.
// It contains the dstSrcLsIdsMapping and the mappedSnapshot.
type rotateResult struct {
	dstSrcLsIdsMapping *cppbridge.IdsMapping
	mappedSnapshot     *cppbridge.LabelSetSnapshot
}

// rotate performs a rotate operation on the old LSS and returns a rotateResult.
func (s *RotateLSSSuite) rotate(shrinkBoundary uint32, oldLSS, newLSS *cppbridge.LabelSetStorage) *rotateResult {
	s.T().Log("Rotate LSS")
	snapshot := oldLSS.CreateLabelSetSnapshot()
	oldLSS.SetPendingShrinkBoundary(shrinkBoundary)
	dstSrcLsIdsMapping := snapshot.CopyAddedSeries(oldLSS.BitsetSeries(), newLSS)
	mappedSnapshot := newLSS.CreateLabelSetSnapshot()
	oldLSS.FinalizeCopyAndShrink(mappedSnapshot, dstSrcLsIdsMapping)

	return &rotateResult{
		dstSrcLsIdsMapping: dstSrcLsIdsMapping,
		mappedSnapshot:     mappedSnapshot,
	}
}

// rotatedLSS is a helper struct for testing rotated LSS.
// It contains the old LSS, the expected label sets, the old label set IDs,
// the new label set IDs, the old names, the old values, the new names, and the new values.
type rotatedLSS struct {
	oldLSS         *cppbridge.LabelSetStorage
	expectedLSes   []model.LabelSet
	oldLabelSetIDs []uint32
	newLabelSetIDs []uint32
	oldNames       []string
	oldValues      []string
	newNames       []string
	newValues      []string
}

// makeRotatedLSS creates a rotated LSS from the original LSS.
func (s *RotateLSSSuite) makeRotatedLSS(lName string) *rotatedLSS {
	rLSS := &rotatedLSS{
		oldLSS:         cppbridge.NewQueryableLssStorage(),
		expectedLSes:   make([]model.LabelSet, 0, len(s.labelSets)),
		oldLabelSetIDs: make([]uint32, 0, len(s.labelSets)),
		newLabelSetIDs: make([]uint32, 0, len(s.labelSets)),
		oldNames:       make([]string, 0, len(s.labelSets)),
		oldValues:      make([]string, 0, len(s.labelSets)),
		newNames:       make([]string, 0, len(s.labelSets)),
		newValues:      make([]string, 0, len(s.labelSets)),
	}

	bitsetSeries := s.lss.BitsetSeries()
	s.lss.CreateLabelSetSnapshot().CopyAddedSeries(bitsetSeries, rLSS.oldLSS)

	var lsid uint32
	for i, labelSet := range s.labelSets {
		labelSet.Range(func(lname string, lvalue string) bool {
			if !slices.Contains(rLSS.oldNames, lname) {
				rLSS.oldNames = append(rLSS.oldNames, lname)
			}

			if !slices.Contains(rLSS.oldValues, lvalue) && lname == lName {
				rLSS.oldValues = append(rLSS.oldValues, lvalue)
			}

			return true
		})

		if i%2 == 0 {
			continue
		}

		rLSS.oldLabelSetIDs = append(rLSS.oldLabelSetIDs, rLSS.oldLSS.FindOrEmplace(labelSet).LabelSetID)
		rLSS.expectedLSes = append(rLSS.expectedLSes, labelSet)
		rLSS.newLabelSetIDs = append(rLSS.newLabelSetIDs, lsid)
		labelSet.Range(func(lname string, lvalue string) bool {
			if !slices.Contains(rLSS.newNames, lname) {
				rLSS.newNames = append(rLSS.newNames, lname)
			}

			if !slices.Contains(rLSS.newValues, lvalue) && lname == lName {
				rLSS.newValues = append(rLSS.newValues, lvalue)
			}

			return true
		})

		lsid++
	}

	slices.Sort(rLSS.oldNames)
	slices.Sort(rLSS.oldValues)

	slices.Sort(rLSS.newNames)
	slices.Sort(rLSS.newValues)

	return rLSS
}

// convertQueryResultToLabelSets converts the query result to slice of [cppbridge.Labels].
func (s *RotateLSSSuite) convertQueryResultToLabelSets(
	snapshot *cppbridge.LabelSetSnapshot,
	selector uintptr,
) []cppbridge.Labels {
	res := snapshot.Query(selector)
	s.Require().Equal(cppbridge.LSSQueryStatusMatch, res.Status())

	actualLabelSets := make([]cppbridge.Labels, 0, len(res.IDs()))
	for _, lsid := range res.IDs() {

		ls := cppbridge.Labels{}
		snapshot.RangeLabelSet(lsid, func(l cppbridge.Label) error {
			ls = append(ls, cppbridge.Label{
				Name:  unique.Make(l.Name).Value(),
				Value: unique.Make(l.Value).Value(),
			})
			return nil
		})
		actualLabelSets = append(actualLabelSets, ls)
	}

	runtime.KeepAlive(res)

	return actualLabelSets
}

func (s *RotateLSSSuite) mustQuerySelector(matchers []model.LabelMatcher) uintptr {
	s.T().Log("Checking query on snapshot")
	selector, status := s.lss.QuerySelector(matchers)
	s.Require().Equal(cppbridge.LSSQueryStatusMatch, status)

	return selector
}
