package cppbridge_test

import (
	"math"
	"testing"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/stretchr/testify/suite"
)

type LabelSetSnapshotSuite struct {
	suite.Suite
	lss *cppbridge.LabelSetStorage
}

func TestLabelSetSnapshotSuite(t *testing.T) {
	suite.Run(t, new(LabelSetSnapshotSuite))
}

func (s *LabelSetSnapshotSuite) SetupTest() {
	s.lss = cppbridge.NewQueryableLssStorage()
}

func (s *LabelSetSnapshotSuite) TestLabels() {
	lsMap := map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
	}

	lsIn := model.LabelSetFromMap(lsMap)

	lsID := s.lss.FindOrEmplace(lsIn).LabelSetID
	snapshot := s.lss.CreateLabelSetSnapshot()

	lsLength := 0
	_ = snapshot.RangeLabelSet(lsID, func(l cppbridge.Label) error {
		lv, ok := lsMap[l.Name]
		s.Require().True(ok)
		s.Require().Equal(lv, l.Value)
		lsLength++

		return nil
	})

	s.Equal(lsIn.Len(), lsLength)
}

func (s *LabelSetSnapshotSuite) TestGroupSeriesByLabelNames_ByJob() {
	// Arrange
	idA0 := s.lss.FindOrEmplace(model.NewLabelSetBuilder().Set("__name__", "m").Set("job", "a").Set("instance", "i0").Build()).LabelSetID
	idA1 := s.lss.FindOrEmplace(model.NewLabelSetBuilder().Set("__name__", "m").Set("job", "a").Set("instance", "i1").Build()).LabelSetID
	idB := s.lss.FindOrEmplace(model.NewLabelSetBuilder().Set("__name__", "m").Set("job", "b").Set("instance", "i2").Build()).LabelSetID

	jobId := s.lss.GetLabelNameIDs([]string{"job"})

	snap := s.lss.CreateLabelSetSnapshot()

	// Act
	groupedSeries := snap.GroupSeriesByLabelNames([]uint32{idA0, idA1, idB}, jobId)

	// Assert
	s.Equal([][]uint32{{idA0, idA1}, {idB}}, groupedSeries.Groups)
}

func (s *LabelSetSnapshotSuite) TestGroupSeriesByLabelNames_ByJobAndInstance() {
	// Arrange
	idSame0 := s.lss.FindOrEmplace(model.NewLabelSetBuilder().Set("__name__", "m1").Set("job", "a").Set("instance", "i0").Build()).LabelSetID
	idOther := s.lss.FindOrEmplace(model.NewLabelSetBuilder().Set("__name__", "m2").Set("job", "a").Set("instance", "i1").Build()).LabelSetID
	idSame1 := s.lss.FindOrEmplace(model.NewLabelSetBuilder().Set("__name__", "m3").Set("job", "a").Set("instance", "i0").Build()).LabelSetID

	ids := s.lss.GetLabelNameIDs([]string{"job", "instance"})
	jobID := ids[0]
	instanceID := ids[1]

	snap := s.lss.CreateLabelSetSnapshot()

	// Act
	groupedSeries := snap.GroupSeriesByLabelNames(
		[]uint32{idSame0, idOther, idSame1},
		[]uint32{jobID, instanceID},
	)

	// Assert
	s.Equal([][]uint32{{idSame0, idSame1}, {idOther}}, groupedSeries.Groups)
}

func (s *LabelSetSnapshotSuite) TestGroupSeriesByLabelNames_UnknownLabelNameID() {
	// Arrange
	id0 := s.lss.FindOrEmplace(model.NewLabelSetBuilder().Set("__name__", "m").Set("job", "a").Build()).LabelSetID
	id1 := s.lss.FindOrEmplace(model.NewLabelSetBuilder().Set("__name__", "m").Set("job", "b").Build()).LabelSetID

	snap := s.lss.CreateLabelSetSnapshot()

	// Act
	groupedSeries := snap.GroupSeriesByLabelNames([]uint32{id0, id1}, []uint32{math.MaxUint32})

	// Assert
	s.Equal([][]uint32{{id0, id1}}, groupedSeries.Groups)
}
