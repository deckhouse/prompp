//go:build stringlabels

package cppbridge_test

import (
	"strings"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/stretchr/testify/suite"
)

type LabelSetSnapshotSerializeSuite struct {
	suite.Suite
	lss *cppbridge.LabelSetStorage
}

func TestLabelSetSnapshotSerializeSuite(t *testing.T) {
	suite.Run(t, new(LabelSetSnapshotSerializeSuite))
}

func (s *LabelSetSnapshotSerializeSuite) SetupTest() {
	s.lss = cppbridge.NewQueryableLssStorage()
}

func labelsFromRangeLabelSet(snapshot *cppbridge.LabelSetSnapshot, lsID uint32) labels.Labels {
	builder := labels.NewScratchBuilder(10)
	_ = snapshot.RangeLabelSet(lsID, func(l cppbridge.Label) error {
		builder.Add(l.Name, l.Value)
		return nil
	})
	return builder.Labels()
}

func (s *LabelSetSnapshotSerializeSuite) TestMatchesScratchBuilder() {
	// Arrange
	lsMap := map[string]string{
		"__name__": "metric",
		"job":      "prometheus",
		"instance": "localhost:9090",
	}

	lsID := s.lss.FindOrEmplace(model.LabelSetFromMap(lsMap)).LabelSetID
	snapshot := s.lss.CreateLabelSetSnapshot()

	expected := labelsFromRangeLabelSet(snapshot, lsID)

	// Act
	serialized := snapshot.Serialize(lsID)

	// Assert
	s.Equal(expected.Bytes(nil), []byte(serialized))
}

func (s *LabelSetSnapshotSerializeSuite) TestLongLabelValueUsesMultiByteVarint() {
	// Arrange
	longValue := strings.Repeat("x", 200)
	lsID := s.lss.FindOrEmplace(
		model.NewLabelSetBuilder().Set("__name__", "metric").Set("key", longValue).Build(),
	).LabelSetID
	snapshot := s.lss.CreateLabelSetSnapshot()

	expected := labelsFromRangeLabelSet(snapshot, lsID)

	// Act
	serialized := snapshot.Serialize(lsID)

	// Assert
	s.Equal(expected.Bytes(nil), []byte(serialized))
}

func (s *LabelSetSnapshotSerializeSuite) TestMultipleLabelSets() {
	// Arrange
	lsId1 := s.lss.FindOrEmplace(
		model.NewLabelSetBuilder().Set("__name__", "first").Set("env", "prod").Build(),
	).LabelSetID
	lsId2 := s.lss.FindOrEmplace(
		model.NewLabelSetBuilder().Set("__name__", "second").Set("env", "dev").Build(),
	).LabelSetID
	snapshot := s.lss.CreateLabelSetSnapshot()

	expected1 := labelsFromRangeLabelSet(snapshot, lsId1)
	expected2 := labelsFromRangeLabelSet(snapshot, lsId2)

	// Act
	serialized1 := snapshot.Serialize(lsId1)
	serialized2 := snapshot.Serialize(lsId2)

	// Assert
	s.Equal(expected1.Bytes(nil), []byte(serialized1))
	s.Equal(expected2.Bytes(nil), []byte(serialized2))
}
