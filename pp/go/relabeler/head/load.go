package head

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/prometheus/prometheus/pp/go/relabeler/logger"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/util/optional"
)

const (
	HeadWalEncoderDecoderLogShards uint8 = 0
)

// Create head.
func Create(
	id string,
	generation uint64,
	dir string,
	configs []*config.InputRelabelerConfig,
	numberOfShards uint16,
	maxSegmentSize uint32,
	lastAppendedSegmentIDSetter LastAppendedSegmentIDSetter,
	registerer prometheus.Registerer,
) (_ *Head, err error) {
	lsses := make([]*LSS, numberOfShards)
	wals := make([]*ShardWal, numberOfShards)
	dataStorages := make([]*DataStorage, numberOfShards)
	unloadedDataStorages := make([]*UnloadedDataStorage, numberOfShards)
	queriedSeriesStorages := make([]*QueriedSeriesStorage, numberOfShards)

	defer func() {
		if err == nil {
			return
		}
		for _, wal := range wals {
			if wal != nil {
				_ = wal.Close()
			}
		}
	}()

	swn := newSegmentWriteNotifier(numberOfShards, lastAppendedSegmentIDSetter)

	for shardID := uint16(0); shardID < numberOfShards; shardID++ {
		lsses[shardID], wals[shardID], dataStorages[shardID], unloadedDataStorages[shardID], queriedSeriesStorages[shardID], err = createShard(dir, shardID, swn, maxSegmentSize)
		if err != nil {
			return nil, fmt.Errorf("failed to create shard: %w", err)
		}
	}

	return New(
		id,
		generation,
		configs,
		lsses,
		wals,
		dataStorages,
		unloadedDataStorages,
		queriedSeriesStorages,
		numberOfShards,
		registerer,
	)
}

func getShardWalFilename(dir string, shardID uint16) string {
	return filepath.Join(dir, fmt.Sprintf("shard_%d.wal", shardID))
}

func getUnloadedDataStorageFilename(dir string, shardID uint16) string {
	return filepath.Join(dir, fmt.Sprintf("unloaded_%d.ds", shardID))
}

func getQueriedSeriesStorageFilename(dir string, shardID uint16, index uint8) string {
	return filepath.Join(dir, fmt.Sprintf("queried_series_%d_%d.ds", shardID, index))
}

// createShard create shard for head.
func createShard(
	dir string,
	shardID uint16,
	swn *segmentWriteNotifier,
	maxSegmentSize uint32,
) (*LSS, *ShardWal, *DataStorage, *UnloadedDataStorage, *QueriedSeriesStorage, error) {
	dir = filepath.Clean(dir)

	var unloadedDataStorage *UnloadedDataStorage

	shardFile, err := os.Create(getShardWalFilename(dir, shardID))
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to create shard wal file: %w", err)
	}

	defer func() {
		if err == nil {
			return
		}

		_ = shardFile.Close()

		if unloadedDataStorage != nil {
			_ = unloadedDataStorage.Close()
		}
	}()

	lss := &LSS{
		input:  cppbridge.NewLssStorage(),
		target: cppbridge.NewQueryableLssStorage(),
	}

	shardWalEncoder := cppbridge.NewHeadWalEncoder(shardID, HeadWalEncoderDecoderLogShards, lss.target)
	_, err = WriteHeader(shardFile, FileFormatVersion, shardWalEncoder.Version())
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to write header: %w", err)
	}

	sw, err := newSegmentWriter(shardID, shardFile, swn)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to init segmentWriter: %w", err)
	}

	unloadedDataStorage, err = createUnloadedDataStorage(dir, shardID)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to create unloaded data storage file: %w", err)
	}

	var queriedSeriesStorageFile1, queriedSeriesStorageFile2 *os.File
	if queriedSeriesStorageFile1, queriedSeriesStorageFile2, err = openQueriedSeriesStorageFiles(dir, shardID); err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to create queried data storage writer: %w", err)
	}

	return lss,
		newShardWal(shardWalEncoder, maxSegmentSize, sw),
		NewDataStorage(),
		unloadedDataStorage,
		NewQueriedSeriesStorage(queriedSeriesStorageFile1, queriedSeriesStorageFile2),
		nil
}

func createUnloadedDataStorage(dir string, shardID uint16) (*UnloadedDataStorage, error) {
	unloadedDataStorageFile, err := os.Create(getUnloadedDataStorageFilename(dir, shardID))
	if err != nil {
		return nil, err
	}

	unloadedDataStorage, err := NewUnloadedDataStorage(unloadedDataStorageFile)
	if err != nil {
		return unloadedDataStorage, err
	}

	return unloadedDataStorage, nil
}

func openQueriedSeriesStorageFiles(dir string, shardID uint16) (*os.File, *os.File, error) {
	file1, err := os.OpenFile(getQueriedSeriesStorageFilename(dir, shardID, 0), os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create queried series storage file: %w", err)
	}

	file2, err := os.OpenFile(getQueriedSeriesStorageFilename(dir, shardID, 1), os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		_ = file1.Close()
		return nil, nil, fmt.Errorf("failed to create queried series storage file: %w", err)
	}

	return file1, file2, nil
}

func filesAreEmpty(files ...*os.File) bool {
	for _, file := range files {
		if info, err := file.Stat(); err == nil {
			if info.Size() > 0 {
				return false
			}
		}
	}

	return true
}

func Load(
	id string,
	generation uint64,
	dir string,
	configs []*config.InputRelabelerConfig,
	numberOfShards uint16,
	maxSegmentSize uint32,
	lastAppendedSegmentIDSetter LastAppendedSegmentIDSetter,
	registerer prometheus.Registerer,
	unloadDataStorageInterval time.Duration,
) (_ *Head, corrupted bool, numberOfSegments uint32, err error) {
	shardLoadResults := make([]ShardLoadResult, numberOfShards)
	wg := &sync.WaitGroup{}
	swn := newSegmentWriteNotifier(numberOfShards, lastAppendedSegmentIDSetter)
	for shardID := uint16(0); shardID < numberOfShards; shardID++ {
		wg.Add(1)
		go func(shardID uint16, dir string, notifier *segmentWriteNotifier) {
			defer wg.Done()
			var err error
			shardLoadResults[shardID], err = NewShardLoader(
				shardID,
				dir,
				maxSegmentSize,
				notifier,
				unloadDataStorageInterval).Load()
			if err != nil {
				logger.Warnf("load shard error: %v", err)
			}
		}(shardID, dir, swn)
	}
	wg.Wait()

	lsses := make([]*LSS, numberOfShards)
	wals := make([]*ShardWal, numberOfShards)
	dataStorages := make([]*DataStorage, numberOfShards)
	unloadedDataStorages := make([]*UnloadedDataStorage, numberOfShards)
	queriedSeriesStorages := make([]*QueriedSeriesStorage, numberOfShards)
	numberOfSegmentsRead := optional.Optional[uint32]{}

	for shardID, shardLoadResult := range shardLoadResults {
		lsses[shardID] = shardLoadResult.Lss
		wals[shardID] = shardLoadResult.Wal
		dataStorages[shardID] = shardLoadResult.DataStorage
		unloadedDataStorages[shardID] = shardLoadResult.UnloadedDataStorage
		queriedSeriesStorages[shardID] = shardLoadResult.QueriedSeriesStorage
		if shardLoadResult.Corrupted {
			corrupted = true
		}
		if numberOfSegmentsRead.IsNil() {
			numberOfSegmentsRead.Set(shardLoadResult.NumberOfSegments)
		} else if numberOfSegmentsRead.Value() != shardLoadResult.NumberOfSegments {
			corrupted = true
			// calculating maximum number of segments (critical for remote write).
			if numberOfSegmentsRead.Value() < shardLoadResult.NumberOfSegments {
				numberOfSegmentsRead.Set(shardLoadResult.NumberOfSegments)
			}
		}
	}

	defer func() {
		if err == nil {
			return
		}
		for _, wal := range wals {
			if wal != nil {
				_ = wal.Close()
			}
		}
	}()

	h, err := New(
		id,
		generation,
		configs,
		lsses,
		wals,
		dataStorages,
		unloadedDataStorages,
		queriedSeriesStorages,
		numberOfShards,
		registerer,
	)
	if err != nil {
		return nil, corrupted, numberOfSegmentsRead.Value(), fmt.Errorf("failed to create head: %w", err)
	}

	h.MergeOutOfOrderChunks()

	return h, corrupted, numberOfSegmentsRead.Value(), nil
}

type ShardLoader struct {
	shardID                   uint16
	dir                       string
	maxSegmentSize            uint32
	notifier                  *segmentWriteNotifier
	unloadDataStorageInterval time.Duration
}

func NewShardLoader(
	shardID uint16,
	dir string,
	maxSegmentSize uint32,
	notifier *segmentWriteNotifier,
	unloadDataStorageInterval time.Duration,
) *ShardLoader {
	return &ShardLoader{
		shardID:                   shardID,
		dir:                       dir,
		maxSegmentSize:            maxSegmentSize,
		notifier:                  notifier,
		unloadDataStorageInterval: unloadDataStorageInterval,
	}
}

type ShardLoadResult struct {
	Lss                  *LSS
	DataStorage          *DataStorage
	Wal                  *ShardWal
	UnloadedDataStorage  *UnloadedDataStorage
	QueriedSeriesStorage *QueriedSeriesStorage
	NumberOfSegments     uint32
	Corrupted            bool
}

func (l *ShardLoader) Load() (ShardLoadResult, error) {
	result := ShardLoadResult{
		Lss: &LSS{
			input:  cppbridge.NewLssStorage(),
			target: cppbridge.NewQueryableLssStorage(),
		},
		DataStorage: NewDataStorage(),
		Wal:         newCorruptedShardWal(),
		Corrupted:   true,
	}

	shardWalFile, err := os.OpenFile(getShardWalFilename(l.dir, l.shardID), os.O_RDWR, 0666)
	if err != nil {
		return result, err
	}

	defer func() {
		if result.Corrupted {
			_ = shardWalFile.Close()

			if result.UnloadedDataStorage != nil {
				_ = result.UnloadedDataStorage.Close()
				result.UnloadedDataStorage = nil
			}

			if result.QueriedSeriesStorage != nil {
				_ = result.QueriedSeriesStorage.Close()
				result.QueriedSeriesStorage = nil
			}
		}
	}()

	result.UnloadedDataStorage, err = createUnloadedDataStorage(l.dir, l.shardID)
	if err != nil {
		return result, err
	}

	var queriedSeriesStorageIsEmpty bool
	queriedSeriesStorageIsEmpty, err = l.loadQueriedSeries(&result)
	if err != nil {
		return result, err
	}

	decoder, err := l.loadWalFile(bufio.NewReaderSize(shardWalFile, 1024*1024*4), !queriedSeriesStorageIsEmpty, &result)
	if err != nil {
		return result, err
	}

	if err = l.createShardWal(shardWalFile, decoder, &result); err != nil {
		return result, err
	}

	result.Corrupted = false
	return result, nil
}

func (l *ShardLoader) loadWalFile(reader io.Reader, unloadUnusedData bool, result *ShardLoadResult) (*cppbridge.HeadWalDecoder, error) {
	_, encoderVersion, _, err := ReadHeader(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read wal header: %w", err)
	}

	decoder := cppbridge.NewHeadWalDecoder(result.Lss.target, encoderVersion)
	result.NumberOfSegments, err = l.loadSegments(
		reader,
		decoder,
		result.DataStorage.encoder,
		newDataUnloader(result.DataStorage, result.UnloadedDataStorage, unloadUnusedData, l.unloadDataStorageInterval),
	)
	return decoder, err
}

func (l *ShardLoader) createShardWal(shardWalFile *os.File, walDecoder *cppbridge.HeadWalDecoder, result *ShardLoadResult) error {
	if sw, err := newSegmentWriter(l.shardID, shardWalFile, l.notifier); err != nil {
		return err
	} else {
		l.notifier.Set(l.shardID, result.NumberOfSegments)
		result.Wal = newShardWal(walDecoder.CreateEncoder(), l.maxSegmentSize, sw)
		return nil
	}
}

type dataUnloader struct {
	unloader              *cppbridge.UnusedSeriesDataUnloader
	unloadedDataStorage   *UnloadedDataStorage
	unloadedIntervalIndex int64
	unloadInterval        time.Duration
	needUnload            bool
}

func newDataUnloader(
	dataStorage *DataStorage,
	unloadedDataStorage *UnloadedDataStorage,
	unloadUnusedData bool,
	unloadInterval time.Duration,
) dataUnloader {
	result := dataUnloader{
		unloadedDataStorage:   unloadedDataStorage,
		unloadedIntervalIndex: math.MinInt64,
		needUnload:            unloadUnusedData && unloadInterval > 0,
		unloadInterval:        unloadInterval,
	}

	if result.needUnload {
		result.unloader = dataStorage.CreateUnusedSeriesDataUnloader()
	}

	return result
}

func (d *dataUnloader) UnloadIfNeeded(createTs, encodeTs time.Duration) error {
	if !d.needUnload {
		return nil
	}

	intervalIndex := int64(createTs / d.unloadInterval)

	if d.unloadedIntervalIndex == math.MinInt64 {
		d.unloadedIntervalIndex = intervalIndex

		createTs = encodeTs
		intervalIndex = int64(createTs / d.unloadInterval)
	}

	if intervalIndex > d.unloadedIntervalIndex {
		logger.Warnf("unloading data: prev %d, current: %d, unloadInterval: %d", d.unloadedIntervalIndex, intervalIndex, d.unloadInterval)

		if header, err := d.unloadedDataStorage.WriteSnapshot(d.unloader.CreateSnapshot()); err != nil {
			return fmt.Errorf("failed to write unloaded data: %w", err)
		} else {
			d.unloadedDataStorage.WriteIndex(header)
		}
		d.unloader.Unload()

		d.unloadedIntervalIndex = intervalIndex
	}

	return nil
}

func (l *ShardLoader) loadSegments(
	reader io.Reader,
	walDecoder *cppbridge.HeadWalDecoder,
	encoder *cppbridge.HeadEncoder,
	unloader dataUnloader,
) (uint32, error) {
	numberOfSegments := uint32(0)

	for {
		segment, _, err := ReadSegment(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return numberOfSegments, nil
			}
			return 0, fmt.Errorf("failed to read segment: %w", err)
		}

		createTs, encodeTs, err := walDecoder.DecodeToDataStorage(segment.data, encoder)
		if err != nil {
			return 0, fmt.Errorf("failed to decode segment: %w", err)
		}

		numberOfSegments++

		if createTs != 0 {
			if err = unloader.UnloadIfNeeded(time.Duration(createTs), time.Duration(encodeTs)); err != nil {
				return 0, fmt.Errorf("failed to unload data: %w", err)
			}
		}
	}
}

func (l *ShardLoader) loadQueriedSeries(result *ShardLoadResult) (bool, error) {
	file1, file2, err := openQueriedSeriesStorageFiles(l.dir, l.shardID)
	if err != nil {
		return false, err
	}

	isEmptyStorage := filesAreEmpty(file1, file2)

	result.QueriedSeriesStorage = NewQueriedSeriesStorage(file1, file2)

	if queriedSeries, err := result.QueriedSeriesStorage.Read(); err != nil {
		logger.Warnf("error loading queried series: %v", err)
	} else {
		result.DataStorage.dataStorage.SetQueriedSeriesBitset(queriedSeries)
	}

	return isEmptyStorage, nil
}
