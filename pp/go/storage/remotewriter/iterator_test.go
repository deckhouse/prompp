package remotewriter

import (
	"cmp"
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/golang/snappy"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pp/go/storage/remotewriter/mock"
	"github.com/prometheus/prometheus/pp/go/storage/remotewriter/remotewritertest"
	"github.com/prometheus/prometheus/prompb"
)

type IteratorSuite struct {
	suite.Suite

	segmentSize prometheus.Histogram
}

func TestIteratorSuite(t *testing.T) {
	suite.Run(t, new(IteratorSuite))
}

func (s *IteratorSuite) SetupSuite() {
	s.segmentSize = prometheus.NewHistogram(prometheus.HistogramOpts{})
}

func (s *IteratorSuite) TestHappyPathV1() {
	clock := clockwork.NewRealClock()
	dataDir := s.T().TempDir()
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	numberOfShards := uint16(len(shardFilePaths)) // #nosec G115 // no overflow
	numberOfSegments := uint32(10)

	queueConfig := config.QueueConfig{
		MinShards:         1,
		MaxShards:         1,
		MaxSamplesPerSend: int(numberOfSegments - 1),
		SampleAgeLimit:    model.Duration(1 * time.Minute),
	}

	baseCtx := s.T().Context()

	startTimestamp := clock.Now().UnixMilli()

	tss := remotewritertest.GenerateTimeSeries(startTimestamp, uint64(numberOfSegments))
	err := remotewritertest.WriteToShardWalFileV1Multi(
		baseCtx,
		shardFilePaths,
		tss,
	)
	s.Require().NoError(err)

	discardCache := true
	corrupted := false
	corruptMarker := CorruptMarkerFn(func(string) error {
		corrupted = true
		return nil
	})
	rec := remotewritertest.MakeRecord(numberOfShards)
	rec.SetLastAppendedSegmentID(numberOfSegments/2 - 1)
	ds, err := newDataSourceActive(
		dataDir,
		DestinationConfig{},
		numberOfShards,
		discardCache,
		clock,
		newSegmentReadyChecker(rec),
		corruptMarker,
		rec,
		s.segmentSize,
	)
	s.Require().NoError(err)

	err = ds.Init(baseCtx, 0)
	s.Require().NoError(err)

	actualTargetSegmentID := uint32(0)
	targetSegmentIDSetCloser := &mock.TargetSegmentIDSetCloserMock{
		SetTargetSegmentIDFunc: func(segmentID uint32) error {
			actualTargetSegmentID = segmentID
			return nil
		},
		CloseFunc: func() error { return nil },
	}
	targetSegmentID := uint32(0)
	readTimeout := 10 * time.Second

	actualWR := &prompb.WriteRequest{}
	protobufWriter := &mock.ProtobufWriterMock{
		WriteFunc: func(_ context.Context, data []byte) error {
			decodeErr := s.decodeToWriteRequest(actualWR, data)
			s.Require().NoError(decodeErr)

			return nil
		},
	}

	metrics := newDestinationMetrics("test", "http://test.com")
	it, err := newIterator(
		clock,
		queueConfig,
		ds,
		targetSegmentIDSetCloser,
		targetSegmentID,
		readTimeout,
		protobufWriter,
		metrics,
	)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(it.Close()) }()

	b, err := it.Next(baseCtx)
	s.Require().NoError(err)
	s.Require().NotNil(b)
	s.Require().Equal(int(numberOfSegments), b.NumberOfSamples())
	s.Require().Equal(numberOfSegments/2, b.TargetSegmentID())

	msg := it.EncodeBatch(b)
	s.Require().NotNil(msg)
	s.Require().Equal(uint64(numberOfSegments), msg.NumberOfSamples())
	s.Require().Equal(startTimestamp+int64(numberOfSegments-1), msg.MaxTimestamp)
	s.Require().Equal(numberOfSegments/2, msg.TargetSegmentID)

	err = it.SendMessage(baseCtx, msg)
	s.Require().NoError(err)
	s.Require().Equal(numberOfSegments/2, actualTargetSegmentID)
	s.Require().Equal(tss.ToWriteRequest().String(), actualWR.String())
	s.Require().False(corrupted)
}

func (s *IteratorSuite) TestHappyPathV2() {
	clock := clockwork.NewRealClock()
	dataDir := s.T().TempDir()
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	numberOfShards := uint16(len(shardFilePaths)) // #nosec G115 // no overflow
	numberOfSegments := uint32(10)

	queueConfig := config.QueueConfig{
		MinShards:         1,
		MaxShards:         1,
		MaxSamplesPerSend: int(numberOfSegments - 1),
		SampleAgeLimit:    model.Duration(1 * time.Minute),
	}

	baseCtx := s.T().Context()

	startTimestamp := clock.Now().UnixMilli()

	rec := remotewritertest.MakeRecord(numberOfShards)
	tss := remotewritertest.GenerateTimeSeries(startTimestamp, uint64(numberOfSegments))
	err := remotewritertest.WriteToShardWalFileV2Multi(
		baseCtx,
		shardFilePaths,
		tss,
		rec,
	)
	s.Require().NoError(err)

	discardCache := true
	corrupted := false
	corruptMarker := CorruptMarkerFn(func(string) error {
		corrupted = true
		return nil
	})
	rec.SetLastAppendedSegmentID(numberOfSegments - 1)
	ds, err := newDataSourceActive(
		dataDir,
		DestinationConfig{},
		numberOfShards,
		discardCache,
		clock,
		newSegmentReadyChecker(rec),
		corruptMarker,
		rec,
		s.segmentSize,
	)
	s.Require().NoError(err)

	err = ds.Init(baseCtx, 0)
	s.Require().NoError(err)

	actualTargetSegmentID := uint32(0)
	targetSegmentIDSetCloser := &mock.TargetSegmentIDSetCloserMock{
		SetTargetSegmentIDFunc: func(segmentID uint32) error {
			actualTargetSegmentID = segmentID
			return nil
		},
		CloseFunc: func() error { return nil },
	}
	targetSegmentID := uint32(0)
	readTimeout := 10 * time.Second

	actualWR := &prompb.WriteRequest{}
	protobufWriter := &mock.ProtobufWriterMock{
		WriteFunc: func(_ context.Context, data []byte) error {
			decodeErr := s.decodeToWriteRequest(actualWR, data)
			s.Require().NoError(decodeErr)

			return nil
		},
	}

	metrics := newDestinationMetrics("test", "http://test.com")
	it, err := newIterator(
		clock,
		queueConfig,
		ds,
		targetSegmentIDSetCloser,
		targetSegmentID,
		readTimeout,
		protobufWriter,
		metrics,
	)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(it.Close()) }()

	b, err := it.Next(baseCtx)
	s.Require().NoError(err)
	s.Require().NotNil(b)
	s.Require().Equal(int(numberOfSegments), b.NumberOfSamples())
	s.Require().Equal(numberOfSegments, b.TargetSegmentID())

	msg := it.EncodeBatch(b)
	s.Require().NotNil(msg)
	s.Require().Equal(uint64(numberOfSegments), msg.NumberOfSamples())
	s.Require().Equal(startTimestamp+int64(numberOfSegments-1), msg.MaxTimestamp)
	s.Require().Equal(numberOfSegments, msg.TargetSegmentID)

	err = it.SendMessage(baseCtx, msg)
	s.Require().NoError(err)
	s.Require().Equal(numberOfSegments, actualTargetSegmentID)
	s.Require().Equal(tss.ToWriteRequest().String(), actualWR.String())
	s.Require().False(corrupted)
}

func (*IteratorSuite) decodeToWriteRequest(wr *prompb.WriteRequest, data []byte) error {
	uncompressedData, err := snappy.Decode(nil, data)
	if err != nil {
		return err
	}

	if err = wr.Unmarshal(uncompressedData); err != nil {
		return err
	}

	slices.SortFunc(wr.Timeseries, func(a, b prompb.TimeSeries) int {
		return cmp.Compare(a.Samples[0].Timestamp, b.Samples[0].Timestamp)
	})

	return nil
}

//
// Benchmark
//

// go test -test.fullpath=true -benchmem -run=^$ -tags stringlabels -bench ^BenchmarkIteratorV1$ github.com/prometheus/prometheus/pp/go/storage/remotewriter -v -count=1 -benchtime=1000x
func BenchmarkIteratorV1(b *testing.B) {
	if b.N != 100 && b.N != 1000 {
		return
	}
	clock := clockwork.NewRealClock()
	dataDir := b.TempDir()
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	numberOfShards := uint16(len(shardFilePaths)) // #nosec G115 // no overflow
	numberOfSegments := uint32(b.N * 10)

	queueConfig := config.QueueConfig{
		MinShards:         1,
		MaxShards:         1,
		MaxSamplesPerSend: int(9),
		SampleAgeLimit:    model.Duration(1 * time.Minute),
	}

	baseCtx := b.Context()

	startTimestamp := clock.Now().UnixMilli()

	tss := remotewritertest.GenerateTimeSeries(startTimestamp, uint64(numberOfSegments))
	err := remotewritertest.WriteToShardWalFileV1Multi(
		baseCtx,
		shardFilePaths,
		tss,
	)
	require.NoError(b, err)

	discardCache := true
	corruptMarker := CorruptMarkerFn(func(string) error { return nil })
	rec := remotewritertest.MakeRecord(numberOfShards)
	rec.SetLastAppendedSegmentID(numberOfSegments/2 - 1)
	ds, err := newDataSourceActive(
		dataDir,
		DestinationConfig{},
		numberOfShards,
		discardCache,
		clock,
		newSegmentReadyChecker(rec),
		corruptMarker,
		rec,
		prometheus.NewHistogram(prometheus.HistogramOpts{}),
	)
	require.NoError(b, err)

	err = ds.Init(baseCtx, 0)
	require.NoError(b, err)

	targetSegmentIDSetCloser := &mock.TargetSegmentIDSetCloserMock{
		SetTargetSegmentIDFunc: func(uint32) error {
			return nil
		},
		CloseFunc: func() error { return nil },
	}
	targetSegmentID := uint32(0)
	readTimeout := 10 * time.Second

	protobufWriter := &mock.ProtobufWriterMock{
		WriteFunc: func(context.Context, []byte) error { return nil },
	}

	metrics := newDestinationMetrics("test", "http://test.com")
	it, err := newIterator(
		clock,
		queueConfig,
		ds,
		targetSegmentIDSetCloser,
		targetSegmentID,
		readTimeout,
		protobufWriter,
		metrics,
	)
	require.NoError(b, err)
	defer func() { require.NoError(b, it.Close()) }()

	for range b.N {
		_, _ = it.Next(baseCtx)
	}
}

// go test -test.fullpath=true -benchmem -run=^$ -tags stringlabels -bench ^BenchmarkIteratorV2$ github.com/prometheus/prometheus/pp/go/storage/remotewriter -v -count=1 -benchtime=1000x
func BenchmarkIteratorV2(b *testing.B) {
	if b.N != 100 && b.N != 1000 {
		return
	}

	clock := clockwork.NewRealClock()
	dataDir := b.TempDir()
	shardFilePaths := []string{
		filepath.Join(dataDir, "shard_0.wal"),
		filepath.Join(dataDir, "shard_1.wal"),
	}
	numberOfShards := uint16(len(shardFilePaths)) // #nosec G115 // no overflow
	numberOfSegments := uint32(b.N * 10)

	queueConfig := config.QueueConfig{
		MinShards:         1,
		MaxShards:         1,
		MaxSamplesPerSend: int(9),
		SampleAgeLimit:    model.Duration(1 * time.Minute),
	}

	baseCtx := b.Context()

	startTimestamp := clock.Now().UnixMilli()

	rec := remotewritertest.MakeRecord(numberOfShards)
	tss := remotewritertest.GenerateTimeSeries(startTimestamp, uint64(numberOfSegments))
	err := remotewritertest.WriteToShardWalFileV2Multi(
		baseCtx,
		shardFilePaths,
		tss,
		rec,
	)
	require.NoError(b, err)

	discardCache := true
	corruptMarker := CorruptMarkerFn(func(string) error { return nil })
	rec.SetLastAppendedSegmentID(numberOfSegments - 1)
	ds, err := newDataSourceActive(
		dataDir,
		DestinationConfig{},
		numberOfShards,
		discardCache,
		clock,
		newSegmentReadyChecker(rec),
		corruptMarker,
		rec,
		prometheus.NewHistogram(prometheus.HistogramOpts{}),
	)
	require.NoError(b, err)

	err = ds.Init(baseCtx, 0)
	require.NoError(b, err)

	targetSegmentIDSetCloser := &mock.TargetSegmentIDSetCloserMock{
		SetTargetSegmentIDFunc: func(uint32) error {
			return nil
		},
		CloseFunc: func() error { return nil },
	}
	targetSegmentID := uint32(0)
	readTimeout := 10 * time.Second

	protobufWriter := &mock.ProtobufWriterMock{
		WriteFunc: func(context.Context, []byte) error { return nil },
	}

	metrics := newDestinationMetrics("test", "http://test.com")
	it, err := newIterator(
		clock,
		queueConfig,
		ds,
		targetSegmentIDSetCloser,
		targetSegmentID,
		readTimeout,
		protobufWriter,
		metrics,
	)
	require.NoError(b, err)
	defer func() { require.NoError(b, it.Close()) }()

	for range b.N {
		_, _ = it.Next(baseCtx)
	}
}
