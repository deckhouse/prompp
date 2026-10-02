package catalog

import (
	"math"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
)

type RecordFieldsSuite struct {
	suite.Suite
}

func TestRecordFieldsSuite(t *testing.T) {
	suite.Run(t, new(RecordFieldsSuite))
}

func (*RecordFieldsSuite) newRecord() *SerializedRecord {
	return &SerializedRecord{
		id:                    uuid.MustParse("01929a6c-7b1e-7000-8000-000000000001"),
		numberOfShards:        4,
		createdAt:             100,
		updatedAt:             200,
		corrupted:             false,
		status:                StatusActive,
		numberOfSegments:      10,
		lastAppendedSegmentID: lastAppendedSegmentIDByNumberOfSegments(10),
		mint:                  math.MaxInt64,
		maxt:                  math.MinInt64,
	}
}

func (s *RecordFieldsSuite) TestFullFieldsNewRecord() {
	sr := s.newRecord()

	fields := fullFields(sr)

	s.Equal(fieldsAll&^(fieldDeletedAt|fieldMinT|fieldMaxT), fields)
}

func (s *RecordFieldsSuite) TestFullFieldsDeletedRecordWithTimeBounds() {
	sr := s.newRecord()
	sr.deletedAt = 300
	sr.mint = 10
	sr.maxt = 20

	fields := fullFields(sr)

	s.Equal(fieldsAll, fields)
}

func (s *RecordFieldsSuite) TestDiffFieldsEqualRecords() {
	sr := s.newRecord()

	fields := diffFields(sr, createSerializedRecordCopy(sr))

	s.Equal(fieldMask(0), fields)
}

func (s *RecordFieldsSuite) TestDiffFieldsByField() {
	testCases := []struct {
		name     string
		change   func(sr *SerializedRecord)
		expected fieldMask
	}{
		{"number of shards", func(sr *SerializedRecord) { sr.numberOfShards = 8 }, fieldNumberOfShards},
		{"created at", func(sr *SerializedRecord) { sr.createdAt = 101 }, fieldCreatedAt},
		{"updated at", func(sr *SerializedRecord) { sr.updatedAt = 201 }, fieldUpdatedAt},
		{"deleted at", func(sr *SerializedRecord) { sr.deletedAt = 300 }, fieldDeletedAt},
		{"corrupted", func(sr *SerializedRecord) { sr.corrupted = true }, fieldCorrupted},
		{"status", func(sr *SerializedRecord) { sr.status = StatusRotated }, fieldStatus},
		{"mint", func(sr *SerializedRecord) { sr.mint = 10 }, fieldMinT},
		{"maxt", func(sr *SerializedRecord) { sr.maxt = 20 }, fieldMaxT},
		{"segments count is not compared", func(sr *SerializedRecord) { sr.numberOfSegments = 11 }, 0},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			old := s.newRecord()
			changed := createSerializedRecordCopy(old)
			tc.change(changed)

			fields := diffFields(old, changed)

			s.Equal(tc.expected, fields)
		})
	}
}

func (s *RecordFieldsSuite) TestApplyRecordChangesOnlyMaskedFields() {
	sr := s.newRecord()
	changed := createSerializedRecordCopy(sr)
	changed.status = StatusRotated
	changed.updatedAt = 300
	changed.corrupted = true
	changed.numberOfSegments = 20

	applyRecordChanges(sr, changed, fieldStatus|fieldUpdatedAt)

	expected := s.newRecord()
	expected.status = StatusRotated
	expected.updatedAt = 300
	s.Equal(expected, sr)
}

func (s *RecordFieldsSuite) TestApplyRecordChangesSegmentsCount() {
	sr := s.newRecord()
	changed := createSerializedRecordCopy(sr)
	changed.numberOfSegments = 20

	applyRecordChanges(sr, changed, fieldSegmentsCount)

	s.Equal(uint32(20), sr.numberOfSegments)
	s.Require().NotNil(sr.lastAppendedSegmentID.RawValue())
	s.Equal(uint32(19), sr.lastAppendedSegmentID.Value())
}

func (s *RecordFieldsSuite) TestApplyRecordChangesZeroSegmentsCount() {
	sr := s.newRecord()
	changed := createSerializedRecordCopy(sr)
	changed.numberOfSegments = 0

	applyRecordChanges(sr, changed, fieldSegmentsCount)

	s.Equal(uint32(0), sr.numberOfSegments)
	s.True(sr.lastAppendedSegmentID.IsNil())
}
