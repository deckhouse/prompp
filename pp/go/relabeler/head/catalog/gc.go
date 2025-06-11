package catalog

import (
	"context"
	"errors"
	"fmt"
	"github.com/jonboulle/clockwork"
	"os"
	"path/filepath"
	"time"

	"github.com/prometheus/prometheus/pp/go/relabeler/head/ready"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
)

type TimeBoundCalculator interface {
	CalculateTimeBounds(ctx context.Context, headRecord *Record) (mint int64, maxt int64, err error)
}

type GC struct {
	clock               clockwork.Clock
	dataDir             string
	catalog             *Catalog
	readyNotifiable     ready.Notifiable
	retentionDuration   time.Duration
	timeBoundCalculator TimeBoundCalculator
	stop                chan struct{}
	stopped             chan struct{}
}

func NewGC(clock clockwork.Clock, dataDir string, catalog *Catalog, readyNotifiable ready.Notifiable, retentionDuration time.Duration, timeBoundCalculator TimeBoundCalculator) *GC {
	return &GC{
		clock:               clock,
		dataDir:             dataDir,
		catalog:             catalog,
		readyNotifiable:     readyNotifiable,
		retentionDuration:   retentionDuration,
		timeBoundCalculator: timeBoundCalculator,
		stop:                make(chan struct{}),
		stopped:             make(chan struct{}),
	}
}

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
			gc.Iterate(ctx)
		case <-gc.stop:
			return errors.New("stopped")
		}
	}
}

func (gc *GC) Iterate(ctx context.Context) {
	logger.Debugf("catalog gc iteration: head started")
	defer func() {
		logger.Debugf("catalog gc iteration: head ended")
	}()

	records, err := gc.catalog.List(
		func(record *Record) bool {
			return record.DeletedAt() == 0
		},
		func(lhs, rhs *Record) bool {
			return lhs.CreatedAt() < rhs.CreatedAt()
		},
	)
	if err != nil {
		logger.Debugf("catalog gc failed: %v", err)
		return
	}

	var retentionDurationIsExceeded bool
	var headTimeBoundsIsCalculated bool
	for _, record := range records {
		if record.deletedAt != 0 {
			continue
		}

		logger.Debugf("catalog gc iteration: head: %s", record.ID())
		if record.ReferenceCount() > 0 {
			return
		}

		retentionDurationIsExceeded, headTimeBoundsIsCalculated, err = gc.retentionDurationIsExceeded(ctx, record)
		if err != nil {
			logger.Errorf("calculate retention duration excess: %v", err)
			return
		}

		if retentionDurationIsExceeded || (record.status == StatusPersisted && !record.Corrupted()) {
			if err = gc.deleteRecord(ctx, record); err != nil {
				logger.Errorf("delete record: %v", err)
				return
			}
		}

		// avoid multiple calculations in one iteration
		if headTimeBoundsIsCalculated {
			return
		}
	}
}

func (gc *GC) retentionDurationIsExceeded(ctx context.Context, record *Record) (_ bool, calculated bool, err error) {
	mint, maxt := record.mint, record.maxt
	if mint > maxt {
		// recalculate mint & maxt
		mint, maxt, err = gc.timeBoundCalculator.CalculateTimeBounds(ctx, record)
		if err != nil {
			return false, false, fmt.Errorf("calculate time bounds: %w", err)
		}
		calculated = true
	}

	retentionDeadline := gc.clock.Now().Add(-gc.retentionDuration).UnixMilli()
	retentionDurationIsExceeded := retentionDeadline > maxt

	return retentionDurationIsExceeded, calculated, nil
}

func (gc *GC) deleteRecord(_ context.Context, record *Record) (err error) {
	if err = os.RemoveAll(filepath.Join(gc.dataDir, record.Dir())); err != nil {
		return fmt.Errorf("remove head dir: %w", err)
	}

	if err = gc.catalog.Delete(record.ID()); err != nil {
		return fmt.Errorf("delete head record: %w", err)
	}

	logger.Debugf("catalog gc iteration: head: %s: %s", record.ID(), "removed")

	return nil
}

func (gc *GC) Stop() {
	close(gc.stop)
	<-gc.stopped
}
