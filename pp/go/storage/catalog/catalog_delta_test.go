package catalog

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
)

// deltaTestLog captures the latest write without retaining the caller's record pointer.
type deltaTestLog struct {
	FileLog
	last SerializedRecord
	err  error
}

func (l *deltaTestLog) Write(sr *SerializedRecord) error {
	l.last = *sr

	return l.err
}

func (s *V3FileTestSuite) TestV3Catalog_WritesSmallDeltaWithoutMutatingOnFailure() {
	log := &deltaTestLog{FileLog: *s.fl}
	c, err := New(clockwork.NewFakeClock(), log, DefaultIDGenerator{}, DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)
	record, err := c.Create(4)
	s.Require().NoError(err)
	fullFrame, err := appendFrameV3(nil, &log.last)
	s.Require().NoError(err)
	record.SetNumberOfSegments(42)
	before := record.SerializedRecord
	log.err = errors.New("write failed")

	_, err = c.SetStatus(record.ID(), StatusActive)

	s.Require().ErrorIs(err, log.err)
	s.Equal(before, record.SerializedRecord)
	s.Equal(fieldStatus|fieldUpdatedAt|fieldSegmentsCount, log.last.fields)
	log.err = nil
	_, err = c.SetStatus(record.ID(), StatusActive)
	s.Require().NoError(err)
	deltaFrame, err := appendFrameV3(nil, &log.last)
	s.Require().NoError(err)
	s.Less(len(deltaFrame), len(fullFrame))
	s.Equal(uint32(42), log.last.numberOfSegments)
}

func BenchmarkCatalogStatusDelta(b *testing.B) {
	log := &deltaTestLog{}
	c := &Catalog{clock: clockwork.NewFakeClock(), log: log, records: make(map[string]*Record)}
	record := NewRecordWithData(uuid.New(), 4, 100, 100, 0, false, 0, StatusNew, nil)
	c.records[record.ID()] = record
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		status := StatusPersisted
		if record.status == StatusPersisted {
			status = StatusRotated
		}

		if _, err := c.persistStatusWithTimeBounds(record, status, record.mint, record.maxt); err != nil {
			b.Fatal(err)
		}
	}
}
