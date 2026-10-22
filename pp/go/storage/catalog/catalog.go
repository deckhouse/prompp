package catalog

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/util"
)

//go:generate -command moq go run github.com/matryer/moq --rm --skip-ensure --pkg catalog_test --out

const (
	// DefaultMaxLogFileSize default size of log file.
	DefaultMaxLogFileSize = 4 << 20

	// uuidVersion7 identifies UUIDs with an embedded Unix timestamp in milliseconds.
	uuidVersion7 = 7

	// compactErr format string for compact error.
	compactErr = "compact: %w"
	// logWriteErr format string for log write error.
	logWriteErr = "log write: %w"
	// notFoundErr format string for not found id error.
	notFoundErr = "not found: %s"
)

//
// Log
//

// Log head-log file, contains [Record]s of heads.
//
//go:generate moq catalog_moq_test.go . Log
type Log interface {
	// NeedsRewrite reports whether loading must finish with a format conversion.
	NeedsRewrite() bool

	// ReWrite rewrite [FileLog] with [SerializedRecord]s.
	ReWrite(srecords ...*SerializedRecord) error

	// Read [SerializedRecord] from [FileLog].
	Read(sr *SerializedRecord) error

	// Size return current size of [FileHandler].
	Size() int

	// Write [Record] to [FileLog].
	Write(srecord *SerializedRecord) error
}

//
// IDGenerator
//

// IDGenerator generator UUID.
type IDGenerator interface {
	// Generate UUID.
	Generate() uuid.UUID
}

// DefaultIDGenerator default generator UUID.
type DefaultIDGenerator struct{}

// Generate UUID.
func (DefaultIDGenerator) Generate() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.New()
	}

	return id
}

//
// Catalog
//

// Catalog tracks heads and retains unavailable metadata for recovery at the next startup.
// Heads awaiting recovery are hidden from [Catalog.Get] and [Catalog.List].
type Catalog struct {
	mtx                 sync.Mutex
	clock               clockwork.Clock
	log                 Log
	idGenerator         IDGenerator
	records             map[string]*Record
	maxLogFileSize      int
	corruptedHead       prometheus.Counter
	activeHeadCreatedAt prometheus.Gauge
	inspector           HeadDirInspector
	corruptedRecords    prometheus.Counter
	journalFull         prometheus.Counter
	compactions         prometheus.Counter
	logRecords          int
	tombstones          int

	// pendingRecords retains heads with unavailable metadata for recovery on the next startup.
	pendingRecords map[string]*Record
}

// New init new [Catalog].
func New(
	clock clockwork.Clock,
	log Log,
	idGenerator IDGenerator,
	maxLogFileSize int,
	registerer prometheus.Registerer,
	options ...Option,
) (*Catalog, error) {
	factory := util.NewUnconflictRegisterer(registerer)
	catalog := &Catalog{
		compactions: factory.NewCounter(prometheus.CounterOpts{
			Name: "prompp_head_catalog_compactions_total",
			Help: "Total number of successful catalog log compactions.",
		}),
		corruptedRecords: factory.NewCounter(prometheus.CounterOpts{
			Name: "prompp_head_catalog_corrupted_records_total",
			Help: "Total number of damaged catalog records encountered during loading.",
		}),
		journalFull: factory.NewCounter(prometheus.CounterOpts{
			Name: "prompp_head_catalog_journal_full_total",
			Help: "Total number of compacted snapshots exceeding the journal capacity.",
		}),
		clock:          clock,
		log:            log,
		idGenerator:    idGenerator,
		records:        make(map[string]*Record),
		pendingRecords: make(map[string]*Record),
		maxLogFileSize: maxLogFileSize,
		corruptedHead: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "prompp_head_catalog_corrupted_head_total",
				Help: "Total number of corrupted heads.",
			},
		),
		activeHeadCreatedAt: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "prompp_head_catalog_active_head_created_at",
				Help: "The time when the active head was created.",
			},
		),
	}

	for _, option := range options {
		option(catalog)
	}

	if err := catalog.sync(); err != nil {
		return nil, fmt.Errorf("failed to sync catalog: %w", err)
	}

	return catalog, nil
}

// Compact catalog.
func (c *Catalog) Compact() error {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return c.compactLog()
}

// Create creates new [Record] and write to [Log].
func (c *Catalog) Create(numberOfShards uint16) (*Record, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if err := c.compactIfNeeded(); err != nil {
		return nil, fmt.Errorf(compactErr, err)
	}

	id := c.idGenerator.Generate()
	now := c.clock.Now().UnixMilli()
	r := NewRecordWithData(
		id,
		numberOfShards,
		now,
		now,
		0,
		false,
		0,
		StatusNew,
		nil,
	)

	if err := c.log.Write(&r.SerializedRecord); err != nil {
		return r, fmt.Errorf(logWriteErr, err)
	}
	c.logRecords++
	c.records[id.String()] = r

	return r, nil
}

// Delete record by ID.
func (c *Catalog) Delete(id string) error {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if err := c.compactIfNeeded(); err != nil {
		return fmt.Errorf(compactErr, err)
	}

	r, ok := c.records[id]
	if !ok || r.deletedAt > 0 {
		return nil
	}

	changed := r.SerializedRecord
	changed.deletedAt = c.clock.Now().UnixMilli()
	changed.updatedAt = changed.deletedAt
	changed.fields = diffFields(&r.SerializedRecord, &changed) | fieldUpdatedAt | fieldSegmentsCount

	if err := c.log.Write(&changed); err != nil {
		return fmt.Errorf(logWriteErr, err)
	}

	c.logRecords++
	applyRecordChanges(&r.SerializedRecord, &changed, changed.fields&^fieldSegmentsCount)
	c.tombstones++
	delete(c.records, r.id.String())

	return nil
}

// Get returns [Record] if exist.
func (c *Catalog) Get(id string) (*Record, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	r, ok := c.records[id]
	if !ok {
		return nil, fmt.Errorf(notFoundErr, id)
	}

	return r, nil
}

// List returns slice of records with filter and sort.
func (c *Catalog) List(filterFn func(record *Record) bool, sortLess func(lhs, rhs *Record) bool) []*Record {
	records := c.listWithFilter(filterFn)

	if sortLess != nil {
		sort.Slice(records, func(i, j int) bool {
			return sortLess(records[i], records[j])
		})
	}

	return records
}

// OnDiskSize size of [Log] file on disk.
func (c *Catalog) OnDiskSize() int64 {
	return int64(c.log.Size())
}

// SetCorrupted set corrupted flag for ID and returns [Record] if exist.
func (c *Catalog) SetCorrupted(id string) (_ *Record, err error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if err = c.compactIfNeeded(); err != nil {
		return nil, fmt.Errorf(compactErr, err)
	}

	r, ok := c.records[id]
	if !ok {
		return nil, fmt.Errorf(notFoundErr, id)
	}

	if r.corrupted {
		return r, nil
	}

	changed := r.SerializedRecord
	changed.corrupted = true
	changed.updatedAt = c.clock.Now().UnixMilli()
	changed.fields = diffFields(&r.SerializedRecord, &changed) | fieldUpdatedAt | fieldSegmentsCount

	if err = c.log.Write(&changed); err != nil {
		return r, fmt.Errorf(logWriteErr, err)
	}

	c.logRecords++
	applyRecordChanges(&r.SerializedRecord, &changed, changed.fields&^fieldSegmentsCount)

	c.corruptedHead.Inc()

	return r, nil
}

// SetStatus set status for ID and returns [Record] if exist.
func (c *Catalog) SetStatus(id string, status Status) (_ *Record, err error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if err = c.compactIfNeeded(); err != nil {
		return nil, fmt.Errorf(compactErr, err)
	}

	r, ok := c.records[id]
	if !ok {
		return nil, fmt.Errorf(notFoundErr, id)
	}

	return c.persistStatusWithTimeBounds(r, status, r.mint, r.maxt)
}

// SetStatusWithTimeBounds set status and time bounds of the [Head] data for ID and returns [Record] if exist.
func (c *Catalog) SetStatusWithTimeBounds(id string, status Status, mint, maxt int64) (_ *Record, err error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if err = c.compactIfNeeded(); err != nil {
		return nil, fmt.Errorf(compactErr, err)
	}

	r, ok := c.records[id]
	if !ok {
		return nil, fmt.Errorf(notFoundErr, id)
	}

	return c.persistStatusWithTimeBounds(r, status, mint, maxt)
}

// SetTimeBounds set time bounds of the [Head] data for ID and returns [Record] if exist.
func (c *Catalog) SetTimeBounds(id string, mint, maxt int64) (_ *Record, err error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if err = c.compactIfNeeded(); err != nil {
		return nil, fmt.Errorf(compactErr, err)
	}

	r, ok := c.records[id]
	if !ok {
		return nil, fmt.Errorf(notFoundErr, id)
	}

	return c.persistStatusWithTimeBounds(r, r.status, mint, maxt)
}

// applyLoadedRecord merges a [SerializedRecord] and tracks its earliest update time for recovery.
// Deletion records remove heads even when no earlier record for the UUID was loaded.
func (c *Catalog) applyLoadedRecord(sr *SerializedRecord, minUpdated map[uuid.UUID]int64) {
	c.logRecords++
	earliest, seenUpdate := minUpdated[sr.id]
	if sr.fields.has(fieldUpdatedAt) && (!seenUpdate || sr.updatedAt < earliest) {
		minUpdated[sr.id] = sr.updatedAt
	}

	if sr.fields.has(fieldDeletedAt) && sr.deletedAt != 0 {
		c.tombstones++
		delete(c.records, sr.id.String())

		return
	}

	current, ok := c.records[sr.id.String()]
	if !ok {
		current = NewEmptyRecord()
		current.id = sr.id
		c.records[sr.id.String()] = current
	}

	applyRecordChanges(&current.SerializedRecord, sr, sr.fields)
	current.fields |= sr.fields
}

// compactIfNeeded compact [Catalog] if necessary.
func (c *Catalog) compactIfNeeded() error {
	if fl, ok := c.log.(*FileLog); ok {
		if !fl.needsCompaction(c.maxLogFileSize) {
			return nil
		}

		return c.compactLog()
	}

	if c.log.Size() < c.maxLogFileSize {
		return nil
	}

	return c.compactLog()
}

// compactLog rewrite [Log] with current in-memory [Record]s.
func (c *Catalog) compactLog() error {
	srecords := make([]*SerializedRecord, 0, len(c.records)+len(c.pendingRecords))
	for _, records := range []map[string]*Record{c.records, c.pendingRecords} {
		for _, record := range records {
			snapshot := record.SerializedRecord
			snapshot.fields = fullFields(&snapshot)
			srecords = append(srecords, &snapshot)
		}
	}

	sort.Slice(srecords, func(i, j int) bool {
		return srecords[i].createdAt < srecords[j].createdAt
	})

	err := c.log.ReWrite(srecords...)
	if errors.Is(err, ErrJournalFull) {
		c.journalFull.Inc()
		var buffer [maxRecordSizeV3]byte
		total := 0
		for _, record := range srecords {
			frame, _ := appendFrameV3(buffer[:0], record)
			total += len(frame)
		}

		logger.Errorf("catalog journal cannot hold %d live heads (%d bytes): %v", len(srecords), total, err)
	}

	if err == nil {
		c.compactions.Inc()
		logger.Infof(
			"catalog compacted: live=%d droppedTombstones=%d droppedDeltas=%d",
			len(srecords), c.tombstones, max(0, c.logRecords-c.tombstones-len(srecords)),
		)
		c.logRecords = len(srecords)
		c.tombstones = 0
	}

	return err
}

// listWithFilter returns slice of filtered records.
func (c *Catalog) listWithFilter(filterFn func(record *Record) bool) []*Record {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	records := make([]*Record, 0, len(c.records))
	for _, record := range c.records {
		if filterFn != nil && !filterFn(record) {
			continue
		}
		records = append(records, record)
	}

	return records
}

// persistStatusWithTimeBounds writes status and time bound changes to [Log] before updating [Record].
func (c *Catalog) persistStatusWithTimeBounds(r *Record, status Status, mint, maxt int64) (_ *Record, err error) {
	if r.status == status && r.mint == mint && r.maxt == maxt {
		if status == StatusActive {
			c.activeHeadCreatedAt.Set(float64(r.createdAt))
		}

		return r, nil
	}

	changed := r.SerializedRecord
	changed.status = status
	changed.mint = mint
	changed.maxt = maxt
	changed.updatedAt = c.clock.Now().UnixMilli()
	changed.fields = diffFields(&r.SerializedRecord, &changed) | fieldUpdatedAt | fieldSegmentsCount

	if err = c.log.Write(&changed); err != nil {
		return r, fmt.Errorf(logWriteErr, err)
	}

	c.logRecords++
	applyRecordChanges(&r.SerializedRecord, &changed, changed.fields&^fieldSegmentsCount)

	if status == StatusActive {
		c.activeHeadCreatedAt.Set(float64(r.createdAt))
	}

	return r, nil
}

// recoveredCreationTime prefers the immutable UUID timestamp, then filesystem birth time.
func (c *Catalog) recoveredCreationTime(id uuid.UUID, fallback int64) int64 {
	if id.Version() == uuidVersion7 {
		seconds, nanos := id.Time().UnixTime()

		return time.Unix(seconds, nanos).UnixMilli()
	}

	if c.inspector != nil {
		if birth, err := c.inspector.BirthTime(id.String()); err == nil && !birth.IsZero() {
			return birth.UnixMilli()
		}
	}

	return fallback
}

// repairRecords reconstructs missing creation metadata and rotates heads affected by damaged frames.
func (c *Catalog) repairRecords(suspect map[uuid.UUID]struct{}, minUpdated map[uuid.UUID]int64) bool {
	changed := false
	for id, record := range c.records {
		_, damaged := suspect[record.id]
		if record.numberOfShards == 0 {
			restored, err := c.restoreRecordMetadata(record, minUpdated[record.id])
			if err != nil {
				logger.Warnf("deferring head metadata recovery until next startup: id=%s err=%v", id, err)
				c.pendingRecords[id] = record
			}

			changed = true
			if !restored {
				delete(c.records, id)

				continue
			}

			damaged = true
		}

		if damaged && (record.status == StatusActive || record.status == StatusNew) {
			record.status = StatusRotated
			changed = true
		}
	}

	return changed
}

// restoreRecordMetadata fills missing shard and creation metadata for a head.
// Heads without WAL shards are marked [StatusPersisted] for garbage collection.
// It returns false for missing directories or unavailable inspection; errors defer recovery until the next startup.
func (c *Catalog) restoreRecordMetadata(record *Record, fallback int64) (bool, error) {
	id := record.id.String()
	if !record.fields.has(fieldCreatedAt) {
		record.createdAt = c.recoveredCreationTime(record.id, fallback)
	}

	if c.inspector == nil {
		return false, fmt.Errorf("restore head %s: directory inspector is unavailable", id)
	}

	shards, err := c.inspector.NumberOfShards(id)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("inspect head %s: %w", id, err)
	}

	record.numberOfShards = shards
	if shards == 0 {
		record.status = StatusPersisted
	}

	return true, nil
}

// sync catalog with [Log].
func (c *Catalog) sync() error {
	suspect := make(map[uuid.UUID]struct{})
	minUpdated := make(map[uuid.UUID]int64)
	rewrite := c.log.NeedsRewrite()
	for {
		r := NewEmptyRecord()
		if err := c.log.Read(&r.SerializedRecord); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			if damaged, ok := errors.AsType[*CorruptedRecordError](err); ok {
				logger.Errorf(
					"catalog record is corrupted: id=%s offset=%d err=%v",
					damaged.ID, damaged.Offset, damaged.Err,
				)
				c.corruptedRecords.Inc()
				suspect[damaged.ID] = struct{}{}
				rewrite = true

				continue
			}

			if fl, ok := c.log.(*FileLog); ok && fl.version == LogFileVersionV3 {
				return err
			}

			logger.Errorf("catalog is corrupted: %v", err)
			rewrite = true

			break
		}

		c.applyLoadedRecord(&r.SerializedRecord, minUpdated)
	}

	if c.repairRecords(suspect, minUpdated) {
		rewrite = true
	}

	if rewrite {
		return c.compactLog()
	}

	return nil
}
