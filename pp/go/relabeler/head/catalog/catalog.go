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
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/prometheus/pp/go/util"
)

const (
	DefaultMaxLogFileSize = 4 << 20
)

type Log interface {
	Write(record *Record) error
	ReWrite(records ...*Record) error
	Read(record *Record) error
	Size() int
}

type IDGenerator interface {
	Generate() uuid.UUID
}

type DefaultIDGenerator struct{}

func (DefaultIDGenerator) Generate() uuid.UUID {
	return uuid.New()
}

type Catalog struct {
	mtx                 sync.Mutex
	clock               clockwork.Clock
	log                 Log
	idGenerator         IDGenerator
	records             map[string]*Record
	maxLogFileSize int
	corruptedHead       prometheus.Counter
	activeHeadCreatedAt prometheus.Gauge
}

func New(clock clockwork.Clock, log Log, idGenerator IDGenerator, maxLogFileSize int, registerer prometheus.Registerer) (*Catalog, error) {
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
		return nil, fmt.Errorf("faield to sync catalog: %w", err)
	}

	return catalog, nil
}

func (c *Catalog) List(filterFn func(record *Record) bool, sortLess func(lhs, rhs *Record) bool) (records []*Record, err error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	records = make([]*Record, 0, len(c.records))
	for _, record := range c.records {
		if filterFn != nil && !filterFn(record) {
			continue
		}
		records = append(records, record)
	}

	if sortLess != nil {
		sort.Slice(records, func(i, j int) bool {
			return sortLess(records[i], records[j])
		})
	}

	return records, nil
}

func (c *Catalog) Create(numberOfShards uint16) (r *Record, err error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if err = c.compactIfNeeded(); err != nil {
		return nil, fmt.Errorf("compact: %w", err)
	}

	id := c.idGenerator.Generate()
	now := c.clock.Now().UnixMilli()
	r = &Record{
		id:             id,
		numberOfShards: numberOfShards,
		createdAt:      now,
		updatedAt:      now,
		deletedAt:      0,
		referenceCount: 0,
		status:         StatusNew,
	}

	if err = c.log.Write(r); err != nil {
		return r, fmt.Errorf("log write: %w", err)
	}
	c.records[id.String()] = r

	return r, nil
}

func (c *Catalog) Get(id string) (*Record, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	r, ok := c.records[id]
	if !ok {
		return nil, fmt.Errorf("not found: %s", id)
	}

	return r, nil
}

func (c *Catalog) SetStatus(id string, status Status) (_ *Record, err error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if err = c.compactIfNeeded(); err != nil {
		return nil, fmt.Errorf("compact: %w", err)
	}

	r, ok := c.records[id]
	if !ok {
		return nil, fmt.Errorf("not found: %s", id)
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
		return r, fmt.Errorf("log write: %w", err)
	}

	applyRecordChanges(r, changed)
	c.records[id] = r

	if status == StatusActive {
		c.activeHeadCreatedAt.Set(float64(r.createdAt))
	}

	return r, nil
}

func (c *Catalog) SetCorrupted(id string) (_ *Record, err error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if err = c.compactIfNeeded(); err != nil {
		return nil, fmt.Errorf("compact: %w", err)
	}

	r, ok := c.records[id]
	if !ok {
		return nil, fmt.Errorf("not found: %s", id)
	}

	if r.corrupted {
		return r, nil
	}

	changed := createRecordCopy(r)
	changed.corrupted = true
	changed.updatedAt = c.clock.Now().UnixMilli()

	if err = c.log.Write(changed); err != nil {
		return r, fmt.Errorf("log write: %w", err)
	}

	applyRecordChanges(r, changed)
	c.records[id] = r

	c.corruptedHead.Inc()

	return r, nil
}

func (c *Catalog) Delete(id string) (err error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	if err = c.compactIfNeeded(); err != nil {
		return fmt.Errorf("compact: %w", err)
	}

	r, ok := c.records[id]
	if !ok || r.deletedAt > 0 {
		return nil
	}

	changed := createRecordCopy(r)
	changed.deletedAt = c.clock.Now().UnixMilli()
	changed.updatedAt = r.deletedAt

	if err = c.log.Write(changed); err != nil {
		return fmt.Errorf("log write: %w", err)
	}

	applyRecordChanges(r, changed)
	delete(c.records, r.id.String())

	return nil
}

func (c *Catalog) Compact() error {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return c.compact()
}

func (c *Catalog) sync() error {
	for {
		r := NewRecord()
		if err := c.log.Read(r); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			// this could happen if log file is corrupted
			logger.Errorf("catalog is corrupted: %v", err)

			return c.compact()
		}
		c.records[r.id.String()] = r
	}
}

func (c *Catalog) compactIfNeeded() error {
	if c.log.Size() < c.maxLogFileSize {
		return nil
	}

	return c.compact()
}

func (c *Catalog) compact() error {
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

func (c *Catalog) OnDiskSize() int64 {
	return int64(c.log.Size())
}
