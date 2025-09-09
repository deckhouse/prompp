package storage

import (
	"fmt"
	"os"
	"time"

	"github.com/go-kit/log"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/head/container"
	"github.com/prometheus/prometheus/pp/go/storage/head/manager"
	"github.com/prometheus/prometheus/pp/go/storage/logger"
)

func HeadManagerCtor(
	l log.Logger,
	clock clockwork.Clock,
	dataDir string,
	hcatalog *catalog.Catalog,
	blockDuration time.Duration,
	maxSegmentSize uint32,
	numberOfShards uint16,
	registerer prometheus.Registerer,
) (*HeadManager, error) {
	dirStat, err := os.Stat(dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to stat dir: %w", err)
	}

	if !dirStat.IsDir() {
		return nil, fmt.Errorf("%s is not directory", dataDir)
	}

	InitLogHandler(l)

	builder := NewBuilder(
		hcatalog,
		dataDir,
		maxSegmentSize,
		registerer,
	)

	loader := NewLoader(
		dataDir,
		maxSegmentSize,
		registerer,
	)

	h, err := uploadOrBuildHead(
		clock,
		hcatalog,
		builder,
		loader,
		blockDuration,
		numberOfShards,
	)
	if err != nil {
		return nil, err
	}

	if _, err = hcatalog.SetStatus(h.ID(), catalog.StatusActive); err != nil {
		return nil, fmt.Errorf("failed to set active status: %w", err)
	}

	activeHead := container.NewWeighted(h)

	m := manager.NewManager(
		activeHead,
		builder,
		loader,
		numberOfShards,
		registerer,
	)

	return m, nil
}

func uploadOrBuildHead(
	clock clockwork.Clock,
	hcatalog *catalog.Catalog,
	builder *Builder,
	loader *Loader,
	blockDuration time.Duration,
	numberOfShards uint16,
) (*HeadOnDisk, error) {
	headRecords := hcatalog.List(
		func(record *catalog.Record) bool {
			statusIsAppropriate := record.Status() == catalog.StatusNew ||
				record.Status() == catalog.StatusActive

			isInBlockTimeRange := clock.Now().Sub(
				time.UnixMilli(record.CreatedAt()),
			).Milliseconds() < blockDuration.Milliseconds()

			return record.DeletedAt() == 0 && statusIsAppropriate && isInBlockTimeRange
		},
		func(lhs, rhs *catalog.Record) bool {
			return lhs.CreatedAt() > rhs.CreatedAt()
		},
	)

	if numberOfShards == 0 {
		numberOfShards = DefaultNumberOfShards
	}

	var generation uint64
	if len(headRecords) == 0 {
		// TODO	// m.counter.With(prometheus.Labels{"type": "created"}).Inc()
		return builder.Build(generation, numberOfShards)
	}

	h, corrupted := loader.UploadHead(headRecords[0], generation)
	if corrupted {
		if !headRecords[0].Corrupted() {
			if _, setCorruptedErr := hcatalog.SetCorrupted(headRecords[0].ID()); setCorruptedErr != nil {
				logger.Errorf("failed to set corrupted state, head id: %s: %v", headRecords[0].ID(), setCorruptedErr)
			}
		}
		// TODO	// m.counter.With(prometheus.Labels{"type": "corrupted"}).Inc()

		if _, err := hcatalog.SetStatus(headRecords[0].ID(), catalog.StatusRotated); err != nil {
			logger.Warnf("failed to set rotated status for head {%s}: %s", headRecords[0].ID(), err)
		}

		_ = h.Close()

		// TODO	// m.counter.With(prometheus.Labels{"type": "created"}).Inc()
		return builder.Build(generation, numberOfShards)
	}

	return h, nil
}
