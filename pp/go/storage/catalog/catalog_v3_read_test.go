package catalog

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// recoveryInspector supplies deterministic directory metadata for recovery scenarios.
type recoveryInspector struct {
	shards uint16
	birth  time.Time
	err    error
}

// timedV7Generator constructs deterministic UUIDv7 identifiers from the supplied clock time.
type timedV7Generator struct {
	at time.Time
}

func (g timedV7Generator) Generate() uuid.UUID {
	var timestamp [8]byte
	binary.BigEndian.PutUint64(timestamp[:], uint64(g.at.UnixMilli())) // #nosec G115 // positive test time
	var id uuid.UUID
	copy(id[:6], timestamp[2:])
	id[6] = 0x70
	id[8] = 0x80

	return id
}

func (i recoveryInspector) BirthTime(string) (time.Time, error) {
	if i.birth.IsZero() {
		return time.Time{}, errors.New("birth time unavailable")
	}

	return i.birth, nil
}

func (i recoveryInspector) NumberOfShards(string) (uint16, error) {
	return i.shards, i.err
}

func (s *V3FileTestSuite) TestV3Catalog_DeletionAfterCorruptionDoesNotResurrect() {
	initial := newAppendTestRecord(1, 100)
	s.Require().NoError(s.fl.Write(&initial.SerializedRecord))
	delta := SerializedRecord{id: initial.id, fields: fieldStatus, status: StatusActive}
	offset := s.fl.file.writeOffset
	s.Require().NoError(s.fl.Write(&delta))
	delta = SerializedRecord{id: initial.id, fields: fieldDeletedAt, deletedAt: 1000}
	s.Require().NoError(s.fl.Write(&delta))
	s.corruptRecordByte(s.fl, offset+int64(recordChecksumOffsetV3), 1)
	s.crashAndReopen()

	c, err := New(clockwork.NewFakeClock(), s.fl, DefaultIDGenerator{}, DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)
	s.Empty(c.List(nil, nil))
	var got SerializedRecord
	s.Require().ErrorIs(s.fl.Read(&got), io.EOF)
}

func (s *V3FileTestSuite) TestV3Catalog_DeltasSurviveDowngradeAndRestart() {
	c, err := New(clockwork.NewFakeClock(), s.fl, DefaultIDGenerator{}, DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)
	first, err := c.Create(2)
	s.Require().NoError(err)
	second, err := c.Create(3)
	s.Require().NoError(err)
	first.SetNumberOfSegments(42)
	_, err = c.SetStatusWithTimeBounds(first.ID(), StatusActive, 10, 20)
	s.Require().NoError(err)
	s.Require().NoError(c.Delete(second.ID()))
	// A tombstone whose UUID has never been seen must not create a phantom head.
	deleted := SerializedRecord{id: uuid.New(), deletedAt: 123, fields: fieldDeletedAt}
	s.Require().NoError(s.fl.Write(&deleted))
	s.Require().NoError(s.fl.Close())

	s.fl, err = NewFileLogV2(s.filePath)
	s.Require().NoError(err)
	s.True(s.fl.NeedsRewrite())
	s.Require().ErrorIs(s.fl.Write(&first.SerializedRecord), ErrLogNeedsRewrite)
	c, err = New(clockwork.NewFakeClock(), s.fl, DefaultIDGenerator{}, DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)
	s.False(s.fl.NeedsRewrite())
	s.Len(c.List(nil, nil), 1)
	s.Require().NoError(s.fl.Close())
	s.fl, err = NewFileLogV2(s.filePath)
	s.Require().NoError(err)
	before, err := os.ReadFile(s.filePath)
	s.Require().NoError(err)
	c, err = New(clockwork.NewFakeClock(), s.fl, DefaultIDGenerator{}, DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)
	got, err := c.Get(first.ID())
	s.Require().NoError(err)
	s.Equal(uint16(2), got.NumberOfShards())
	s.Equal(uint32(42), got.NumberOfSegments())
	s.Require().NotNil(got.LastAppendedSegmentID())
	s.Equal(uint32(41), *got.LastAppendedSegmentID())
	s.Equal(StatusActive, got.Status())
	s.False(got.HasTimeBounds())
	after, err := os.ReadFile(s.filePath)
	s.Require().NoError(err)
	s.Equal(before, after)
}

func (s *V3FileTestSuite) TestV3Catalog_RepairsBeforeDowngrade() {
	for _, target := range []uint64{LogFileVersionV3, LogFileVersionV2} {
		for _, status := range []Status{StatusNew, StatusActive, StatusRotated, StatusPersisted} {
			s.Run(fmt.Sprintf("v%d/status%d", target, status), func() {
				path := filepath.Join(s.T().TempDir(), "head.log")
				fl, err := NewFileLogV3(path)
				s.Require().NoError(err)
				record := newAppendTestRecord(1, 100)
				record.status = status
				s.Require().NoError(fl.Write(&record.SerializedRecord))
				damagedOffset := fl.file.writeOffset
				delta := SerializedRecord{id: record.id, fields: fieldUpdatedAt, updatedAt: 200}
				s.Require().NoError(fl.Write(&delta))
				s.corruptRecordByte(fl, damagedOffset+int64(recordChecksumOffsetV3), 1)
				s.Require().NoError(fl.file.Close())
				fl, err = NewFileLog(path, target)
				s.Require().NoError(err)
				registry := prometheus.NewRegistry()
				c, err := New(clockwork.NewFakeClock(), fl, DefaultIDGenerator{}, DefaultMaxLogFileSize, registry)
				s.Require().NoError(err)
				s.Equal(float64(1), testutil.ToFloat64(c.corruptedRecords))
				got, err := c.Get(record.ID())
				s.Require().NoError(err)
				s.Equal(record.NumberOfShards(), got.NumberOfShards())
				s.Equal(record.CreatedAt(), got.CreatedAt())
				want := status
				if status == StatusNew || status == StatusActive {
					want = StatusRotated
				}

				s.Equal(want, got.Status())
				s.False(got.Corrupted())
				s.Require().NoError(fl.Close())
				fl, err = NewFileLog(path, target)
				s.Require().NoError(err)
				s.T().Cleanup(func() { _ = fl.Close() })
				before, err := os.ReadFile(path)
				s.Require().NoError(err)
				c, err = New(clockwork.NewFakeClock(), fl, DefaultIDGenerator{}, DefaultMaxLogFileSize, nil)
				s.Require().NoError(err)
				s.Zero(testutil.ToFloat64(c.corruptedRecords))
				after, err := os.ReadFile(path)
				s.Require().NoError(err)
				s.Equal(before, after)
			})
		}
	}
}

//revive:disable-next-line:cyclomatic // this is test.
//revive:disable-next-line:cognitive-complexity // this is test.
func (s *V3FileTestSuite) TestV3Catalog_RestoresLostInitialRecord() {
	const (
		// directoryReadError identifies recovery after a failed directory read.
		directoryReadError = "directory read error"
		// shardCountOverflow identifies recovery when the shard count cannot fit in uint16.
		shardCountOverflow = "shard count overflow"
	)

	for _, target := range []uint64{LogFileVersionV3, LogFileVersionV2} {
		for _, scenario := range []struct {
			name      string
			id        uuid.UUID
			inspector HeadDirInspector
			createdAt int64
			keep      bool
			pending   bool
			empty     bool
		}{
			{
				name: "uuid v7", id: (timedV7Generator{at: time.UnixMilli(1760936591360)}).Generate(),
				inspector: recoveryInspector{shards: 4}, createdAt: 1760936591360, keep: true,
			},
			{
				name:      "directory birth",
				id:        uuid.New(),
				inspector: recoveryInspector{shards: 4, birth: time.UnixMilli(500)},
				createdAt: 500,
				keep:      true,
			},
			{
				name: "minimum update of this head", id: uuid.New(), inspector: recoveryInspector{shards: 4},
				createdAt: 200, keep: true,
			},
			{name: "directory missing", id: uuid.New(), inspector: recoveryInspector{err: os.ErrNotExist}},
			{name: "empty directory", id: uuid.New(), empty: true},
			{name: "directory without WAL", id: uuid.New(), empty: true},
			{name: "inspector missing", id: uuid.New(), pending: true},
			{
				name: "permission denied", id: uuid.New(),
				inspector: recoveryInspector{err: os.ErrPermission}, pending: true,
			},
			{name: "I/O error", id: uuid.New(), inspector: recoveryInspector{err: syscall.EIO}, pending: true},
			{name: directoryReadError, id: uuid.New(), pending: true},
			{name: shardCountOverflow, id: uuid.New(), pending: true},
		} {
			s.Run(fmt.Sprintf("v%d/%s", target, scenario.name), func() {
				path := filepath.Join(s.T().TempDir(), "head.log")
				fl, err := NewFileLogV3(path)
				s.Require().NoError(err)
				initial := newAppendTestRecord(1, 100)
				initial.id = scenario.id
				s.Require().NoError(fl.ReWrite(&initial.SerializedRecord))
				base := fl.file.journalStart
				delta := SerializedRecord{
					id: scenario.id, fields: fieldStatus | fieldUpdatedAt | fieldSegmentsCount,
					status: StatusActive, updatedAt: 200, numberOfSegments: 10,
				}
				s.Require().NoError(fl.Write(&delta))
				delta.updatedAt = 300
				s.Require().NoError(fl.Write(&delta))
				other := newAppendTestRecord(9, 1)
				s.Require().NoError(fl.Write(&other.SerializedRecord))
				s.corruptRecordByte(fl, base+int64(recordChecksumOffsetV3), 1)
				s.Require().NoError(fl.file.Close())
				fl, err = NewFileLog(path, target)
				s.Require().NoError(err)
				s.T().Cleanup(func() { _ = fl.Close() })
				if scenario.empty {
					headDir := filepath.Join(filepath.Dir(path), scenario.id.String())
					s.Require().NoError(os.Mkdir(headDir, 0o700))
					if scenario.name == "directory without WAL" {
						s.Require().NoError(os.WriteFile(filepath.Join(headDir, "remote.cursor"), nil, 0o600))
					}

					scenario.inspector = NewHeadDirInspector(filepath.Dir(path))
				}

				if scenario.name == directoryReadError || scenario.name == shardCountOverflow {
					headDir := filepath.Join(filepath.Dir(path), scenario.id.String())
					if scenario.name == directoryReadError {
						s.Require().NoError(os.WriteFile(headDir, nil, 0o600))
					} else {
						s.Require().NoError(os.Mkdir(headDir, 0o700))
						s.Require().NoError(os.WriteFile(filepath.Join(headDir, "shard_65535.wal"), nil, 0o600))
					}

					scenario.inspector = NewHeadDirInspector(filepath.Dir(path))
				}

				c, err := New(clockwork.NewFakeClock(), fl, DefaultIDGenerator{}, DefaultMaxLogFileSize, nil,
					WithHeadDirInspector(scenario.inspector))
				s.Require().NoError(err)
				_, err = c.Get(other.ID())
				s.Require().NoError(err)

				if scenario.empty {
					got, getErr := c.Get(scenario.id.String())
					s.Require().NoError(getErr)
					s.Equal(StatusPersisted, got.Status())
					s.Zero(got.NumberOfShards())
					s.Require().NoError(fl.Close())
					fl, err = NewFileLog(path, target)
					s.Require().NoError(err)
					c, err = New(clockwork.NewFakeClock(), fl, DefaultIDGenerator{}, DefaultMaxLogFileSize, nil,
						WithHeadDirInspector(scenario.inspector))
					s.Require().NoError(err)
					got, err = c.Get(scenario.id.String())
					s.Require().NoError(err)
					s.Equal(StatusPersisted, got.Status())
					gc := NewGC(filepath.Dir(path), c, clockwork.NewFakeClock(), nil, nil, time.Hour)
					gc.Iterate()
					_, err = c.Get(scenario.id.String())
					s.Require().Error(err)
					_, err = os.Stat(filepath.Join(filepath.Dir(path), scenario.id.String()))
					s.Require().ErrorIs(err, os.ErrNotExist)

					return
				}

				if !scenario.keep {
					_, getErr := c.Get(scenario.id.String())
					s.Require().Error(getErr)
					s.Len(c.List(nil, nil), 1)
					s.Require().NoError(fl.Close())
					fl, err = NewFileLog(path, target)
					s.Require().NoError(err)
					c, err = New(clockwork.NewFakeClock(), fl, DefaultIDGenerator{}, DefaultMaxLogFileSize, nil)
					s.Require().NoError(err)
					s.Len(c.List(nil, nil), 1)
					_, err = c.Get(other.ID())
					s.Require().NoError(err)
					_, err = c.Create(2)
					s.Require().NoError(err)
					s.Require().NoError(c.Compact())

					if scenario.pending {
						s.Len(c.pendingRecords, 1)
						createdAt := c.pendingRecords[scenario.id.String()].CreatedAt()
						s.Require().NoError(fl.Close())
						fl, err = NewFileLog(path, target)
						s.Require().NoError(err)
						c, err = New(clockwork.NewFakeClock(), fl, DefaultIDGenerator{}, DefaultMaxLogFileSize, nil,
							WithHeadDirInspector(recoveryInspector{shards: 4}))
						s.Require().NoError(err)
						got, getErr := c.Get(scenario.id.String())
						s.Require().NoError(getErr)
						s.Equal(uint16(4), got.NumberOfShards())
						s.Equal(createdAt, got.CreatedAt())
						s.Equal(uint32(10), got.NumberOfSegments())
						s.Equal(StatusRotated, got.Status())
						s.Empty(c.pendingRecords)
					}

					if scenario.name == directoryReadError || scenario.name == shardCountOverflow {
						_, statErr := os.Stat(filepath.Join(filepath.Dir(path), scenario.id.String()))
						s.Require().NoError(statErr)
					}

					return
				}

				got, err := c.Get(scenario.id.String())
				s.Require().NoError(err)
				s.Equal(uint16(4), got.NumberOfShards())
				s.Equal(scenario.createdAt, got.CreatedAt())
				s.Equal(StatusRotated, got.Status())
				s.Equal(uint32(10), got.NumberOfSegments())
				s.False(got.Corrupted())
			})
		}
	}
}

func (s *V3FileTestSuite) TestV3Catalog_RestoresLostShardCount() {
	initial := newAppendTestRecord(1, 100)
	s.Require().NoError(s.fl.Write(&initial.SerializedRecord))
	delta := SerializedRecord{
		id: initial.id, fields: fieldStatus | fieldCreatedAt | fieldUpdatedAt | fieldSegmentsCount,
		status: StatusActive, createdAt: 123, updatedAt: 200, numberOfSegments: 10,
	}
	s.Require().NoError(s.fl.Write(&delta))
	s.corruptRecordByte(s.fl, s.fl.file.journalStart+int64(recordChecksumOffsetV3), 1)
	s.crashAndReopen()
	headDir := filepath.Join(filepath.Dir(s.filePath), initial.ID())
	s.Require().NoError(os.Mkdir(headDir, 0o700))
	walPath := filepath.Join(headDir, "shard_0.wal")
	s.Require().NoError(os.WriteFile(walPath, []byte("WAL data"), 0o600))
	c, err := New(clockwork.NewFakeClock(), s.fl, DefaultIDGenerator{}, DefaultMaxLogFileSize, nil,
		WithHeadDirInspector(recoveryInspector{shards: 4, err: syscall.EIO}))
	s.Require().NoError(err)
	s.Empty(c.List(nil, nil))
	s.Len(c.pendingRecords, 1)
	gc := NewGC(filepath.Dir(s.filePath), c, clockwork.NewFakeClock(), nil, nil, time.Hour)
	gc.Iterate()
	contents, err := os.ReadFile(walPath)
	s.Require().NoError(err)
	s.Equal("WAL data", string(contents))
	s.Require().NoError(c.Compact())
	s.reopen()
	c, err = New(clockwork.NewFakeClock(), s.fl, DefaultIDGenerator{}, DefaultMaxLogFileSize, nil,
		WithHeadDirInspector(recoveryInspector{shards: 4, birth: time.UnixMilli(500)}))
	s.Require().NoError(err)
	got, err := c.Get(initial.ID())
	s.Require().NoError(err)
	s.Equal(uint16(4), got.NumberOfShards())
	s.Equal(int64(123), got.CreatedAt())
	s.Equal(uint32(10), got.NumberOfSegments())
	s.Equal(StatusRotated, got.Status())

	s.reopen()
	c, err = New(clockwork.NewFakeClock(), s.fl, DefaultIDGenerator{}, DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)
	got, err = c.Get(initial.ID())
	s.Require().NoError(err)
	s.Equal(uint16(4), got.NumberOfShards())
	s.Equal(int64(123), got.CreatedAt())
	s.Equal(uint32(10), got.NumberOfSegments())
	s.Equal(StatusRotated, got.Status())
}
