package storage_test

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/head/services"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/writer"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/stretchr/testify/suite"
)

type idGeneratorStub struct {
	uuid uuid.UUID
}

func newIdGeneratorStub() *idGeneratorStub {
	return &idGeneratorStub{
		uuid: uuid.New(),
	}
}

func (g *idGeneratorStub) Generate() uuid.UUID {
	return g.uuid
}

func (g *idGeneratorStub) last() string {
	return g.uuid.String()
}

type HeadLoadSuite struct {
	suite.Suite
	dataDir         string
	clock           clockwork.Clock
	headIdGenerator *idGeneratorStub
	catalog         *catalog.Catalog
}

func TestHeadLoadSuite(t *testing.T) {
	suite.Run(t, new(HeadLoadSuite))
}

func (s *HeadLoadSuite) SetupTest() {
	dataDir, err := storagetest.CreateDataDirectory(s.T().TempDir())
	s.Require().NoError(err)
	s.dataDir = dataDir

	s.clock = clockwork.NewFakeClockAt(time.Now())
	s.headIdGenerator = newIdGeneratorStub()
	s.createCatalog()
}

func (s *HeadLoadSuite) createCatalog() {
	ctlg, err := storagetest.CreateCatalog(s.clock, filepath.Join(s.dataDir, "catalog.log"), s.headIdGenerator)
	s.Require().NoError(err)
	s.catalog = ctlg
}

func (s *HeadLoadSuite) headDir() string {
	return filepath.Join(s.dataDir, s.headIdGenerator.last())
}

func (s *HeadLoadSuite) createHead(unloadDataStorageInterval time.Duration) (*storage.Head, error) {
	return storage.NewBuilder(
		s.catalog,
		s.dataDir,
		storagetest.MaxSegmentSize,
		prometheus.DefaultRegisterer,
		unloadDataStorageInterval,
	).Build(0, storagetest.NumberOfShards)
}

func (s *HeadLoadSuite) loadNonWritableHead() (*storage.Head, error) {
	rec, err := s.catalog.Create(1)
	s.Require().NoError(err)
	headDir := filepath.Join(s.dataDir, rec.Dir())
	s.Require().NoError(os.Mkdir(headDir, 0o777))
	shardFilePath := storage.GetShardWalFilename(headDir, 0)
	shardFile, err := util.CreateFileAppender(shardFilePath, 0o666)
	s.Require().NoError(err)

	_, err = writer.WriteHeader(shardFile, wal.FileFormatVersion, cppbridge.EncodersVersion()-1)
	s.Require().NoError(err)
	s.Require().NoError(shardFile.Close())

	loader := storage.NewLoader(s.dataDir, storagetest.MaxSegmentSize, prometheus.DefaultRegisterer, storagetest.UnloadDataStorageInterval)
	return loader.Load(rec, 0)
}

func (s *HeadLoadSuite) mustCreateHead(unloadDataStorageInterval time.Duration) *storage.Head {
	h, err := s.createHead(unloadDataStorageInterval)
	s.Require().NoError(err)

	s.catalog.SetStatus(h.ID(), catalog.StatusActive)

	return h
}

func (s *HeadLoadSuite) loadHead(unloadDataStorageInterval time.Duration) (*storage.Head, error) {
	record, err := s.catalog.Get(s.headIdGenerator.last())
	s.Require().NoError(err)

	return storage.NewLoader(s.dataDir, storagetest.MaxSegmentSize, prometheus.DefaultRegisterer, unloadDataStorageInterval).Load(record, 0)
}

func (s *HeadLoadSuite) mustLoadHead(unloadDataStorageInterval time.Duration) *storage.Head {
	loadedHead, err := s.loadHead(unloadDataStorageInterval)
	s.Require().NoError(err)

	return loadedHead
}

func (s *HeadLoadSuite) lockFileForCreation(fileName string) {
	s.Require().NoError(os.RemoveAll(fileName))
	s.Require().NoError(os.Mkdir(fileName, os.ModeDir))
}

func (s *HeadLoadSuite) appendTimeSeries(head *storage.Head, timeSeries []storagetest.TimeSeries) {
	storagetest.MustAppendTimeSeries(&s.Suite, head, timeSeries)
}

func (*HeadLoadSuite) shards(head *storage.Head) (result []*shard.Shard) {
	for sd := range head.RangeShards() {
		result = append(result, sd)
	}

	return result
}

func (s *HeadLoadSuite) TestErrorCreateShardFileInOneShard() {
	// Arrange
	s.Require().NoError(os.Mkdir(s.headDir(), 0), os.ModeDir)
	s.lockFileForCreation(storage.GetShardWalFilename(s.headDir(), 0))

	// Act
	head, err := s.createHead(0)

	// Assert
	s.Require().Error(err)
	s.Nil(head)
}

func (s *HeadLoadSuite) TestErrorOpenShardFileInOneShard() {
	// Arrange
	sourceHead := s.mustCreateHead(0)
	s.NoError(sourceHead.Close())

	s.Require().NoError(os.Remove(storage.GetShardWalFilename(s.headDir(), 0)))

	// Act
	head, err := s.loadHead(0)

	// Assert
	s.Require().Error(err)
	s.Nil(s.shards(head)[0].UnloadedDataStorage())
	s.Require().NoError(head.Close())
}

func (s *HeadLoadSuite) TestErrorOpenShardFileInAllShards() {
	// Arrange
	sourceHead := s.mustCreateHead(0)
	s.NoError(sourceHead.Close())

	s.Require().NoError(os.Remove(storage.GetShardWalFilename(s.headDir(), 0)))
	s.Require().NoError(os.Remove(storage.GetShardWalFilename(s.headDir(), 1)))

	// Act
	head, err := s.loadHead(0)

	// Assert
	s.Require().Error(err)
	s.Nil(s.shards(head)[0].UnloadedDataStorage())
	s.Nil(s.shards(head)[1].UnloadedDataStorage())
	s.Require().NoError(head.Close())
}

func (s *HeadLoadSuite) TestLoadWithDisabledDataUnloading() {
	// Arrange
	sourceHead := s.mustCreateHead(0)
	s.appendTimeSeries(sourceHead, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "wal_metric"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
				{Timestamp: 1, Value: 2},
				{Timestamp: 2, Value: 3},
			},
		},
	})
	s.Require().NoError(sourceHead.Close())

	// Act
	loadedHead := s.mustLoadHead(0)

	queryResult := s.shards(loadedHead)[0].DataStorage().Query(cppbridge.DataStorageQuery{
		StartTimestampMs: 0,
		EndTimestampMs:   2,
		LabelSetIDs:      []uint32{0},
	})
	err := loadedHead.Close()

	// Assert
	s.Require().NoError(err)
	s.Nil(s.shards(loadedHead)[0].UnloadedDataStorage())
	s.Nil(s.shards(loadedHead)[0].QueriedSeriesStorage())
	s.Nil(s.shards(loadedHead)[1].UnloadedDataStorage())
	s.Nil(s.shards(loadedHead)[1].QueriedSeriesStorage())
	s.Equal(cppbridge.DataStorageQueryStatusSuccess, queryResult.Status)
	s.Equal(storagetest.SamplesMap{
		0: []cppbridge.Sample{
			{Timestamp: 0, Value: 1},
			{Timestamp: 1, Value: 2},
			{Timestamp: 2, Value: 3},
		},
	}, storagetest.GetSamplesFromSerializedData(queryResult.SerializedData))
	s.Equal([]cppbridge.Labels{
		{{Name: "__name__", Value: "wal_metric"}},
	}, s.shards(loadedHead)[0].LSS().Target().GetLabelSets([]uint32{0}).LabelsSets())
}

func (s *HeadLoadSuite) TestAppendAfterLoad() {
	// Arrange
	sourceHead := s.mustCreateHead(0)
	s.appendTimeSeries(sourceHead, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "wal_metric"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
				{Timestamp: 1, Value: 2},
				{Timestamp: 2, Value: 3},
			},
		},
	})
	s.Require().NoError(sourceHead.Close())

	// Act
	loadedHead := s.mustLoadHead(0)
	s.appendTimeSeries(loadedHead, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "wal_metric"),
			Samples: []cppbridge.Sample{
				{Timestamp: 3, Value: 4},
			},
		},
	})

	queryResult := s.shards(loadedHead)[0].DataStorage().Query(cppbridge.DataStorageQuery{
		StartTimestampMs: 0,
		EndTimestampMs:   4,
		LabelSetIDs:      []uint32{0},
	})

	err := loadedHead.Close()

	// Assert
	s.Require().NoError(err)
	s.Equal(cppbridge.DataStorageQueryStatusSuccess, queryResult.Status)
	s.Equal(storagetest.SamplesMap{
		0: []cppbridge.Sample{
			{Timestamp: 0, Value: 1},
			{Timestamp: 1, Value: 2},
			{Timestamp: 2, Value: 3},
			{Timestamp: 3, Value: 4},
		},
	}, storagetest.GetSamplesFromSerializedData(queryResult.SerializedData))
	s.Equal([]cppbridge.Labels{
		{{Name: "__name__", Value: "wal_metric"}},
	}, s.shards(loadedHead)[0].LSS().Target().GetLabelSets([]uint32{0}).LabelsSets())
}

func (s *HeadLoadSuite) TestLoadWithEnabledDataUnloading() {
	// Arrange
	sourceHead := s.mustCreateHead(0)
	s.appendTimeSeries(sourceHead, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "wal_metric"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
				{Timestamp: 1, Value: 2},
				{Timestamp: 2, Value: 3},
			},
		},
	})
	s.appendTimeSeries(sourceHead, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "wal_metric"),
			Samples: []cppbridge.Sample{
				{Timestamp: 100, Value: 1},
				{Timestamp: 101, Value: 2},
				{Timestamp: 102, Value: 3},
			},
		},
	})

	s.Require().NoError(sourceHead.Close())

	// Act
	loadedHead := s.mustLoadHead(storagetest.UnloadDataStorageInterval)

	// Assert
	s.Require().NotNil(s.shards(loadedHead)[0].UnloadedDataStorage())
	s.Require().NotNil(s.shards(loadedHead)[1].UnloadedDataStorage())
	s.True(s.shards(loadedHead)[0].UnloadedDataStorage().IsEmpty())
	s.True(s.shards(loadedHead)[1].UnloadedDataStorage().IsEmpty())
	s.Require().NoError(loadedHead.Close())
}

func (s *HeadLoadSuite) TestLoadWithDataUnloading() {
	// Arrange
	sourceHead := s.mustCreateHead(storagetest.UnloadDataStorageInterval)
	s.appendTimeSeries(sourceHead, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "wal_metric"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
				{Timestamp: 1, Value: 2},
				{Timestamp: 2, Value: 3},
			},
		},
	})
	s.appendTimeSeries(sourceHead, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "wal_metric"),
			Samples: []cppbridge.Sample{
				{Timestamp: 100, Value: 1},
				{Timestamp: 101, Value: 2},
				{Timestamp: 102, Value: 3},
			},
		},
	})

	s.Require().NoError(services.UnloadUnusedSeriesDataWithHead(sourceHead))
	s.Require().NoError(sourceHead.Close())

	// Act
	loadedHead := s.mustLoadHead(storagetest.UnloadDataStorageInterval)

	// Assert
	s.NotNil(s.shards(loadedHead)[0].UnloadedDataStorage())
	s.NotNil(s.shards(loadedHead)[1].UnloadedDataStorage())
	s.False(s.shards(loadedHead)[0].UnloadedDataStorage().IsEmpty())
	s.True(s.shards(loadedHead)[1].UnloadedDataStorage().IsEmpty())
	s.Require().NoError(loadedHead.Close())
}

func (s *HeadLoadSuite) TestErrorDataUnloading() {
	// Arrange
	sourceHead := s.mustCreateHead(storagetest.UnloadDataStorageInterval)
	s.appendTimeSeries(sourceHead, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "wal_metric"),
			Samples: []cppbridge.Sample{
				{Timestamp: 0, Value: 1},
				{Timestamp: 1, Value: 2},
				{Timestamp: 2, Value: 3},
			},
		},
	})
	s.appendTimeSeries(sourceHead, []storagetest.TimeSeries{
		{
			Labels: labels.FromStrings("__name__", "wal_metric"),
			Samples: []cppbridge.Sample{
				{Timestamp: 100, Value: 1},
				{Timestamp: 101, Value: 2},
				{Timestamp: 102, Value: 3},
			},
		},
	})

	s.Require().NoError(services.UnloadUnusedSeriesDataWithHead(sourceHead))
	s.Require().NoError(sourceHead.Close())

	// Act
	s.lockFileForCreation(storage.GetUnloadedDataStorageFilename(s.headDir(), 0))
	s.lockFileForCreation(storage.GetUnloadedDataStorageFilename(s.headDir(), 1))
	loadedHead, err := s.loadHead(storagetest.UnloadDataStorageInterval)

	// Assert
	s.Require().Error(err)
	s.NotNil(s.shards(loadedHead)[0].UnloadedDataStorage())
	s.NotNil(s.shards(loadedHead)[1].UnloadedDataStorage())
	s.Require().NoError(loadedHead.Close())
}

func (s *HeadLoadSuite) TestInvalidEncoderVersion() {
	// Arrange
	// Act
	head, err := s.loadNonWritableHead()
	// Assert
	s.Require().ErrorIs(err, cppbridge.ErrInvalidEncoderVersion)
	s.Require().NoError(head.Close())
}

func (s *HeadLoadSuite) TestLoadWalV2() {
	rec, err := s.catalog.Create(1)
	s.Require().NoError(err)
	headDir := filepath.Join(s.dataDir, rec.Dir())
	s.Require().NoError(os.Mkdir(headDir, 0o777))
	shardFilePath := storage.GetShardWalFilename(headDir, 0)
	shardFile, err := util.CreateFileAppender(shardFilePath, 0o666)
	s.Require().NoError(err)

	shardWalEncoder := cppbridge.NewHeadWalEncoder(0, 0, cppbridge.NewQueryableLssStorage())

	_, err = writer.WriteHeader(shardFile, wal.FileFormatVersionV2, shardWalEncoder.Version())
	s.Require().NoError(err)

	encodedSegment, err := shardWalEncoder.Finalize()
	s.Require().NoError(err)

	encodedSegment.SetSegmentID(rec.NextSegmentID())
	_, err = writer.WriteSegmentV2(shardFile, encodedSegment)
	s.Require().NoError(err)

	s.Require().NoError(shardFile.Close())

	s.Require().Equal(uint16(math.MaxUint16), rec.GetShardBySegmentID(encodedSegment.ID()))

	h, err := storage.NewLoader(
		s.dataDir,
		storagetest.MaxSegmentSize,
		prometheus.DefaultRegisterer,
		storagetest.UnloadDataStorageInterval,
	).Load(rec, 0)
	s.Require().NoError(err)
	s.Require().NoError(h.Close())
	s.Require().Equal(uint16(math.MaxUint16), rec.GetShardBySegmentID(encodedSegment.ID()))
}

type EnsureSameErrorTypesTestSuite struct {
	suite.Suite
}

func TestEnsureSameErrorTypesTestSuite(t *testing.T) {
	suite.Run(t, new(EnsureSameErrorTypesTestSuite))
}

func (s *EnsureSameErrorTypesTestSuite) TestEnsureSameErrorTypesNoErrors() {
	// Arrange
	targetErr := errors.New("target error")
	var errs []error

	// Act
	err := storage.EnsureSameErrorTypes(errs, targetErr)

	// Assert
	s.Require().NoError(err)
}

func (s *EnsureSameErrorTypesTestSuite) TestEnsureSameErrorTypesSingleNonTargetError() {
	// Arrange
	targetErr := errors.New("target error")
	anotherErr := errors.New("another error")
	errs := []error{anotherErr}

	// Act
	err := storage.EnsureSameErrorTypes(errs, targetErr)

	// Assert
	s.Require().Error(err)
	s.Require().ErrorIs(err, anotherErr)
	s.Require().NotErrorIs(err, targetErr)
}

func (s *EnsureSameErrorTypesTestSuite) TestEnsureSameErrorTypesAllErrorsOfTargetType() {
	// Arrange
	targetErr := errors.New("target error")
	errs := []error{fmt.Errorf("err1: %w", targetErr), fmt.Errorf("err2: %w", targetErr)}

	// Act
	err := storage.EnsureSameErrorTypes(errs, targetErr)

	// Assert
	s.Require().Error(err)
	s.Require().ErrorIs(err, targetErr)
}

func (s *EnsureSameErrorTypesTestSuite) TestEnsureSameErrorTypesNotAllErrorsOfTargetType() {
	// Arrange
	targetErr := errors.New("target error")
	anotherErr := errors.New("another error")
	errs := []error{fmt.Errorf("err1: %w", targetErr), fmt.Errorf("err2: %w", anotherErr)}

	// Act
	resultErr := storage.EnsureSameErrorTypes(errs, targetErr)

	// Assert
	s.Require().Error(resultErr)
	s.Require().ErrorIs(resultErr, anotherErr)
	s.Require().NotErrorIs(resultErr, targetErr)
}
