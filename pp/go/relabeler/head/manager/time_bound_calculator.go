package manager

import (
	"context"
	"errors"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/relabeler/head"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/catalog"
	"path/filepath"
)

type TimeBoundCalculator struct {
	dir        string
	catalog    Catalog
	registerer prometheus.Registerer
}

func NewTimeBoundCalculator(dir string, catalog Catalog, registerer prometheus.Registerer) *TimeBoundCalculator {
	return &TimeBoundCalculator{dir: dir, catalog: catalog, registerer: registerer}
}

func (c *TimeBoundCalculator) CalculateTimeBounds(_ context.Context, record *catalog.Record) (mint int64, maxt int64, err error) {
	h, _, _, err := head.Load(
		record.ID(),
		0,
		filepath.Join(c.dir, record.Dir()),
		nil,
		record.NumberOfShards(),
		0,
		head.NoOpNumberOfSegmentsSetter{},
		c.registerer,
	)
	if err != nil {
		return 0, 0, err
	}

	stats := h.Status(1)
	mint, maxt = stats.HeadStats.MinTime, stats.HeadStats.MaxTime
	h.Stop()

	if err = errors.Join(h.Close(), h.Discard()); err != nil {
		return mint, maxt, err
	}

	if _, err = c.catalog.SetTimeBounds(record.ID(), mint, maxt); err != nil {
		return mint, maxt, fmt.Errorf("set time bounds in record: %w", err)
	}

	return mint, maxt, nil
}
