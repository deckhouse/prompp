package cppbridge_test

import (
	"sync"
	"testing"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/stretchr/testify/suite"
)

type LabelSetSnapshotSuite struct {
	suite.Suite

	lsses []*cppbridge.LabelSetStorage
	mls   model.LabelSet
	lsids []uint32
}

func TestLabelSetSnapshotSuite(t *testing.T) {
	suite.Run(t, new(LabelSetSnapshotSuite))
}

func (s *LabelSetSnapshotSuite) SetupTest() {
	s.lsses = []*cppbridge.LabelSetStorage{
		cppbridge.NewLssStorage(),
		cppbridge.NewQueryableLssStorage(),
	}
	s.mls = model.LabelSetFromMap(map[string]string{
		"__name__": "somename",
		"job":      "somejob",
	})
	s.lsids = make([]uint32, 0, len(s.lsses))
	for _, lss := range s.lsses {
		s.lsids = append(s.lsids, lss.FindOrEmplace(s.mls).LabelSetID)
	}
}

type bytesTestCase struct {
	labelSet model.LabelSet
	names    []string
	expected []byte
}

func (s *LabelSetSnapshotSuite) TestBytes() {
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
	for _, lss := range s.lsses {
		for _, testCase := range testCases {
			s.testBytesImpl(lss, &testCase, bytes)
		}
	}
}

func (s *LabelSetSnapshotSuite) testBytesImpl(
	lss *cppbridge.LabelSetStorage,
	testCase *bytesTestCase,
	bytes []byte,
) {
	// Arrange
	lsid := lss.FindOrEmplace(testCase.labelSet).LabelSetID
	snapshot := lss.CreateLabelSetSnapshot(&testSnapshotSource{})

	// Act
	bytes = snapshot.LabelSetBytes(lsid, bytes, false)

	// Assert
	s.Equal(testCase.expected, bytes)
}

func (s *LabelSetSnapshotSuite) TestBytesWithLabels() {
	testCases := []bytesTestCase{
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
	for _, lss := range s.lsses {
		for _, testCase := range testCases {
			s.testBytesWithLabelsImpl(lss, &testCase, bytes)
		}
	}
}

func (s *LabelSetSnapshotSuite) testBytesWithLabelsImpl(
	lss *cppbridge.LabelSetStorage,
	testCase *bytesTestCase,
	bytes []byte,
) {
	// Arrange
	lsid := lss.FindOrEmplace(testCase.labelSet).LabelSetID
	snapshot := lss.CreateLabelSetSnapshot(&testSnapshotSource{})

	// Act
	bytes = snapshot.LabelSetBytesWithLabels(lsid, bytes, false, testCase.names)

	// Assert
	s.Equal(testCase.expected, bytes)
}

func (s *LabelSetSnapshotSuite) TestBytesWithoutLabels() {
	testCases := []bytesTestCase{
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
	for _, lss := range s.lsses {
		for _, testCase := range testCases {
			s.testBytesWithoutLabelsImpl(lss, &testCase, bytes)
		}
	}
}

func (s *LabelSetSnapshotSuite) testBytesWithoutLabelsImpl(
	lss *cppbridge.LabelSetStorage,
	testCase *bytesTestCase,
	bytes []byte,
) {
	// Arrange
	lsid := lss.FindOrEmplace(testCase.labelSet).LabelSetID
	snapshot := lss.CreateLabelSetSnapshot(&testSnapshotSource{})

	// Act
	bytes = snapshot.LabelSetBytesWithoutLabels(lsid, bytes, false, testCase.names)

	// Assert
	s.Equal(testCase.expected, bytes)
}

func (s *LabelSetSnapshotSuite) TestLabelSetGetValue() {
	for i, lss := range s.lsses {
		// Arrange
		mls := s.mls.With("test", s.T().Name())

		lsid := lss.FindOrEmplace(mls).LabelSetID
		snapshot := lss.CreateLabelSetSnapshot(&testSnapshotSource{})

		// Act
		nameT := snapshot.LabelSetGetValue(lsid, "test")
		nameF := snapshot.LabelSetGetValue(s.lsids[i], "test")

		// Assert
		s.Equal(s.T().Name(), nameT)
		s.Empty(nameF)
	}
}

func (s *LabelSetSnapshotSuite) TestLabelSetHasDuplicateLabelNames() {
	for i, lss := range s.lsses {
		// Arrange
		mlsDuplicate := model.LabelSetFromPairs(
			"__name__", "somename",
			"__name__", s.T().Name(),
			"job", "somejob",
		)

		lsid := lss.FindOrEmplace(mlsDuplicate).LabelSetID
		snapshot := lss.CreateLabelSetSnapshot(&testSnapshotSource{})

		// Act
		nameF, okF := snapshot.LabelSetHasDuplicateLabelNames(s.lsids[i], false)
		nameT, okT := snapshot.LabelSetHasDuplicateLabelNames(lsid, false)
		nameD, okD := snapshot.LabelSetHasDuplicateLabelNames(lsid, true)

		// Assert
		s.False(okF)
		s.Empty(nameF)
		s.True(okT)
		s.Equal("__name__", nameT)
		s.False(okD)
		s.Empty(nameD)
	}
}

func (s *LabelSetSnapshotSuite) TestLabelSetHasLabelName() {
	for _, lss := range s.lsses {
		// Arrange
		mls := s.mls.With("test", s.T().Name())

		lsid := lss.FindOrEmplace(mls).LabelSetID
		snapshot := lss.CreateLabelSetSnapshot(&testSnapshotSource{})

		// Act
		okT := snapshot.LabelSetHasLabelName(lsid, "test")
		okF := snapshot.LabelSetHasLabelName(lsid, "test1")

		// Assert
		s.True(okT)
		s.False(okF)
	}
}

func (s *LabelSetSnapshotSuite) TestLabelSetHashForLabels() {
	for i, lss := range s.lsses {
		// Arrange
		mls := s.mls.With("test", s.T().Name())

		lsid := lss.FindOrEmplace(mls).LabelSetID
		snapshot := lss.CreateLabelSetSnapshot(&testSnapshotSource{})

		// Act
		hashExpected := snapshot.LabelSetHash(s.lsids[i], false)
		hashActual := snapshot.LabelSetHashForLabels(lsid, []string{"__name__", "job"}, false)

		// Assert
		s.Equal(hashExpected, hashActual)
	}
}

func (s *LabelSetSnapshotSuite) TestLabelSetHashForLabelsDrop() {
	for i, lss := range s.lsses {
		// Arrange
		mls := s.mls.With("test", s.T().Name())

		lsid := lss.FindOrEmplace(mls).LabelSetID
		snapshot := lss.CreateLabelSetSnapshot(&testSnapshotSource{})

		// Act
		hashExpected := snapshot.LabelSetHash(s.lsids[i], true)
		hashActual := snapshot.LabelSetHashForLabels(lsid, []string{"__name__", "job"}, true)

		// Assert
		s.Equal(hashExpected, hashActual)
	}
}

func (s *LabelSetSnapshotSuite) TestLabelSetHashWithoutLabels() {
	for i, lss := range s.lsses {
		// Arrange
		mls := s.mls.With("test", s.T().Name())

		lsid := lss.FindOrEmplace(mls).LabelSetID
		snapshot := lss.CreateLabelSetSnapshot(&testSnapshotSource{})

		// Act
		hashExpected := snapshot.LabelSetHash(s.lsids[i], true)
		hashActual := snapshot.LabelSetHashWithoutLabels(lsid, []string{"test"})

		// Assert
		s.Equal(hashExpected, hashActual)
	}
}

func (s *LabelSetSnapshotSuite) TestLabelSetLength() {
	for _, lss := range s.lsses {
		// Arrange
		mls := s.mls.With("test", s.T().Name())

		lsid := lss.FindOrEmplace(mls).LabelSetID
		snapshot := lss.CreateLabelSetSnapshot(&testSnapshotSource{})

		// Act
		length := snapshot.LabelSetLength(lsid, false)
		lengthDrop := snapshot.LabelSetLength(lsid, true)

		// Assert
		s.Equal(mls.Len(), length)
		s.Equal(mls.Len()-1, lengthDrop)
	}
}

func (s *LabelSetSnapshotSuite) TestRangeLabelSet() {
	for _, lss := range s.lsses {
		// Arrange
		mls := s.mls.With("test", s.T().Name())
		builder := model.NewLabelSetSimpleBuilderSize(mls.Len())
		builderDrop := model.NewLabelSetSimpleBuilderSize(mls.Len() - 1)

		// Act
		lsid := lss.FindOrEmplace(mls).LabelSetID
		snapshot := lss.CreateLabelSetSnapshot(&testSnapshotSource{})

		snapshot.RangeLabelSet(lsid, false, func(l cppbridge.Label) error {
			builder.Add(l.Name, l.Value)
			return nil
		})
		snapshot.RangeLabelSet(lsid, true, func(l cppbridge.Label) error {
			builderDrop.Add(l.Name, l.Value)
			return nil
		})

		// Assert
		s.Equal(mls.String(), builder.Build().String())
		s.Equal(mls.Without("__name__").String(), builderDrop.Build().String())
	}
}

func (s *LabelSetSnapshotSuite) TestLabelSetFromBuilderHash() {
	for _, lss := range s.lsses {
		// Arrange
		mls := s.mls.With("test", s.T().Name())

		lsid := lss.FindOrEmplace(mls).LabelSetID
		snapshot := lss.CreateLabelSetSnapshot(&testSnapshotSource{})

		// Act
		hashExpected := snapshot.LabelSetHash(lsid, false)
		hashActual, empty := cppbridge.LabelSetFromBuilderHash(
			nil,
			nil,
			snapshot,
			lsid,
		)

		// Assert
		s.False(empty)
		s.Equal(hashExpected, hashActual)
	}
}

func (s *LabelSetSnapshotSuite) TestLabelSetFromBuilderHashDel() {
	for i, lss := range s.lsses {
		// Arrange
		mls := s.mls.With("test", s.T().Name())

		lsid := lss.FindOrEmplace(mls).LabelSetID
		snapshot := lss.CreateLabelSetSnapshot(&testSnapshotSource{})

		// Act
		hashExpected := snapshot.LabelSetHash(s.lsids[i], false)
		hashActual, empty := cppbridge.LabelSetFromBuilderHash(
			nil,
			[]string{"test"},
			snapshot,
			lsid,
		)

		// Assert
		s.False(empty)
		s.Equal(hashExpected, hashActual)
	}
}

func (s *LabelSetSnapshotSuite) TestLabelSetFromBuilderHashAdd() {
	for i, lss := range s.lsses {
		// Arrange
		mls := s.mls.With("test", s.T().Name())

		lsid := lss.FindOrEmplace(mls).LabelSetID
		snapshot := lss.CreateLabelSetSnapshot(&testSnapshotSource{})

		// Act
		hashExpected := snapshot.LabelSetHash(lsid, false)
		hashActual, empty := cppbridge.LabelSetFromBuilderHash(
			[]cppbridge.Label{{Name: "test", Value: s.T().Name()}},
			nil,
			snapshot,
			s.lsids[i],
		)

		// Assert
		s.False(empty)
		s.Equal(hashExpected, hashActual)
	}
}

func (s *LabelSetSnapshotSuite) TestEqualWithBuilderTrue() {
	for i, lss := range s.lsses {
		// Arrange
		mls := s.mls.With("test", s.T().Name())

		lsid := lss.FindOrEmplace(mls).LabelSetID
		snapshot := lss.CreateLabelSetSnapshot(&testSnapshotSource{})

		// Act
		eq := snapshot.LabelSetEqualWithBuilder(
			[]cppbridge.Label{{Name: "test", Value: s.T().Name()}},
			nil,
			snapshot,
			s.lsids[i],
			lsid,
		)

		// Assert
		s.True(eq)
	}
}

func (s *LabelSetSnapshotSuite) TestEqualWithBuilderTrue_Builder() {
	builderLSS := cppbridge.NewLssStorage()
	builderLSID := builderLSS.FindOrEmplace(s.mls.With("test", s.T().Name())).LabelSetID
	builderSnapshot := builderLSS.CreateLabelSetSnapshot(&testSnapshotSource{})

	for i, lss := range s.lsses {
		// Arrange
		snapshot := lss.CreateLabelSetSnapshot(&testSnapshotSource{})

		// Act
		eq := snapshot.LabelSetEqualWithBuilder(
			nil,
			[]string{"test"},
			builderSnapshot,
			builderLSID,
			s.lsids[i],
		)

		// Assert
		s.True(eq)
	}
}

func (s *LabelSetSnapshotSuite) TestEqualWithBuilderTrue_Builder_Len() {
	builderLSS := cppbridge.NewLssStorage()
	builderLSID := builderLSS.FindOrEmplace(s.mls.With("test", s.T().Name())).LabelSetID
	builderSnapshot := builderLSS.CreateLabelSetSnapshot(&testSnapshotSource{})

	for _, lss := range s.lsses {
		// Arrange
		mls := s.mls.With("test", s.T().Name())

		lsid := lss.FindOrEmplace(mls).LabelSetID
		snapshot := lss.CreateLabelSetSnapshot(&testSnapshotSource{})

		// Act
		eq := snapshot.LabelSetEqualWithBuilder(
			nil,
			nil,
			builderSnapshot,
			builderLSID,
			lsid,
		)

		// Assert
		s.True(eq)
	}
}

func (s *LabelSetSnapshotSuite) TestEqualWithBuilderFalse_Builder() {
	builderLSS := cppbridge.NewLssStorage()
	builderLSID := builderLSS.FindOrEmplace(s.mls.With("test", s.T().Name())).LabelSetID
	builderSnapshot := builderLSS.CreateLabelSetSnapshot(&testSnapshotSource{})

	for _, lss := range s.lsses {
		// Arrange
		mls := s.mls.With("test1", s.T().Name())

		lsid := lss.FindOrEmplace(mls).LabelSetID
		snapshot := lss.CreateLabelSetSnapshot(&testSnapshotSource{})

		// Act
		eq := snapshot.LabelSetEqualWithBuilder(
			[]cppbridge.Label{{Name: "test", Value: s.T().Name()}},
			nil,
			builderSnapshot,
			builderLSID,
			lsid,
		)

		// Assert
		s.False(eq)
	}
}

//
// LSSWithSnapshotSuite
//

type LSSWithSnapshotSuite struct {
	suite.Suite

	lsses []*cppbridge.LSSWithSnapshot
}

func TestLSSWithSnapshotSuite(t *testing.T) {
	suite.Run(t, new(LSSWithSnapshotSuite))
}

func (s *LSSWithSnapshotSuite) SetupTest() {
	s.lsses = []*cppbridge.LSSWithSnapshot{
		cppbridge.NewLSSWithSnapshot(cppbridge.NewLssStorage()),
		cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage()),
	}

	labelSets := []model.LabelSet{
		model.LabelSetFromMap(map[string]string{"lol": "kek"}),
		model.LabelSetFromMap(map[string]string{"lol": "kek", "che": "bureck"}),
		model.LabelSetFromMap(map[string]string{"lol": "kek", "zhe": "bureck"}),
		model.LabelSetFromMap(map[string]string{"foo": "bar"}),
		model.LabelSetFromMap(map[string]string{"foo": "baz"}),
	}

	for _, lss := range s.lsses {
		for _, labelSet := range labelSets {
			lss.FindOrEmplace(labelSet)
		}
	}
}

func (s *LSSWithSnapshotSuite) TestFindOrEmplaceFromBuilderWithExistingLabelSet() {
	for _, lss := range s.lsses {
		// Arrange
		labelSetSnapshot := lss.Snapshot()
		sortedAdd := []cppbridge.Label{{Name: "che", Value: "bureck"}}
		lsidAdd := uint32(0)
		hashAdd, _ := cppbridge.LabelSetFromBuilderHash(sortedAdd, nil, labelSetSnapshot, lsidAdd)

		sortedDel := []string{"che"}
		lsidDel := uint32(1)
		hashDel, _ := cppbridge.LabelSetFromBuilderHash(nil, sortedDel, labelSetSnapshot, lsidDel)

		// Act
		existingLsIdWithAdd, lengthWithAdd := lss.FindOrEmplaceFromBuilder(
			sortedAdd,
			nil,
			labelSetSnapshot,
			hashAdd,
			lsidAdd,
		)

		existingLsIdWithDel, lengthWithDel := lss.FindOrEmplaceFromBuilder(
			nil,
			sortedDel,
			labelSetSnapshot,
			hashDel,
			lsidDel,
		)

		// Assert
		s.Equal(uint16(2), lengthWithAdd)
		s.Equal(uint16(1), lengthWithDel)
		s.Equal(uint32(1), existingLsIdWithAdd)
		s.Equal(uint32(0), existingLsIdWithDel)
	}
}

func (s *LSSWithSnapshotSuite) TestFindOrEmplaceFromBuilderWithNewLabelSet() {
	for _, lss := range s.lsses {
		// Arrange
		lsid := uint32(0)
		labelSetSnapshot := lss.Snapshot()
		sortedAdd := []cppbridge.Label{{Name: "new_lol", Value: "new_kek"}}
		hash, _ := cppbridge.LabelSetFromBuilderHash(sortedAdd, nil, labelSetSnapshot, lsid)

		// Act
		_, expectedLsId, _ := lss.Stats()
		existingLsId, length := lss.FindOrEmplaceFromBuilder(
			sortedAdd,
			nil,
			labelSetSnapshot,
			hash,
			lsid,
		)

		// Assert
		s.Equal(uint16(2), length)
		s.Equal(uint32(expectedLsId), existingLsId)
	}
}

func (s *LSSWithSnapshotSuite) TestFindOrEmplaceFromBuilderWithNewLabelSetAnother() {
	for _, lss := range s.lsses {
		// Arrange
		lssAnother := cppbridge.NewLSSWithSnapshot(cppbridge.NewQueryableLssStorage())
		lsid := lssAnother.FindOrEmplace(model.LabelSetFromMap(map[string]string{"lol": "kek"}))
		labelSetSnapshot := lssAnother.Snapshot()
		sortedAdd := []cppbridge.Label{{Name: "new_lol", Value: "new_kek"}}
		hash, _ := cppbridge.LabelSetFromBuilderHash(sortedAdd, nil, labelSetSnapshot, lsid)

		// Act
		_, expectedLsId, _ := lss.Stats()
		existingLsId, length := lss.FindOrEmplaceFromBuilder(
			sortedAdd,
			nil,
			labelSetSnapshot,
			hash,
			lsid,
		)

		// Assert
		s.Equal(uint16(2), length)
		s.Equal(uint32(expectedLsId), existingLsId)
	}
}

func (s *LSSWithSnapshotSuite) TestStats() {
	for _, lss := range s.lsses {
		// Arrange
		labelSetSnapshot := lss.Snapshot()
		lsid := uint32(0)

		// Act
		sortedAdd := []cppbridge.Label{{Name: "new_lol_0", Value: "new_kek_0"}}
		hash, _ := cppbridge.LabelSetFromBuilderHash(sortedAdd, nil, labelSetSnapshot, lsid)
		_, bitsetSizeBegin := lss.StatsWithReset()
		_, _ = lss.FindOrEmplaceFromBuilder(
			sortedAdd,
			nil,
			labelSetSnapshot,
			hash,
			lsid,
		)
		_, bitsetSizeSingle := lss.StatsWithReset()

		sortedAdd = []cppbridge.Label{{Name: "new_lol_1", Value: "new_kek_1"}}
		hash, _ = cppbridge.LabelSetFromBuilderHash(sortedAdd, nil, labelSetSnapshot, lsid)
		_, _ = lss.FindOrEmplaceFromBuilder(
			sortedAdd,
			nil,
			labelSetSnapshot,
			hash,
			lsid,
		)

		sortedAdd = []cppbridge.Label{{Name: "new_lol_0", Value: "new_kek_0"}}
		hash, _ = cppbridge.LabelSetFromBuilderHash(sortedAdd, nil, labelSetSnapshot, lsid)
		_, _ = lss.FindOrEmplaceFromBuilder(
			sortedAdd,
			nil,
			labelSetSnapshot,
			hash,
			lsid,
		)
		_, bitsetSizeDouble := lss.StatsWithReset()

		_, bitsetSizeEnd := lss.StatsWithReset()

		// Assert
		s.Equal(uint32(0), bitsetSizeBegin)
		s.Equal(uint32(1), bitsetSizeSingle)
		s.Equal(uint32(2), bitsetSizeDouble)
		s.Equal(uint32(0), bitsetSizeEnd)
	}
}

func (s *LabelSetSnapshotSuite) TestLabelSetFromBuilderHash2() {
	for _, lss := range s.lsses {
		// Arrange
		mls := s.mls.With("test", s.T().Name())

		lsid := lss.FindOrEmplace(mls).LabelSetID
		snapshot := lss.CreateLabelSetSnapshot(&testSnapshotSource{})

		// Act
		hashExpected := snapshot.LabelSetHash(lsid, false)
		hashActual, empty := cppbridge.LabelSetFromBuilderHash(
			nil,
			nil,
			snapshot,
			lsid,
		)

		// Assert
		s.False(empty)
		s.Equal(hashExpected, hashActual)
	}
}

func (s *LabelSetSnapshotSuite) TestLabelSetFromBuilderHashEmpty() {
	sortedDel := make([]string, 0, s.mls.Len())
	for lname := range s.mls.Range {
		sortedDel = append(sortedDel, lname)
	}

	for i, lss := range s.lsses {
		// Arrange
		lsid := s.lsids[i]
		snapshot := lss.CreateLabelSetSnapshot(&testSnapshotSource{})

		// Act
		hashActual, empty := cppbridge.LabelSetFromBuilderHash(
			nil,
			sortedDel,
			snapshot,
			lsid,
		)

		// Assert
		s.Empty(hashActual)
		s.True(empty)
	}
}

func BenchmarkLabels_FindOrEmplaceFromBuilder(b *testing.B) {
	lss := cppbridge.NewLSSWithSnapshot(cppbridge.NewLssStorage())
	labelSet := model.LabelSetFromMap(map[string]string{
		"__aaa__":  "11111",
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})
	lsid := lss.FindOrEmplace(labelSet)
	labelSetSnapshot := lss.Snapshot()
	locker := sync.RWMutex{}
	hash, _ := cppbridge.LabelSetFromBuilderHash(nil, nil, lss.Snapshot(), lsid)

	for i := 0; i < b.N; i++ {
		locker.RLock()
		_, _ = lss.FindOrEmplaceFromBuilder(
			nil,
			nil,
			labelSetSnapshot,
			hash,
			lsid,
		)
		locker.RUnlock()
	}
}

func BenchmarkLabels_LoadFromCache(b *testing.B) {
	lss := cppbridge.NewLSSWithSnapshot(cppbridge.NewLssStorage())
	labelSet := model.LabelSetFromMap(map[string]string{
		"__aaa__":  "11111",
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})
	lsid := lss.FindOrEmplace(labelSet)
	hash, _ := cppbridge.LabelSetFromBuilderHash(nil, nil, lss.Snapshot(), lsid)
	lsCache := model.NewCacheWithBitset()
	lsCache.Store(hash, lsid, 5)

	for i := 0; i < b.N; i++ {
		lsID, _, _ := lsCache.Load(hash)

		labelSetSnapshot := lss.Snapshot()

		_ = labelSetSnapshot.LabelSetEqualWithBuilder(
			nil,
			nil,
			labelSetSnapshot,
			lsid,
			lsID,
		)
	}
}

func BenchmarkLabels_LoadFromCache2(b *testing.B) {
	lss := cppbridge.NewLSSWithSnapshot(cppbridge.NewLssStorage())
	labelSet := model.LabelSetFromMap(map[string]string{
		"__aaa__":  "11111",
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})
	lsid := lss.FindOrEmplace(labelSet)
	hash, _ := cppbridge.LabelSetFromBuilderHash(nil, nil, lss.Snapshot(), lsid)
	lsCache := model.NewCacheWithBitset()
	lsCache.Store(hash, lsid, 5)

	for i := 0; i < b.N; i++ {
		lsCache.Store(hash, lsid, 5)
	}
}
