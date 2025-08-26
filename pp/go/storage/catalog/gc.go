package catalog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/prometheus/prometheus/pp/go/storage/logger"
)

//
// HeadsCatalog
//

// HeadsCatalog of current head records.
type HeadsCatalog interface {
	// Delete record by ID.
	Delete(id string) error

	// List returns slice of records with filter and sort.
	List(filterFn func(record *Record) bool, sortLess func(lhs, rhs *Record) bool) []*Record
}

//
// Notifiable
//

// Notifiable notifies the recipient that it is ready to work.
type Notifiable interface {
	// ReadyChan notifies the recipient that it is ready to work.
	ReadyChan() <-chan struct{}
}

//
// GC
//

// GC garbage collector for old [Head].
type GC struct {
	dataDir         string
	catalog         HeadsCatalog
	readyNotifiable Notifiable
	stop            chan struct{}
	stopped         chan struct{}
}

// NewGC init new [GC].
func NewGC(dataDir string, catalog HeadsCatalog, readyNotifiable Notifiable) *GC {
	return &GC{
		dataDir:         dataDir,
		catalog:         catalog,
		readyNotifiable: readyNotifiable,
		stop:            make(chan struct{}),
		stopped:         make(chan struct{}),
	}
}

// Iterate over the [Catalog] list and remove old [Head]s.
func (gc *GC) Iterate() {
	logger.Debugf("catalog gc iteration: head started")
	defer logger.Debugf("catalog gc iteration: head ended")

	records := gc.catalog.List(
		func(record *Record) bool {
			return record.DeletedAt() == 0
		},
		func(lhs, rhs *Record) bool {
			return lhs.CreatedAt() < rhs.CreatedAt()
		},
	)

	for _, record := range records {
		if record.deletedAt != 0 {
			continue
		}
		logger.Debugf("catalog gc iteration: head: %s", record.ID())
		if record.ReferenceCount() > 0 {
			return
		}

		if record.Corrupted() {
			logger.Debugf("catalog gc iteration: head: %s: %s", record.ID(), "corrupted")
			continue
		}

		if err := os.RemoveAll(filepath.Join(gc.dataDir, record.Dir())); err != nil {
			logger.Errorf("failed to remote head dir: %w", err)
			return
		}

		if err := gc.catalog.Delete(record.ID()); err != nil {
			logger.Errorf("failed to delete head record: %w", err)
			return
		}

		logger.Debugf("catalog gc iteration: head: %s: %s", record.ID(), "removed")
	}
}

// Run main loop [GC].
func (gc *GC) Run(ctx context.Context) error {
	defer close(gc.stopped)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-gc.readyNotifiable.ReadyChan():
		case <-gc.stop:
			return errors.New("stopped")
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Minute):
			gc.Iterate()
		case <-gc.stop:
			return errors.New("stopped")
		}
	}
}

// Stop the garbage collection loop.
func (gc *GC) Stop() {
	close(gc.stop)
	<-gc.stopped
}
