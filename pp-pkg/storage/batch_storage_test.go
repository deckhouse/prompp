package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	pp_model "github.com/prometheus/prometheus/pp/go/model"
	pp_storage "github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/storage"
)

const (
	maxSegmentSize            uint32 = 100e3
	unloadDataStorageInterval        = 5 * time.Minute
)

type testTimeSeriesBatch struct {
	timeSeries []pp_model.TimeSeries
}

func (b *testTimeSeriesBatch) TimeSeries() []pp_model.TimeSeries {
	return b.timeSeries
}

func (b *testTimeSeriesBatch) Destroy() {
	b.timeSeries = nil
}

type BatchStorageSuite struct {
	suite.Suite
	ctx     context.Context
	dataDir string
	catalog *catalog.Catalog
	builder *pp_storage.Builder
	clock   clockwork.Clock
	state   *cppbridge.StateV2
	adapter *Adapter
	manager *pp_storage.Manager
}

func TestBatchStorageSuite(t *testing.T) {
	suite.Run(t, new(BatchStorageSuite))
}

func (s *BatchStorageSuite) SetupTest() {
	s.ctx = context.Background()
	s.clock = clockwork.NewFakeClock()
	s.dataDir = s.createDataDir()
	s.builder = pp_storage.NewBuilder(
		s.catalog,
		s.dataDir,
		maxSegmentSize,
		prometheus.DefaultRegisterer,
		unloadDataStorageInterval,
	)
	s.state = cppbridge.NewTransitionStateV2()
	s.createManagerAndAdapter()
}

func (s *BatchStorageSuite) createDataDir() string {
	dir := filepath.Join(s.T().TempDir(), "data")
	s.Require().NoError(os.MkdirAll(dir, 0o755))
	return dir
}

func (s *BatchStorageSuite) createManagerAndAdapter() {
	fileLog, err := catalog.NewFileLogV2(filepath.Join(s.dataDir, "head.log"))
	s.Require().NoError(err)

	headCatalog, err := catalog.New(
		s.clock,
		fileLog,
		catalog.DefaultIDGenerator{},
		1024,
		nil,
	)
	s.Require().NoError(err)

	s.catalog = headCatalog

	var errManager error
	s.manager, errManager = pp_storage.NewManager(
		&pp_storage.Options{
			Seed:                0,
			BlockDuration:       2 * time.Hour,
			CommitInterval:      5 * time.Second,
			MaxRetentionPeriod:  24 * time.Hour,
			HeadRetentionPeriod: 4 * time.Hour,
			KeeperCapacity:      2,
			DataDir:             s.dataDir,
			MaxSegmentSize:      maxSegmentSize,
			NumberOfShards:      2,
		},
		s.clock,
		headCatalog,
		pp_storage.NewTriggerNotifier(),
		pp_storage.NewTriggerNotifier(),
		nil,
		prometheus.DefaultRegisterer,
	)
	s.Require().NoError(errManager)

	s.adapter = NewAdapter(
		s.clock,
		s.manager.Proxy(),
		s.manager.Builder(),
		s.manager.MergeOutOfOrderChunks,
		prometheus.DefaultRegisterer,
	)
}

func (s *BatchStorageSuite) TestAppendTimeSeries() {
	// Arrange
	batch := &testTimeSeriesBatch{
		timeSeries: []pp_model.TimeSeries{
			{
				LabelSet:  pp_model.NewLabelSetBuilder().Set("__name__", "metric").Set("job", "test").Build(),
				Timestamp: 1000,
				Value:     42.0,
			},
			{
				LabelSet:  pp_model.NewLabelSetBuilder().Set("__name__", "metric").Set("job", "test").Build(),
				Timestamp: 2000,
				Value:     43.0,
			},
		},
	}

	// Act
	stats, err := s.adapter.BatchStorage().(*BatchStorage).AppendTimeSeries(s.ctx, batch, s.state, false)

	// Assert
	s.Require().NoError(err)
	s.Require().NotNil(stats)
	s.Require().Equal(uint32(2), stats.SamplesAdded)
}

func (s *BatchStorageSuite) TestAppendTimeSeriesEmptyBatch() {
	// Arrange
	batch := &testTimeSeriesBatch{
		timeSeries: []pp_model.TimeSeries{},
	}

	// Act
	stats, err := s.adapter.BatchStorage().(*BatchStorage).AppendTimeSeries(s.ctx, batch, s.state, false)

	// Assert
	s.Require().NoError(err)
	s.Require().Equal(uint32(0), stats.SamplesAdded)
}

func (s *BatchStorageSuite) TestCommit_WhenNoSamplesAdded_ReturnsNil() {
	// Arrange

	// Act
	err := s.adapter.BatchStorage().(*BatchStorage).Commit(s.ctx)

	// Assert
	s.Require().NoError(err)
}

func (s *BatchStorageSuite) TestCommit_WithSamplesAdded() {
	// Arrange
	bs := s.adapter.BatchStorage().(*BatchStorage)

	batch := &testTimeSeriesBatch{
		timeSeries: []pp_model.TimeSeries{
			{
				LabelSet:  pp_model.NewLabelSetBuilder().Set("__name__", "test_metric").Set("job", "batch_test").Build(),
				Timestamp: 5000,
				Value:     100.0,
			},
		},
	}

	_, err := bs.AppendTimeSeries(s.ctx, batch, s.state, false)
	s.Require().NoError(err)

	// Act
	err = bs.Commit(s.ctx)

	// Assert
	s.Require().NoError(err)

	shard := s.manager.Proxy().Get().Shards()[0]
	s.Equal(
		[]cppbridge.Labels{{{Name: "__name__", Value: "test_metric"}, {Name: "job", Value: "batch_test"}}},
		shard.LSS().Target().GetLabelSets([]uint32{0}).LabelsSets())

	queryResult := shard.DataStorage().Query(cppbridge.DataStorageQuery{
		StartTimestampMs: 0,
		EndTimestampMs:   5000,
		LabelSetIDs:      []uint32{0},
	})
	s.Require().Equal(cppbridge.DataStorageQueryStatusSuccess, queryResult.Status)
	s.Equal(storagetest.SamplesMap{
		0: []cppbridge.Sample{
			{Timestamp: 5000, Value: 100.0},
		},
	}, storagetest.GetSamplesFromSerializedData(queryResult.SerializedData))
}

// TestEphemeralHead_IsSterileBetweenEvaluations validates the hypothesis that
// each call to adapter.BatchStorage() produces a fresh, empty ephemeral head.
//
// Background: rule evaluation creates a BatchStorage per Group.Eval() iteration
// and uses it as a primary in the FanoutQueryable used by the rule QueryFunc.
// If the ephemeral head leaked data between evaluations, a recording rule (or
// the ALERTS series written by alerting rules) from a previous iteration could
// influence subsequent rules' query results — which would be a serious data
// flow bug between rules in the same group.
//
// This test writes a series into one BatchStorage and confirms that a freshly
// obtained BatchStorage from the same adapter does NOT see that series.
func (s *BatchStorageSuite) TestEphemeralHead_IsSterileBetweenEvaluations() {
	// Arrange: write a series into the first ephemeral head.
	bs1 := s.adapter.BatchStorage().(*BatchStorage)
	batch := &testTimeSeriesBatch{
		timeSeries: []pp_model.TimeSeries{
			{
				LabelSet:  pp_model.NewLabelSetBuilder().Set("__name__", "leak_check").Set("job", "ephemeral").Build(),
				Timestamp: 1_000,
				Value:     1.0,
			},
		},
	}
	_, err := bs1.AppendTimeSeries(s.ctx, batch, s.state, false)
	s.Require().NoError(err)

	// Sanity: bs1's own querier MUST see the series we just wrote.
	q1, err := bs1.Querier(0, 10_000)
	s.Require().NoError(err)
	matcher := labels.MustNewMatcher(labels.MatchEqual, "__name__", "leak_check")
	got1 := collectLabelSets(q1.Select(s.ctx, false, nil, matcher))
	s.Require().Len(got1, 1, "bs1 must see its own series")

	// Act: obtain a brand-new BatchStorage from the SAME adapter.
	bs2 := s.adapter.BatchStorage().(*BatchStorage)

	// Assert 1: bs2's transactionHead MUST be a different instance.
	s.Require().NotSame(bs1.transactionHead, bs2.transactionHead,
		"each BatchStorage() call must build a brand-new TransactionHead")

	// Assert 2: bs2 MUST NOT see the series written into bs1.
	q2, err := bs2.Querier(0, 10_000)
	s.Require().NoError(err)
	got2 := collectLabelSets(q2.Select(s.ctx, false, nil, matcher))
	s.Require().Empty(got2,
		"bs2 must be sterile and not leak series from bs1 — got %v", got2)

	// Assert 3: nothing reached the main (active) head either, since bs1.Commit() was not called.
	mainQ, err := s.adapter.Querier(0, 10_000)
	s.Require().NoError(err)
	gotMain := collectLabelSets(mainQ.Select(s.ctx, false, nil, matcher))
	s.Require().Empty(gotMain,
		"main head must NOT see uncommitted ephemeral data — got %v", gotMain)
}

// collectLabelSets exhausts a SeriesSet and returns its series labels.
func collectLabelSets(ss storage.SeriesSet) []labels.Labels {
	var out []labels.Labels
	for ss.Next() {
		out = append(out, ss.At().Labels())
	}
	return out
}
