package storage

import (
	"fmt"
	"os"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/head/container"
	"github.com/prometheus/prometheus/pp/go/storage/head/manager"
	"github.com/prometheus/prometheus/pp/go/storage/logger"
)

var DefaultNumberOfShards uint16 = 2

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

	initLogHandler(l)

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
			return lhs.CreatedAt() < rhs.CreatedAt()
		},
	)

	if numberOfShards == 0 {
		numberOfShards = DefaultNumberOfShards
	}

	var generation uint64
	if len(headRecords) == 0 {
		return builder.Build(generation, numberOfShards)
	}

	h, numberOfSegments, corrupted := loader.UploadHead(headRecords[0], generation)
	if corrupted {
		if _, err := hcatalog.SetStatus(headRecords[0].ID(), catalog.StatusRotated); err != nil {
			// TODO Warning ?
			return nil, fmt.Errorf("failed to set rotated status: %w", err)
		}

		// TODO loadResult.head.Stop()

		return builder.Build(generation, numberOfShards)
	}
}

// initLogHandler init log handler for pp.
func initLogHandler(l log.Logger) {
	l = log.With(l, "pp_caller", log.Caller(4))

	logger.Debugf = func(template string, args ...any) {
		level.Debug(l).Log("msg", fmt.Sprintf(template, args...))
	}

	logger.Infof = func(template string, args ...any) {
		level.Info(l).Log("msg", fmt.Sprintf(template, args...))
	}

	logger.Warnf = func(template string, args ...any) {
		level.Warn(l).Log("msg", fmt.Sprintf(template, args...))
	}

	logger.Errorf = func(template string, args ...any) {
		level.Error(l).Log("msg", fmt.Sprintf(template, args...))
	}
}
