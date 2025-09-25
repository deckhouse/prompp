package catalog

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/util"
)

const (
	// DefaultMaxLogFileSize default size of log file.
	DefaultMaxLogFileSize = 4 << 20

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
type Log interface {
	// ReWrite rewrite [FileLog] with [Record]s.
	ReWrite(records ...*Record) error

	// Read [Record] from [FileLog].
	Read(record *Record) error

	// Size return current size of [FileHandler].
	Size() int

	// Write [Record] to [FileLog].
	Write(record *Record) error
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
	return uuid.New()
}

//
// Catalog
//

// Catalog of current head records.
type Catalog struct {
	mtx                 sync.Mutex
	clock               clockwork.Clock
	log                 Log
	idGenerator         IDGenerator
	records             map[string]*Record
	maxLogFileSize      int
	corruptedHead       prometheus.Counter
	activeHeadCreatedAt prometheus.Gauge
}

// New init new [Catalog].
func New(
	clock clockwork.Clock,
	log Log,
	idGenerator IDGenerator,
	maxLogFileSize int,
	registerer prometheus.Registerer,
) (*Catalog, error) {
	factory := util.NewUnconflictRegisterer(registerer)
	catalog := &Catalog{
		clock:          clock,
		log:            log,
		idGenerator:    idGenerator,
		records:        make(map[string]*Record),
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
	r := &Record{
		id:             id,
		numberOfShards: numberOfShards,
		createdAt:      now,
		updatedAt:      now,
		deletedAt:      0,
		referenceCount: 0,
		status:         StatusNew,
	}

	if err := c.log.Write(r); err != nil {
		return r, fmt.Errorf(logWriteErr, err)
	}
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

	changed := createRecordCopy(r)
	changed.deletedAt = c.clock.Now().UnixMilli()
	changed.updatedAt = r.deletedAt

	if err := c.log.Write(changed); err != nil {
		return fmt.Errorf(logWriteErr, err)
	}

	applyRecordChanges(r, changed)
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
	records := c.list(filterFn)

	if sortLess != nil {
		sort.Slice(records, func(i, j int) bool {
			return sortLess(records[i], records[j])
		})
	}

	return records
}

// list returns slice of filtered records
func (c *Catalog) list(filterFn func(record *Record) bool) []*Record {
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

	changed := createRecordCopy(r)
	changed.corrupted = true
	changed.updatedAt = c.clock.Now().UnixMilli()

	if err = c.log.Write(changed); err != nil {
		return r, fmt.Errorf(logWriteErr, err)
	}

	applyRecordChanges(r, changed)
	c.records[id] = r

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

	if r.status == status {
		if status == StatusActive {
			c.activeHeadCreatedAt.Set(float64(r.createdAt))
		}

		return r, nil
	}

	changed := createRecordCopy(r)
	changed.status = status
	changed.updatedAt = c.clock.Now().UnixMilli()

	if err = c.log.Write(changed); err != nil {
		return r, fmt.Errorf(logWriteErr, err)
	}

	applyRecordChanges(r, changed)
	c.records[id] = r

	if status == StatusActive {
		c.activeHeadCreatedAt.Set(float64(r.createdAt))
	}

	return r, nil
}

// compactIfNeeded compact [Catalog] if necessary.
func (c *Catalog) compactIfNeeded() error {
	if c.log.Size() < c.maxLogFileSize {
		return nil
	}

	return c.compactLog()
}

// compactLog delete old(deleted [Record]s).
func (c *Catalog) compactLog() error {
	records := make([]*Record, 0, len(c.records))
	for _, record := range c.records {
		if record.deletedAt == 0 {
			records = append(records, record)
		}
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].createdAt < records[j].createdAt
	})

	return c.log.ReWrite(records...)
}

// sync catalog with [Log].
func (c *Catalog) sync() error {
	for {
		r := NewEmptyRecord()
		if err := c.log.Read(r); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			// this could happen if log file is corrupted
			logger.Errorf("catalog is corrupted: %v", err)

			return c.compactLog()
		}
		c.records[r.id.String()] = r
	}
}
