package catalog_test

import (
	"errors"
	"io"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/storage/catalog"
)

type CatalogSuite struct {
	suite.Suite

	gen   *testFixedUUIDGen
	clock *clockwork.FakeClock
}

func TestCatalogSuite(t *testing.T) {
	suite.Run(t, new(CatalogSuite))
}

func (s *CatalogSuite) SetupTest() {
	s.clock = clockwork.NewFakeClockAt(time.Unix(1, 0))
	s.gen = newTestFixedUUIDGen(3)
}

func (s *CatalogSuite) TestHappyPath() {
	tmpFile, err := os.CreateTemp("", "log_file")
	s.Require().NoError(err)

	logFileName := tmpFile.Name()
	s.Require().NoError(tmpFile.Close())

	l, err := catalog.NewFileLogV2(logFileName)
	s.Require().NoError(err)

	c, err := catalog.New(
		s.clock,
		l,
		s.gen,
		catalog.DefaultMaxLogFileSize,
		nil,
	)
	s.Require().NoError(err)

	now := s.clock.Now().UnixMilli()

	var nos1 uint16 = 2
	var nos2 uint16 = 4

	r1, err := c.Create(nos1)
	s.Require().NoError(err)

	s.Require().Equal(s.gen.last(), r1.ID())
	s.Require().Equal(s.gen.last(), r1.Dir())
	s.Require().Equal(nos1, r1.NumberOfShards())
	s.Require().Equal(now, r1.CreatedAt())
	s.Require().Equal(now, r1.UpdatedAt())
	s.Require().Equal(int64(0), r1.DeletedAt())
	s.Require().Equal(catalog.StatusNew, r1.Status())

	s.clock.Advance(time.Second)
	now = s.clock.Now().UnixMilli()

	r2, err := c.Create(nos2)
	s.Require().NoError(err)

	s.Require().Equal(s.gen.last(), r2.ID())
	s.Require().Equal(s.gen.last(), r2.Dir())
	s.Require().Equal(nos2, r2.NumberOfShards())
	s.Require().Equal(now, r2.CreatedAt())
	s.Require().Equal(now, r2.UpdatedAt())
	s.Require().Equal(int64(0), r2.DeletedAt())
	s.Require().Equal(catalog.StatusNew, r2.Status())

	_, err = c.SetStatus(r1.ID(), catalog.StatusPersisted)
	s.Require().NoError(err)

	c = nil
	s.Require().NoError(l.Close())

	l, err = catalog.NewFileLogV2(logFileName)
	s.Require().NoError(err)
	c, err = catalog.New(
		s.clock,
		l,
		catalog.DefaultIDGenerator{},
		catalog.DefaultMaxLogFileSize,
		nil,
	)
	s.Require().NoError(err)

	records := c.List(nil, nil)
	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt() < records[j].CreatedAt()
	})
	s.Require().Len(records, 2)
	s.Require().Equal(r1.ID(), records[0].ID())
	s.Require().Equal(r2.ID(), records[1].ID())
	s.Require().Equal(nos1, records[0].NumberOfShards())
	s.Require().Equal(nos2, records[1].NumberOfShards())
	s.Require().Equal(catalog.StatusPersisted, records[0].Status())
	s.Require().Equal(catalog.StatusNew, records[1].Status())
}

func (s *CatalogSuite) TestDeleteReopenFileLog() {
	tmpFile, err := os.CreateTemp("", "log_file")
	s.Require().NoError(err)

	logFileName := tmpFile.Name()
	s.Require().NoError(tmpFile.Close())

	l, err := catalog.NewFileLogV2(logFileName)
	s.Require().NoError(err)

	c, err := catalog.New(
		s.clock,
		l,
		s.gen,
		catalog.DefaultMaxLogFileSize,
		nil,
	)
	s.Require().NoError(err)

	r1, err := c.Create(2)
	s.Require().NoError(err)

	r2, err := c.Create(4)
	s.Require().NoError(err)

	_, err = c.SetStatus(r1.ID(), catalog.StatusPersisted)
	s.Require().NoError(err)
	s.Require().NoError(c.Delete(r1.ID()))

	s.Require().NoError(l.Close())

	l, err = catalog.NewFileLogV2(logFileName)
	s.Require().NoError(err)
	c, err = catalog.New(
		s.clock,
		l,
		s.gen,
		catalog.DefaultMaxLogFileSize,
		nil,
	)
	s.Require().NoError(err)

	_, err = c.Get(r1.ID())
	s.Require().Error(err)
	s.Require().ErrorContains(err, "not found: "+r1.ID())

	_, err = c.Get(r2.ID())
	s.Require().NoError(err)

	records := c.List(nil, nil)
	s.Require().Len(records, 1)
	s.Require().Equal(r2.ID(), records[0].ID())
}

func (s *CatalogSuite) TestCatalogSyncFail() {
	tmpFile, err := os.CreateTemp("", "log_file")
	s.Require().NoError(err)

	logFileName := tmpFile.Name()
	s.Require().NoError(tmpFile.Close())

	l, err := catalog.NewFileLogV2(logFileName)
	s.Require().NoError(err)

	c, err := catalog.New(
		s.clock,
		l,
		s.gen,
		catalog.DefaultMaxLogFileSize,
		prometheus.DefaultRegisterer,
	)
	s.Require().NoError(err)

	var nos1 uint16 = 2
	var nos2 uint16 = 4

	r1, err := c.Create(nos1)
	s.Require().NoError(err)

	r2, err := c.Create(nos2)
	s.Require().NoError(err)

	fileInfo, err := os.Stat(logFileName)
	s.Require().NoError(err)
	s.Require().NoError(os.Truncate(logFileName, fileInfo.Size()-1))

	l, err = catalog.NewFileLogV2(logFileName)
	s.Require().NoError(err)

	c, err = catalog.New(
		s.clock,
		l,
		s.gen,
		catalog.DefaultMaxLogFileSize,
		nil,
	)
	s.Require().NoError(err)

	restoredR1, err := c.Get(r1.ID())
	s.Require().NoError(err)

	_, err = c.Get(r2.ID())
	s.Require().Error(err)

	s.Require().Equal(r1.ID(), restoredR1.ID())
	s.Require().Equal(r1.Dir(), restoredR1.Dir())
	s.Require().Equal(r1.NumberOfShards(), restoredR1.NumberOfShards())
	s.Require().Equal(r1.CreatedAt(), restoredR1.CreatedAt())
	s.Require().Equal(r1.UpdatedAt(), restoredR1.UpdatedAt())
	s.Require().Equal(r1.DeletedAt(), restoredR1.DeletedAt())
	s.Require().Equal(r1.Status(), restoredR1.Status())
}

func (s *CatalogSuite) TestEmptyLog() {
	l := &LogMock{
		ReadFunc: func(*catalog.SerializedRecord) error { return io.EOF },
	}

	c, err := catalog.New(s.clock, l, s.gen, catalog.DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)
	s.Require().Empty(c.List(nil, nil))
}

func (s *CatalogSuite) TestSeedReadsTwoRecords() {
	writtenRecords := []*catalog.Record{
		catalog.NewRecordWithData(s.gen.ids[0], 2, 50, 50, 0, false, 0, catalog.StatusNew, nil),
		catalog.NewRecordWithData(s.gen.ids[1], 4, 200, 200, 0, false, 0, catalog.StatusPersisted, nil),
	}

	l := &LogMock{
		ReadFunc: func(sr *catalog.SerializedRecord) error {
			if len(writtenRecords) > 0 {
				*sr = writtenRecords[0].SerializedRecord

				writtenRecords = writtenRecords[1:]

				return nil
			}

			return io.EOF
		},
	}

	c, err := catalog.New(s.clock, l, s.gen, catalog.DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)

	got := c.List(nil, nil)
	s.Require().Len(got, 2)
	ids := map[string]struct{}{}
	for _, r := range got {
		ids[r.ID()] = struct{}{}
	}
	s.Require().Len(ids, 2)
	s.Require().Contains(ids, s.gen.ids[0].String())
	s.Require().Contains(ids, s.gen.ids[1].String())
}

func (s *CatalogSuite) TestSyncReadError_CompactSucceeds() {
	l := &LogMock{
		ReadFunc:    func(*catalog.SerializedRecord) error { return errors.New("corrupt read") },
		ReWriteFunc: func(...*catalog.SerializedRecord) error { return nil },
	}

	c, err := catalog.New(s.clock, l, s.gen, catalog.DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)

	s.Require().Empty(c.List(nil, nil))
	s.Require().GreaterOrEqual(len(l.ReWriteCalls()), 1)
}

func (s *CatalogSuite) TestSyncReadError_CompactFails() {
	expectedError := errors.New("rewrite failed")
	l := &LogMock{
		ReadFunc:    func(*catalog.SerializedRecord) error { return errors.New("corrupt read") },
		ReWriteFunc: func(...*catalog.SerializedRecord) error { return expectedError },
	}

	_, err := catalog.New(s.clock, l, s.gen, catalog.DefaultMaxLogFileSize, nil)
	s.Require().ErrorIs(err, expectedError)
	s.Require().ErrorContains(err, "failed to sync catalog")
}

func (s *CatalogSuite) TestCreate_CompactWhenSizeAtThresholdThenWrite() {
	l := &LogMock{
		ReadFunc:    func(*catalog.SerializedRecord) error { return io.EOF },
		ReWriteFunc: func(...*catalog.SerializedRecord) error { return nil },
		SizeFunc:    func() int { return 42 },
		WriteFunc:   func(*catalog.SerializedRecord) error { return nil },
	}

	c, err := catalog.New(s.clock, l, s.gen, 42, nil)
	s.Require().NoError(err)

	r, err := c.Create(3)
	s.Require().NoError(err)
	s.Require().Equal(s.gen.ids[0].String(), r.ID())
	s.Require().Len(l.ReWriteCalls(), 1)
	s.Require().Len(l.ReadCalls(), 1)
	s.Require().Len(l.WriteCalls(), 1)
}

func (s *CatalogSuite) TestCreate_CompactError() {
	expectedError := errors.New("compact failed")
	l := &LogMock{
		ReadFunc:    func(*catalog.SerializedRecord) error { return io.EOF },
		ReWriteFunc: func(...*catalog.SerializedRecord) error { return expectedError },
		SizeFunc:    func() int { return 42 },
	}

	c, err := catalog.New(s.clock, l, s.gen, 42, nil)
	s.Require().NoError(err)

	_, err = c.Create(1)
	s.Require().ErrorIs(err, expectedError)
}

func (s *CatalogSuite) TestCreate_WriteErrorReturnsRecordAndError() {
	expectedError := errors.New("write failed")
	l := &LogMock{
		ReadFunc:  func(*catalog.SerializedRecord) error { return io.EOF },
		WriteFunc: func(*catalog.SerializedRecord) error { return expectedError },
		SizeFunc:  func() int { return 42 },
	}

	c, err := catalog.New(s.clock, l, s.gen, catalog.DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)

	r, err := c.Create(5)
	s.Require().ErrorIs(err, expectedError)
	s.Require().ErrorContains(err, "log write:")
	s.Require().NotNil(r)
	s.Require().Equal(s.gen.ids[0].String(), r.ID())
}

func (s *CatalogSuite) TestGet_NotFound() {
	l := &LogMock{
		ReadFunc: func(*catalog.SerializedRecord) error { return io.EOF },
		SizeFunc: func() int { return 42 },
	}

	c, err := catalog.New(s.clock, l, s.gen, catalog.DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)

	nilID := uuid.Nil.String()
	_, err = c.Get(nilID)
	s.Require().Error(err)
	s.Require().ErrorContains(err, "not found: "+nilID)
}

func (s *CatalogSuite) TestDelete_UnknownID() {
	l := &LogMock{
		ReadFunc: func(*catalog.SerializedRecord) error { return io.EOF },
		SizeFunc: func() int { return 42 },
	}

	c, err := catalog.New(s.clock, l, s.gen, catalog.DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)

	unknownID := uuid.New().String()
	s.Require().NoError(c.Delete(unknownID))
}

func (s *CatalogSuite) TestDelete_SuccessAndGetFails() {
	l := &LogMock{
		ReadFunc:  func(*catalog.SerializedRecord) error { return io.EOF },
		SizeFunc:  func() int { return 42 },
		WriteFunc: func(*catalog.SerializedRecord) error { return nil },
	}

	c, err := catalog.New(s.clock, l, s.gen, catalog.DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)

	r, err := c.Create(2)
	s.Require().NoError(err)
	s.Require().NoError(c.Delete(r.ID()))

	_, err = c.Get(r.ID())
	s.Require().Error(err)
	s.Require().ErrorContains(err, "not found: "+r.ID())
}

func (s *CatalogSuite) TestDelete_CompactError() {
	size := 1000
	l := &LogMock{
		ReadFunc:    func(*catalog.SerializedRecord) error { return io.EOF },
		SizeFunc:    func() int { return size },
		WriteFunc:   func(*catalog.SerializedRecord) error { return nil },
		ReWriteFunc: func(...*catalog.SerializedRecord) error { return nil },
	}

	c, err := catalog.New(s.clock, l, s.gen, size, nil)
	s.Require().NoError(err)

	r, err := c.Create(1)
	s.Require().NoError(err)

	expectedError := errors.New("compact: compact failed")
	l.ReWriteFunc = func(...*catalog.SerializedRecord) error { return expectedError }
	err = c.Delete(r.ID())
	s.Require().ErrorIs(err, expectedError)
}

func (s *CatalogSuite) TestDelete_WriteError() {
	l := &LogMock{
		ReadFunc:  func(*catalog.SerializedRecord) error { return io.EOF },
		WriteFunc: func(*catalog.SerializedRecord) error { return nil },
		SizeFunc:  func() int { return 42 },
	}

	c, err := catalog.New(s.clock, l, s.gen, catalog.DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)

	r, err := c.Create(2)
	s.Require().NoError(err)

	expectedError := errors.New("write failed")
	l.WriteFunc = func(*catalog.SerializedRecord) error { return expectedError }
	err = c.Delete(r.ID())
	s.Require().ErrorIs(err, expectedError)

	got, err := c.Get(r.ID())
	s.Require().NoError(err)
	s.Require().Equal(r.ID(), got.ID())
}

func (s *CatalogSuite) TestList_FilterAndSort() {
	l := &LogMock{
		ReadFunc:  func(*catalog.SerializedRecord) error { return io.EOF },
		SizeFunc:  func() int { return 42 },
		WriteFunc: func(*catalog.SerializedRecord) error { return nil },
	}

	c, err := catalog.New(s.clock, l, s.gen, catalog.DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)

	_, err = c.Create(1)
	s.Require().NoError(err)

	s.clock.Advance(time.Hour)
	_, err = c.Create(2)
	s.Require().NoError(err)

	s.clock.Advance(time.Hour)
	_, err = c.Create(3)
	s.Require().NoError(err)

	all := c.List(nil, nil)
	s.Require().Len(all, 3)

	sh2 := c.List(func(r *catalog.Record) bool { return r.NumberOfShards() == 2 }, nil)
	s.Require().Len(sh2, 1)

	sorted := c.List(nil, func(a, b *catalog.Record) bool {
		return a.CreatedAt() < b.CreatedAt()
	})
	s.Require().Len(sorted, 3)
	for i := 1; i < len(sorted); i++ {
		s.Require().LessOrEqual(sorted[i-1].CreatedAt(), sorted[i].CreatedAt())
	}
}

func (s *CatalogSuite) TestOnDiskSize() {
	l := &LogMock{
		ReadFunc: func(*catalog.SerializedRecord) error { return io.EOF },
		SizeFunc: func() int { return 42 },
	}

	c, err := catalog.New(s.clock, l, s.gen, catalog.DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)
	s.Require().Equal(int64(42), c.OnDiskSize())
}

func (s *CatalogSuite) TestSetCorrupted_NotFound() {
	l := &LogMock{
		ReadFunc: func(*catalog.SerializedRecord) error { return io.EOF },
		SizeFunc: func() int { return 42 },
	}

	c, err := catalog.New(s.clock, l, s.gen, catalog.DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)

	nilID := uuid.Nil.String()
	_, err = c.SetCorrupted(nilID)
	s.Require().Error(err)
	s.Require().ErrorContains(err, "not found: "+nilID)
}

func (s *CatalogSuite) TestSetCorrupted_IdempotentNoSecondWrite() {
	l := &LogMock{
		ReadFunc:  func(*catalog.SerializedRecord) error { return io.EOF },
		SizeFunc:  func() int { return 42 },
		WriteFunc: func(*catalog.SerializedRecord) error { return nil },
	}

	c, err := catalog.New(s.clock, l, s.gen, catalog.DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)

	_, err = c.Create(2)
	s.Require().NoError(err)
	s.Require().Len(l.WriteCalls(), 1)

	_, err = c.SetCorrupted(s.gen.ids[0].String())
	s.Require().NoError(err)
	s.Require().Len(l.WriteCalls(), 2)

	_, err = c.SetCorrupted(s.gen.ids[0].String())
	s.Require().NoError(err)
	s.Require().Len(l.WriteCalls(), 2)
}

func (s *CatalogSuite) TestSetCorrupted_WriteError() {
	l := &LogMock{
		ReadFunc:  func(*catalog.SerializedRecord) error { return io.EOF },
		WriteFunc: func(*catalog.SerializedRecord) error { return nil },
		SizeFunc:  func() int { return 42 },
	}

	c, err := catalog.New(s.clock, l, s.gen, catalog.DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)

	r, err := c.Create(2)
	s.Require().NoError(err)

	expectedError := errors.New("write failed")
	l.WriteFunc = func(*catalog.SerializedRecord) error { return expectedError }
	_, err = c.SetCorrupted(s.gen.ids[0].String())
	s.Require().ErrorIs(err, expectedError)
	s.Require().ErrorContains(err, "log write:")

	got, err := c.Get(r.ID())
	s.Require().NoError(err)
	s.Require().False(got.Corrupted())
}

func (s *CatalogSuite) TestSetCorrupted_CompactError() {
	l := &LogMock{
		ReadFunc:    func(*catalog.SerializedRecord) error { return io.EOF },
		SizeFunc:    func() int { return 42 },
		WriteFunc:   func(*catalog.SerializedRecord) error { return nil },
		ReWriteFunc: func(...*catalog.SerializedRecord) error { return nil },
	}

	c, err := catalog.New(s.clock, l, s.gen, 1000, nil)
	s.Require().NoError(err)

	_, err = c.Create(1)
	s.Require().NoError(err)

	expectedError := errors.New("compact failed")
	l.SizeFunc = func() int { return 1000 }
	l.ReWriteFunc = func(...*catalog.SerializedRecord) error { return expectedError }
	_, err = c.SetCorrupted(s.gen.ids[0].String())
	s.Require().ErrorIs(err, expectedError)
	s.Require().ErrorContains(err, "compact:")
}

func (s *CatalogSuite) TestSetStatus_NotFound() {
	l := &LogMock{
		ReadFunc: func(*catalog.SerializedRecord) error { return io.EOF },
		SizeFunc: func() int { return 42 },
	}

	c, err := catalog.New(s.clock, l, s.gen, catalog.DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)

	nilID := uuid.Nil.String()
	_, err = c.SetStatus(nilID, catalog.StatusActive)
	s.Require().Error(err)
	s.Require().ErrorContains(err, "not found: "+nilID)
}

func (s *CatalogSuite) TestSetStatus_SameStatusActive() {
	l := &LogMock{
		ReadFunc:  func(*catalog.SerializedRecord) error { return io.EOF },
		SizeFunc:  func() int { return 42 },
		WriteFunc: func(*catalog.SerializedRecord) error { return nil },
	}

	c, err := catalog.New(s.clock, l, s.gen, catalog.DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)

	r, err := c.Create(2)
	s.Require().NoError(err)

	_, err = c.SetStatus(r.ID(), catalog.StatusActive)
	s.Require().NoError(err)
	s.Require().Len(l.WriteCalls(), 2)

	_, err = c.SetStatus(r.ID(), catalog.StatusActive)
	s.Require().NoError(err)
	s.Require().Len(l.WriteCalls(), 2)
}

func (s *CatalogSuite) TestSetStatus_Transition() {
	l := &LogMock{
		ReadFunc:  func(*catalog.SerializedRecord) error { return io.EOF },
		SizeFunc:  func() int { return 42 },
		WriteFunc: func(*catalog.SerializedRecord) error { return nil },
	}

	c, err := catalog.New(s.clock, l, s.gen, catalog.DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)

	r, err := c.Create(2)
	s.Require().NoError(err)

	_, err = c.SetStatus(r.ID(), catalog.StatusActive)
	s.Require().NoError(err)
	s.Require().Equal(catalog.StatusActive, c.List(nil, nil)[0].Status())
}

func (s *CatalogSuite) TestSetStatus_CompactError() {
	l := &LogMock{
		ReadFunc:    func(*catalog.SerializedRecord) error { return io.EOF },
		SizeFunc:    func() int { return 42 },
		WriteFunc:   func(*catalog.SerializedRecord) error { return nil },
		ReWriteFunc: func(...*catalog.SerializedRecord) error { return nil },
	}

	c, err := catalog.New(s.clock, l, s.gen, 1000, nil)
	s.Require().NoError(err)

	_, err = c.Create(1)
	s.Require().NoError(err)

	expectedError := errors.New("compact fail")
	l.ReWriteFunc = func(...*catalog.SerializedRecord) error { return expectedError }
	l.SizeFunc = func() int { return 1000 }
	_, err = c.SetStatus(s.gen.ids[0].String(), catalog.StatusPersisted)
	s.Require().ErrorIs(err, expectedError)
	s.Require().ErrorContains(err, "compact:")
}

func (s *CatalogSuite) TestSetStatus_WriteError() {
	l := &LogMock{
		ReadFunc:  func(*catalog.SerializedRecord) error { return io.EOF },
		WriteFunc: func(*catalog.SerializedRecord) error { return nil },
		SizeFunc:  func() int { return 42 },
	}

	c, err := catalog.New(s.clock, l, s.gen, catalog.DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)

	r, err := c.Create(2)
	s.Require().NoError(err)

	expectedError := errors.New("write failed")
	l.WriteFunc = func(*catalog.SerializedRecord) error { return expectedError }
	_, err = c.SetStatus(r.ID(), catalog.StatusPersisted)
	s.Require().ErrorIs(err, expectedError)
	s.Require().ErrorContains(err, "log write:")
	s.Require().Equal(catalog.StatusNew, c.List(nil, nil)[0].Status())
}

func (s *CatalogSuite) TestCompact_RemovesDeletedFromRewriteAndSortsByCreatedAt() {
	l := &LogMock{
		ReadFunc:    func(*catalog.SerializedRecord) error { return io.EOF },
		SizeFunc:    func() int { return 42 },
		WriteFunc:   func(*catalog.SerializedRecord) error { return nil },
		ReWriteFunc: func(...*catalog.SerializedRecord) error { return nil },
	}

	c, err := catalog.New(s.clock, l, s.gen, catalog.DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)

	rEarly, err := c.Create(1)
	s.Require().NoError(err)
	earlyCreated := rEarly.CreatedAt()

	s.clock.Advance(time.Hour)
	rLate, err := c.Create(1)
	s.Require().NoError(err)
	s.Require().NoError(c.Delete(rLate.ID()))
	s.Require().NoError(c.Compact())

	s.Require().Len(l.ReWriteCalls(), 1)
	records := c.List(nil, nil)
	s.Require().Len(records, 1)
	s.Require().Equal(earlyCreated, records[0].CreatedAt())
}

func (s *CatalogSuite) TestCompact_SortsTwoRecordsByCreatedAt() {
	l := &LogMock{
		ReadFunc:    func(*catalog.SerializedRecord) error { return io.EOF },
		SizeFunc:    func() int { return 42 },
		WriteFunc:   func(*catalog.SerializedRecord) error { return nil },
		ReWriteFunc: func(...*catalog.SerializedRecord) error { return nil },
	}

	c, err := catalog.New(s.clock, l, s.gen, catalog.DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)

	r1, err := c.Create(1)
	s.Require().NoError(err)
	t1 := r1.CreatedAt()

	s.clock.Advance(time.Hour)
	r2, err := c.Create(1)
	s.Require().NoError(err)
	t2 := r2.CreatedAt()
	s.Require().Less(t1, t2)

	s.Require().NoError(c.Compact())

	// Verify Compact rewrote the log with records sorted by createdAt by
	// inspecting the slice passed to Log.ReWrite. We can't rely on
	// catalog.List(nil, nil) here because the catalog backs records with a
	// Go map and List preserves map iteration order when no sortLess is
	// given — that order is intentionally randomized.
	calls := l.ReWriteCalls()
	s.Require().Len(calls, 1)
	s.Require().Len(calls[0].Srecords, 2)
	s.Require().Same(&r1.SerializedRecord, calls[0].Srecords[0])
	s.Require().Same(&r2.SerializedRecord, calls[0].Srecords[1])
}

func (s *CatalogSuite) TestCompact_ReWriteError() {
	l := &LogMock{
		ReadFunc:    func(*catalog.SerializedRecord) error { return io.EOF },
		SizeFunc:    func() int { return 42 },
		WriteFunc:   func(*catalog.SerializedRecord) error { return nil },
		ReWriteFunc: func(...*catalog.SerializedRecord) error { return errors.New("rewrite") },
	}

	c, err := catalog.New(s.clock, l, s.gen, catalog.DefaultMaxLogFileSize, nil)
	s.Require().NoError(err)

	_, err = c.Create(1)
	s.Require().NoError(err)

	expectedError := errors.New("rewrite failed")
	l.ReWriteFunc = func(...*catalog.SerializedRecord) error { return expectedError }

	err = c.Compact()
	s.Require().ErrorIs(err, expectedError)
}

//
// testFixedUUIDGen
//

// testFixedUUIDGen returns UUIDs from a fixed list in order.
type testFixedUUIDGen struct {
	ids      []uuid.UUID
	lastUUID uuid.UUID
	i        int
}

func newTestFixedUUIDGen(n int) *testFixedUUIDGen {
	fGen := &testFixedUUIDGen{
		ids: make([]uuid.UUID, 0, n),
	}
	for range n {
		fGen.ids = append(fGen.ids, uuid.New())
	}

	return fGen
}

// Generate UUID. Implementation [catalog.IDGenerator].
func (g *testFixedUUIDGen) Generate() uuid.UUID {
	g.lastUUID = g.ids[g.i]
	g.i++
	return g.lastUUID
}

// last returns last UUID as string.
func (g *testFixedUUIDGen) last() string {
	return g.lastUUID.String()
}
