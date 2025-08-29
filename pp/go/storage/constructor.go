package storage

import (
	"fmt"
	"os"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/head/container"
	"github.com/prometheus/prometheus/pp/go/storage/head/manager"
	"github.com/prometheus/prometheus/pp/go/storage/logger"
)

func HeadManagerCtor(
	l log.Logger,
	dataDir string,
	hcatalog *catalog.Catalog,
	maxSegmentSize uint32,
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

	headRecords := hcatalog.List(
		func(record *catalog.Record) bool {
			return record.DeletedAt() == 0 && record.Status() != catalog.StatusPersisted
		},
		func(lhs, rhs *catalog.Record) bool {
			return lhs.CreatedAt() < rhs.CreatedAt()
		},
	)

	loader := NewLoader(
		dataDir,
		maxSegmentSize,
		registerer,
	)

	builder := NewBuilder(
		hcatalog,
		dataDir,
		maxSegmentSize,
		registerer,
	)

	//
	activeHead := container.NewWeighted(expectedHead)

	m := manager.NewManager(
		activeHead,
		builder,
		loader,
		registerer,
	)

	return m, nil
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
