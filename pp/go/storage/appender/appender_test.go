package appender_test

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/appender"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/head/services"
	"github.com/prometheus/prometheus/pp/go/storage/head/services/mock"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
	"github.com/prometheus/prometheus/pp/go/storage/head/task"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
	prom_storage "github.com/prometheus/prometheus/storage"
)

const (
	numberOfShards uint16 = 2

	maxSegmentSize uint32 = 3

	unloadDataStorageInterval time.Duration = 0

	// names of the series which are sharded to shard 0 and shard 1 of numberOfShards.
	shard0Series = "metric1"
	shard1Series = "metric2"
)

type AppenderSuite struct {
	suite.Suite
	dataDir        string
	head           *storage.Head
	appender       appender.Appender[*task.Generic[*shard.PerGoroutineShard], *shard.Shard, *shard.PerGoroutineShard, *storage.Head]
	walCommitCount int
	walCommitMasks [][]bool
}

func TestAppenderSuite(t *testing.T) {
	suite.Run(t, new(AppenderSuite))
}

func (s *AppenderSuite) SetupTest() {
	s.dataDir = s.createDataDirectory()

	s.createCatalog()

	h, err := storage.NewBuilder(
		s.createCatalog(),
		s.dataDir,
		maxSegmentSize,
		prometheus.DefaultRegisterer,
		unloadDataStorageInterval,
	).Build(0, numberOfShards)
	s.Require().NoError(err)

	s.head = h

	s.walCommitCount = 0
	s.walCommitMasks = nil
	s.appender = appender.New(s.head, s.commitAndFlush)
}

func (s *AppenderSuite) commitAndFlush(h *storage.Head, mask []bool) error {
	s.walCommitCount++
	s.walCommitMasks = append(s.walCommitMasks, slices.Clone(mask))
	return services.CFViaRangeByMask(h, mask)
}

// newHeadWithWalVersion creates a [storage.Head] whose shard WALs use walVersion
// and write segments to the returned mocks, one per shard.
func (s *AppenderSuite) newHeadWithWalVersion(walVersion uint8) (*storage.Head, []*mock.SegmentWriterMock) {
	segmentWriters := make([]*mock.SegmentWriterMock, numberOfShards)
	shards := make([]*shard.Shard, numberOfShards)
	for shardID := range numberOfShards {
		segmentWriters[shardID] = &mock.SegmentWriterMock{
			WriteFunc: func(*cppbridge.HeadEncodedSegment) error { return nil },
			FlushFunc: func() error { return nil },
			CloseFunc: func() error { return nil },
		}

		lss := shard.NewLSS()
		shards[shardID] = shard.NewShard(
			lss,
			shard.NewDataStorage(false, false),
			nil,
			nil,
			wal.NewWal(
				cppbridge.NewHeadWalEncoder(shardID, 0, lss.Target()),
				segmentWriters[shardID],
				walVersion,
				lss,
				maxSegmentSize,
				shardID,
				nil,
			),
			shardID,
		)
	}

	h := head.NewHead("appender-test-head", shards, shard.NewPerGoroutineShard[*storage.Wal], nil, 0, nil)
	s.T().Cleanup(func() { s.Require().NoError(h.Close()) })

	return h, segmentWriters
}

// seriesWithSamples returns numberOfSamples samples of the series with the given name.
func seriesWithSamples(name string, numberOfSamples int64) []model.TimeSeries {
	timeSeries := make([]model.TimeSeries, 0, numberOfSamples)
	for ts := range numberOfSamples {
		timeSeries = append(timeSeries, model.TimeSeries{
			LabelSet:  model.NewLabelSetBuilder().Set("__name__", name).Build(),
			Timestamp: uint64(ts + 1), // #nosec G115 // no overflow
			Value:     1.1,
		})
	}

	return timeSeries
}

func (s *AppenderSuite) createDataDirectory() string {
	dataDir := filepath.Join(s.T().TempDir(), "data")
	s.Require().NoError(os.MkdirAll(dataDir, os.ModeDir))
	return dataDir
}

func (s *AppenderSuite) createCatalog() *catalog.Catalog {
	l, err := catalog.NewFileLogV2(filepath.Join(s.dataDir, "catalog.log"))
	s.Require().NoError(err)

	c, err := catalog.New(
		clockwork.NewRealClock(),
		l,
		catalog.DefaultIDGenerator{},
		catalog.DefaultMaxLogFileSize,
		nil,
	)
	s.Require().NoError(err)

	return c
}

func (s *AppenderSuite) createState(config []*cppbridge.RelabelConfig) *cppbridge.StateV2 {
	statelessRelabeler, err := cppbridge.NewStatelessRelabeler(config)
	s.Require().NoError(err)

	state := cppbridge.NewStateV2WithoutLock()
	state.SetStatelessRelabeler(statelessRelabeler)

	return state
}

type headStorageData struct {
	lssResult []*cppbridge.LabelSetStorageGetLabelSetsResult
	dsResult  []cppbridge.DataStorageQueryResult
	shards    []storageData
}

type storageData struct {
	labels  []cppbridge.Labels
	samples storagetest.SamplesMap
}

func (s *AppenderSuite) getHeadData(labelSetIDs []uint32) headStorageData {
	data := headStorageData{
		lssResult: make([]*cppbridge.LabelSetStorageGetLabelSetsResult, 0, s.head.NumberOfShards()),
		dsResult:  make([]cppbridge.DataStorageQueryResult, 0, s.head.NumberOfShards()),
		shards:    make([]storageData, 0, s.head.NumberOfShards()),
	}

	for sh := range s.head.RangeShards() {
		lssResult := sh.LSS().Target().GetLabelSets(labelSetIDs)
		data.lssResult = append(data.lssResult, lssResult)

		dsResult := sh.DataStorage().Query(cppbridge.DataStorageQuery{
			StartTimestampMs: 0,
			EndTimestampMs:   math.MaxInt64,
			LabelSetIDs:      labelSetIDs,
		}, cppbridge.NoDownsampling, &prom_storage.SelectHints{})
		data.dsResult = append(data.dsResult, dsResult)

		data.shards = append(data.shards, storageData{
			labels:  lssResult.LabelsSets(),
			samples: storagetest.GetSamplesFromSerializedData(dsResult.SerializedData),
		})
	}

	return data
}

func (s *AppenderSuite) TestDropInvalidSeries() {
	// Arrange
	state := s.createState([]*cppbridge.RelabelConfig{})

	// Act
	stats, err := s.appender.Append(
		context.Background(),
		storagetest.NewIncomingData(&s.Suite, []model.TimeSeries{
			{
				LabelSet:  model.NewLabelSetBuilder().Set("name", "metric1").Build(),
				Timestamp: 1,
				Value:     1.1,
			},
		}),
		state,
		false)

	// Assert
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 0, SeriesAdded: 0, SeriesDrop: 1}, stats)
}

func (s *AppenderSuite) TestAppendMultipleSamplesInOneSeries() {
	// Arrange
	state := s.createState([]*cppbridge.RelabelConfig{})

	// Act
	stats, err := s.appender.Append(
		context.Background(),
		storagetest.NewIncomingData(&s.Suite, []model.TimeSeries{
			{
				LabelSet:  model.NewLabelSetBuilder().Set("__name__", "metric1").Build(),
				Timestamp: 1,
				Value:     1.1,
			},
			{
				LabelSet:  model.NewLabelSetBuilder().Set("__name__", "metric1").Build(),
				Timestamp: 2,
				Value:     1.1,
			},
		}),
		state,
		false)

	// Assert
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 2, SeriesAdded: 1, SeriesDrop: 0}, stats)

	data := s.getHeadData([]uint32{0})
	s.Equal([]storageData{
		{
			labels: []cppbridge.Labels{
				{{Name: "__name__", Value: "metric1"}},
			},
			samples: storagetest.SamplesMap{
				0: []cppbridge.Sample{{Timestamp: 1, Value: 1.1}, {Timestamp: 2, Value: 1.1}},
			},
		},
		{
			labels:  []cppbridge.Labels{nil},
			samples: storagetest.SamplesMap{},
		},
	}, data.shards)
}

func (s *AppenderSuite) TestSeriesPerShardTransfer() {
	// Arrange
	state := s.createState([]*cppbridge.RelabelConfig{{
		Regex:       "^label_for_drop$",
		Action:      cppbridge.LabelDrop,
		Separator:   ";",
		Replacement: "$1",
	}})

	// Act
	stats, err := s.appender.Append(
		context.Background(),
		storagetest.NewIncomingData(&s.Suite, []model.TimeSeries{
			{
				LabelSet:  model.NewLabelSetBuilder().Set("__name__", "metric1").Build(),
				Timestamp: 1,
				Value:     1.1,
			},
			{
				LabelSet:  model.NewLabelSetBuilder().Set("__name__", "metric1").Set("label_for_drop", "dropped").Build(),
				Timestamp: 1,
				Value:     1.1,
			},
		}),
		state,
		false)

	// Assert
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 2, SeriesAdded: 2, SeriesDrop: 0}, stats)

	data := s.getHeadData([]uint32{0})
	s.Equal([]storageData{
		{
			labels: []cppbridge.Labels{
				{{Name: "__name__", Value: "metric1"}},
			},
			samples: storagetest.SamplesMap{
				0: []cppbridge.Sample{{Timestamp: 1, Value: 1.1}},
			},
		},
		{
			labels:  []cppbridge.Labels{nil},
			samples: storagetest.SamplesMap{},
		},
	}, data.shards)
}

func (s *AppenderSuite) TestShardedRelabeledSeriesFullNotEmpty() {
	// Arrange
	state := s.createState([]*cppbridge.RelabelConfig{{
		TargetLabel: "label_for_drop",
		Action:      cppbridge.Replace,
		Separator:   ";",
		Replacement: "keep1",
	}})

	// Act
	stats, err := s.appender.Append(
		context.Background(),
		storagetest.NewIncomingData(&s.Suite, []model.TimeSeries{
			{
				LabelSet:  model.NewLabelSetBuilder().Set("__name__", "metric1").Build(),
				Timestamp: 1,
				Value:     1.1,
			},
			{
				LabelSet:  model.NewLabelSetBuilder().Set("__name__", "metric2").Build(),
				Timestamp: 1,
				Value:     1.1,
			},
			{
				LabelSet:  model.NewLabelSetBuilder().Set("__name__", "metric4").Build(),
				Timestamp: 1,
				Value:     1.1,
			},
			{
				LabelSet:  model.NewLabelSetBuilder().Set("__name__", "metric6").Build(),
				Timestamp: 1,
				Value:     1.1,
			},
		}),
		state,
		false)

	// Assert
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 4, SeriesAdded: 4, SeriesDrop: 0}, stats)

	data := s.getHeadData([]uint32{0, 1, 2, 3})
	s.Equal([]storageData{
		{
			labels: []cppbridge.Labels{
				{{Name: "__name__", Value: "metric1"}, {Name: "label_for_drop", Value: "keep1"}},
				{{Name: "__name__", Value: "metric6"}, {Name: "label_for_drop", Value: "keep1"}},
				cppbridge.Labels(nil),
				cppbridge.Labels(nil),
			},
			samples: storagetest.SamplesMap{
				0: []cppbridge.Sample{{Timestamp: 1, Value: 1.1}},
				1: []cppbridge.Sample{{Timestamp: 1, Value: 1.1}},
			},
		},
		{
			labels: []cppbridge.Labels{
				{{Name: "__name__", Value: "metric4"}, {Name: "label_for_drop", Value: "keep1"}},
				{{Name: "__name__", Value: "metric2"}, {Name: "label_for_drop", Value: "keep1"}},
				cppbridge.Labels(nil),
				cppbridge.Labels(nil),
			},
			samples: storagetest.SamplesMap{
				0: []cppbridge.Sample{{Timestamp: 1, Value: 1.1}},
				1: []cppbridge.Sample{{Timestamp: 1, Value: 1.1}},
			},
		},
	}, data.shards)
}

func (s *AppenderSuite) TestTrackStaleness() {
	// Arrange
	state := s.createState([]*cppbridge.RelabelConfig{})
	state.SetRelabelerOptions(&cppbridge.RelabelerOptions{
		HonorTimestamps: true,
	})
	state.EnableTrackStaleness()

	// Act
	stats, err := s.appender.Append(
		context.Background(),
		storagetest.NewIncomingData(&s.Suite, []model.TimeSeries{
			{
				LabelSet:  model.NewLabelSetBuilder().Set("__name__", "metric1").Build(),
				Timestamp: 1,
				Value:     1.1,
			},
			{
				LabelSet:  model.NewLabelSetBuilder().Set("__name__", "metric1").Build(),
				Timestamp: 2,
				Value:     1.1,
			},
		}),
		state,
		false)

	// Assert
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 2, SeriesAdded: 1, SeriesDrop: 0}, stats)

	data := s.getHeadData([]uint32{0})
	s.Equal([]storageData{
		{
			labels: []cppbridge.Labels{
				{{Name: "__name__", Value: "metric1"}},
			},
			samples: storagetest.SamplesMap{
				0: []cppbridge.Sample{{Timestamp: 1, Value: 1.1}, {Timestamp: 2, Value: 1.1}},
			},
		},
		{
			labels:  []cppbridge.Labels{nil},
			samples: storagetest.SamplesMap{},
		},
	}, data.shards)
}

func (s *AppenderSuite) TestTrackStalenessWithoutHonorTimestamps() {
	// Arrange
	const DefaultTimestamp = 1234567

	state := s.createState([]*cppbridge.RelabelConfig{})
	state.EnableTrackStaleness()
	state.SetDefTimestamp(DefaultTimestamp)

	// Act
	stats, err := s.appender.Append(
		context.Background(),
		storagetest.NewIncomingData(&s.Suite, []model.TimeSeries{
			{
				LabelSet:  model.NewLabelSetBuilder().Set("__name__", "metric1").Build(),
				Timestamp: 1,
				Value:     1.1,
			},
			{
				LabelSet:  model.NewLabelSetBuilder().Set("__name__", "metric1").Build(),
				Timestamp: 2,
				Value:     1.1,
			},
		}),
		state,
		false)

	// Assert
	s.Require().NoError(err)
	s.Equal(cppbridge.RelabelerStats{SamplesAdded: 2, SeriesAdded: 1, SeriesDrop: 0}, stats)

	data := s.getHeadData([]uint32{0})
	s.Equal([]storageData{
		{
			labels: []cppbridge.Labels{
				{{Name: "__name__", Value: "metric1"}},
			},
			samples: storagetest.SamplesMap{
				0: []cppbridge.Sample{{Timestamp: DefaultTimestamp, Value: 1.1}},
			},
		},
		{
			labels:  []cppbridge.Labels{nil},
			samples: storagetest.SamplesMap{},
		},
	}, data.shards)
}

func (s *AppenderSuite) TestWithoutCommitToWal() {
	// Arrange
	state := s.createState([]*cppbridge.RelabelConfig{})

	// Act
	_, err := s.appender.Append(
		context.Background(),
		storagetest.NewIncomingData(&s.Suite, []model.TimeSeries{
			{
				LabelSet:  model.NewLabelSetBuilder().Set("__name__", "metric1").Build(),
				Timestamp: 1,
				Value:     1.1,
			},
		}),
		state,
		false)

	// Assert
	s.Require().NoError(err)
	s.Equal(0, s.walCommitCount)
}

func (s *AppenderSuite) TestWithCommitToWal() {
	// Arrange
	state := s.createState([]*cppbridge.RelabelConfig{})

	// Act
	_, err := s.appender.Append(
		context.Background(),
		storagetest.NewIncomingData(&s.Suite, []model.TimeSeries{
			{
				LabelSet:  model.NewLabelSetBuilder().Set("__name__", "metric1").Build(),
				Timestamp: 1,
				Value:     1.1,
			},
		}),
		state,
		true)

	// Assert
	s.Require().NoError(err)
	s.Equal(1, s.walCommitCount)
}

func (s *AppenderSuite) TestWithCommitToWalByLimitExhausted() {
	// Arrange
	state := s.createState([]*cppbridge.RelabelConfig{})

	// Act
	_, err := s.appender.Append(
		context.Background(),
		storagetest.NewIncomingData(&s.Suite, []model.TimeSeries{
			{
				LabelSet:  model.NewLabelSetBuilder().Set("__name__", "metric1").Build(),
				Timestamp: 1,
				Value:     1.1,
			},
			{
				LabelSet:  model.NewLabelSetBuilder().Set("__name__", "metric1").Build(),
				Timestamp: 2,
				Value:     1.1,
			},
			{
				LabelSet:  model.NewLabelSetBuilder().Set("__name__", "metric1").Build(),
				Timestamp: 3,
				Value:     1.1,
			},
		}),
		state,
		false)

	// Assert
	s.Require().NoError(err)
	s.Equal(1, s.walCommitCount)
	s.Equal([][]bool{nil}, s.walCommitMasks)
}

func (s *AppenderSuite) TestLimitExhaustedWalV1CommitsAllShards() {
	// Arrange
	h, segmentWriters := s.newHeadWithWalVersion(wal.FileFormatVersion)
	state := s.createState([]*cppbridge.RelabelConfig{})

	// Act
	_, err := appender.New(h, s.commitAndFlush).Append(
		context.Background(),
		storagetest.NewIncomingData(&s.Suite, seriesWithSamples(shard0Series, int64(maxSegmentSize))),
		state,
		false)

	// Assert
	s.Require().NoError(err)
	s.Equal([][]bool{nil}, s.walCommitMasks)
	s.Len(segmentWriters[0].WriteCalls(), 1)
	s.Len(segmentWriters[1].WriteCalls(), 1)
}

func (s *AppenderSuite) TestLimitExhaustedWalV2CommitsOnlyExhaustedShard() {
	// Arrange
	h, segmentWriters := s.newHeadWithWalVersion(wal.FileFormatVersionV2)
	state := s.createState([]*cppbridge.RelabelConfig{})

	// Act
	_, err := appender.New(h, s.commitAndFlush).Append(
		context.Background(),
		storagetest.NewIncomingData(&s.Suite, seriesWithSamples(shard0Series, int64(maxSegmentSize))),
		state,
		false)

	// Assert
	s.Require().NoError(err)
	s.Equal([][]bool{{true, false}}, s.walCommitMasks)
	s.Len(segmentWriters[0].WriteCalls(), 1)
	s.Empty(segmentWriters[1].WriteCalls())
}

func (s *AppenderSuite) TestLimitExhaustedWalV2CommitsEveryExhaustedShard() {
	// Arrange
	h, segmentWriters := s.newHeadWithWalVersion(wal.FileFormatVersionV2)
	state := s.createState([]*cppbridge.RelabelConfig{})

	// Act
	_, err := appender.New(h, s.commitAndFlush).Append(
		context.Background(),
		storagetest.NewIncomingData(&s.Suite, append(
			seriesWithSamples(shard0Series, int64(maxSegmentSize)),
			seriesWithSamples(shard1Series, int64(maxSegmentSize))...,
		)),
		state,
		false)

	// Assert
	s.Require().NoError(err)
	s.Equal([][]bool{{true, true}}, s.walCommitMasks)
	s.Len(segmentWriters[0].WriteCalls(), 1)
	s.Len(segmentWriters[1].WriteCalls(), 1)
}

func (s *AppenderSuite) TestLimitNotExhaustedWalV2DoesNotCommit() {
	// Arrange
	h, segmentWriters := s.newHeadWithWalVersion(wal.FileFormatVersionV2)
	state := s.createState([]*cppbridge.RelabelConfig{})

	// Act
	_, err := appender.New(h, s.commitAndFlush).Append(
		context.Background(),
		storagetest.NewIncomingData(&s.Suite, seriesWithSamples(shard0Series, int64(maxSegmentSize)-1)),
		state,
		false)

	// Assert
	s.Require().NoError(err)
	s.Empty(s.walCommitMasks)
	s.Empty(segmentWriters[0].WriteCalls())
	s.Empty(segmentWriters[1].WriteCalls())
}

func (s *AppenderSuite) TestCommitToWalWalV2CommitsAllShards() {
	// Arrange
	h, segmentWriters := s.newHeadWithWalVersion(wal.FileFormatVersionV2)
	state := s.createState([]*cppbridge.RelabelConfig{})

	// Act
	_, err := appender.New(h, s.commitAndFlush).Append(
		context.Background(),
		storagetest.NewIncomingData(&s.Suite, seriesWithSamples(shard0Series, 1)),
		state,
		true)

	// Assert
	s.Require().NoError(err)
	s.Equal([][]bool{nil}, s.walCommitMasks)
	s.Len(segmentWriters[0].WriteCalls(), 1)
	s.Len(segmentWriters[1].WriteCalls(), 1)
}

/* func (s *AppenderSuite) TestUseRelabelerCache() {
	// Arrange
	state := s.createState([]*cppbridge.RelabelConfig{})

	// Act
	_, _, _ = s.appender.Append(
		context.Background(),
		storagetest.NewIncomingData(&s.Suite, []model.TimeSeries{
			{
				LabelSet:  model.NewLabelSetBuilder().Set("__name__", "metric1").Build(),
				Timestamp: 1,
				Value:     1.1,
			},
		}),
		state,
		false)

	_, stats, err := s.appender.Append(
		context.Background(),
		storagetest.NewIncomingData(&s.Suite, []model.TimeSeries{
			{
				LabelSet:  model.NewLabelSetBuilder().Set("__name__", "metric1").Build(),
				Timestamp: 1,
				Value:     1.1,
			},
		}),
		state,
		false)

	// Assert
	s.Require().NoError(err)
}*/
