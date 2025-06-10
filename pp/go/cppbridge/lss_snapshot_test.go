package cppbridge_test

import (
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
		hashActual := cppbridge.LabelSetFromBuilderHash(
			nil,
			nil,
			snapshot,
			lsid,
		)

		// Assert
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
		hashActual := cppbridge.LabelSetFromBuilderHash(
			nil,
			[]string{"test"},
			snapshot,
			lsid,
		)

		// Assert
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
		hashActual := cppbridge.LabelSetFromBuilderHash(
			[]cppbridge.Label{{Name: "test", Value: s.T().Name()}},
			nil,
			snapshot,
			s.lsids[i],
		)

		// Assert
		s.Equal(hashExpected, hashActual)
	}
}
