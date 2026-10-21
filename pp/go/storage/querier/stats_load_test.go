package querier_test

import (
	"context"
	"path/filepath"
	"time"

	"github.com/jonboulle/clockwork"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	ppstorage "github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/head/services"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
	"github.com/prometheus/prometheus/storage"
)

func (s *QuerierStatsSuite) TestRealDataStorageLoading() {
	for _, operation := range []string{"select_instant", "select_range", "select_chunks"} {
		s.Run(operation, func() {
			h := s.newUnloadableHead()
			maxt := int64(10)

			if operation == "select_instant" {
				maxt = 5
			}

			q := querier.NewQuerier(h, querier.NewNoOpShardedDeduplicator, 0, maxt, nil, s.metrics)
			cq := querier.NewChunkQuerier(h, querier.NewNoOpShardedDeduplicator, 0, 10, nil, s.metrics)

			if operation == "select_chunks" {
				result := cq.Select(context.Background(), false, s.hints, s.matchers...)
				s.Require().NoError(result.Err())
				s.True(result.Next())
			} else {
				hints := s.hints

				if operation == "select_instant" {
					hints = &storage.SelectHints{}
				}

				result := q.Select(context.Background(), false, hints, s.matchers...)
				s.Require().NoError(result.Err())
				s.True(result.Next())
			}

			s.assertSelect(operation, true)
		})
	}
}

func (s *QuerierStatsSuite) newUnloadableHead() *ppstorage.Head {
	s.T().Helper()
	dir := s.T().TempDir()
	cat, err := storagetest.CreateCatalog(
		clockwork.NewFakeClock(), filepath.Join(dir, "catalog.log"), &catalog.DefaultIDGenerator{},
	)
	s.Require().NoError(err)
	h, err := ppstorage.NewBuilder(cat, dir, maxSegmentSize, nil, time.Nanosecond).Build(0, numberOfShards)
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.Require().NoError(h.Close()) })
	storagetest.MustAppendTimeSeries(&s.Suite, h, []storagetest.TimeSeries{{
		Labels: labels.FromStrings("__name__", "stage_metric", "series", "0"),
		Samples: []cppbridge.Sample{
			{Timestamp: 0, Value: 1},
			{Timestamp: 5, Value: 2},
			{Timestamp: 10, Value: 3},
		},
	}})
	s.Require().NoError(services.UnloadUnusedSeriesDataWithHead(h))

	return h
}
