package catalog

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
)

// deltaTestLog copies each write and can interleave an in-memory update before returning.
type deltaTestLog struct {
	FileLog
	last    SerializedRecord
	err     error
	onWrite func()
}

func (l *deltaTestLog) Write(sr *SerializedRecord) error {
	l.last = *sr

	if l.onWrite != nil {
		l.onWrite()
	}

	return l.err
}

func (s *V3FileTestSuite) TestV3Catalog_MutationsPreserveSegmentCountAdvancedDuringWrite() {
	for _, mutation := range []struct {
		name  string
		apply func(*Catalog, string) error
	}{
		{name: "delete", apply: (*Catalog).Delete},
		{
			name: "mark corrupted",
			apply: func(c *Catalog, id string) error {
				_, err := c.SetCorrupted(id)

				return err
			},
		},
		{
			name: "status and time bounds",
			apply: func(c *Catalog, id string) error {
				_, err := c.SetStatusWithTimeBounds(id, StatusRotated, 10, 20)

				return err
			},
		},
	} {
		s.Run(mutation.name, func() {
			// Arrange
			log := &deltaTestLog{FileLog: *s.fl}
			c, err := New(clockwork.NewFakeClock(), log, DefaultIDGenerator{}, DefaultMaxLogFileSize, nil)
			s.Require().NoError(err)
			record, err := c.Create(4)
			s.Require().NoError(err)
			record.SetNumberOfSegments(42)
			log.onWrite = func() { record.SetNumberOfSegments(43) }

			// Act
			err = mutation.apply(c, record.ID())

			// Assert
			s.Require().NoError(err)
			s.Equal(uint32(42), log.last.numberOfSegments)
			s.Equal(uint32(43), record.NumberOfSegments())
		})
	}
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
