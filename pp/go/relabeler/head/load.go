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

type FileStorage struct {
	fileName string
	file     *os.File
}

func NewFileStorage(fileName string) *FileStorage {
	return &FileStorage{fileName: fileName}
}

func (q *FileStorage) ReadAt(p []byte, off int64) (n int, err error) {
	return q.file.ReadAt(p, off)
}

func (q *FileStorage) Open(flags int) (err error) {
	if q.file == nil {
		q.file, err = os.OpenFile(q.fileName, flags, 0666)
	}

	return
}

func (q *FileStorage) Write(p []byte) (n int, err error) {
	return q.file.Write(p)
}

func (q *FileStorage) Close() error {
	if q.file != nil {
		return q.file.Close()
	}

	return nil
}

func (q *FileStorage) Read(p []byte) (n int, err error) {
	return q.file.Read(p)
}

func (q *FileStorage) Seek(offset int64, whence int) (int64, error) {
	return q.file.Seek(offset, whence)
}

func (q *FileStorage) Sync() error {
	return q.file.Sync()
}

func (q *FileStorage) Truncate(size int64) error {
	return q.file.Truncate(size)
}

func (q *FileStorage) IsEmpty() bool {
	if q.file != nil {
		if info, err := q.file.Stat(); err == nil {
			return info.Size() == 0
		}
	}

	return true
}

type AppendFileStorage struct {
	fileName string
	file     *os.File
}

func NewAppendFileStorage(fileName string) *AppendFileStorage {
	return &AppendFileStorage{fileName: fileName}
}

func (q *AppendFileStorage) Open(flags int) (err error) {
	if q.file == nil {
		q.file, err = os.OpenFile(q.fileName, flags, 0666)
	}

	return
}

func (q *AppendFileStorage) Write(p []byte) (n int, err error) {
	return q.file.Write(p)
}

func (q *AppendFileStorage) Close() error {
	if q.file != nil {
		return q.file.Close()
	}

	return nil
}

func (q *AppendFileStorage) Reader() (StorageReader, error) {
	return os.Open(q.fileName)
}

func (q *AppendFileStorage) Seek(offset int64, whence int) (int64, error) {
	return q.file.Seek(offset, whence)
}

func (q *AppendFileStorage) Sync() error {
	return q.file.Sync()
}

func (q *AppendFileStorage) Truncate(size int64) error {
	return q.file.Truncate(size)
}

func (q *AppendFileStorage) IsEmpty() bool {
	if q.file != nil {
		if info, err := q.file.Stat(); err == nil {
			return info.Size() == 0
		}
	}

	return true
}

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
	unloadDataStorageInterval time.Duration,
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
		lsses[shardID], wals[shardID], dataStorages[shardID], unloadedDataStorages[shardID], queriedSeriesStorages[shardID], err = createShard(dir, shardID, swn, maxSegmentSize, unloadDataStorageInterval)
		if err != nil {
			return nil, fmt.Errorf("failed to create shard: %w", err)
		}
	}

	return New(
		id,
		dir,
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
	unloadDataStorageInterval time.Duration,
) (*LSS, *ShardWal, *DataStorage, *UnloadedDataStorage, *QueriedSeriesStorage, error) {
	dir = filepath.Clean(dir)

	shardFile, err := os.OpenFile(getShardWalFilename(dir, shardID), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to create shard wal file: %w", err)
	}

	defer func() {
		if err != nil {
			_ = shardFile.Close()
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

	var unloadedDataStorage *UnloadedDataStorage
	var queriedSeriesStorage *QueriedSeriesStorage
	if unloadDataStorageInterval != 0 {
		unloadedDataStorage = NewUnloadedDataStorage(NewAppendFileStorage(getUnloadedDataStorageFilename(dir, shardID)))
		queriedSeriesStorage = NewQueriedSeriesStorage(
			NewFileStorage(getQueriedSeriesStorageFilename(dir, shardID, 0)),
			NewFileStorage(getQueriedSeriesStorageFilename(dir, shardID, 1)),
		)
	}

	return lss,
		newShardWal(shardWalEncoder, maxSegmentSize, sw),
		NewDataStorage(),
		unloadedDataStorage,
		queriedSeriesStorage,
		nil
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
		dir,
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
		_ = shardWalFile.Close()
	}()

	queriedSeriesStorageIsEmpty := true
	if l.unloadDataStorageInterval > 0 {
		result.UnloadedDataStorage = NewUnloadedDataStorage(NewAppendFileStorage(getUnloadedDataStorageFilename(l.dir, l.shardID)))
		queriedSeriesStorageIsEmpty, _ = l.loadQueriedSeries(&result)
	}

	decoder, err := l.loadWalFile(bufio.NewReaderSize(shardWalFile, 1024*1024*4), queriedSeriesStorageIsEmpty, &result)
	if err != nil {
		return result, err
	}

	f, err := os.OpenFile(shardWalFile.Name(), os.O_WRONLY, 0666)
	if err != nil {
		return result, err
	}
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return result, errors.Join(err, f.Close())
	}

	if err = l.createShardWal(f, decoder, &result); err != nil {
		return result, errors.Join(err, f.Close())
	}

	result.Corrupted = false
	return result, nil
}

func (l *ShardLoader) loadWalFile(
	reader io.Reader,
	queriedSeriesStorageIsEmpty bool,
	result *ShardLoadResult,
) (*cppbridge.HeadWalDecoder, error) {
	_, encoderVersion, _, err := ReadHeader(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read wal header: %w", err)
	}

	var unloader *dataUnloader
	if !queriedSeriesStorageIsEmpty {
		unloader = &dataUnloader{
			unloadedDataStorage:   result.UnloadedDataStorage,
			unloadedIntervalIndex: math.MinInt64,
			unloadInterval:        l.unloadDataStorageInterval,
			unloader:              result.DataStorage.CreateUnusedSeriesDataUnloader(),
		}
	}

	decoder := cppbridge.NewHeadWalDecoder(result.Lss.target, encoderVersion)
	result.NumberOfSegments, err = l.loadSegments(
		reader,
		decoder,
		result.DataStorage.encoder,
		unloader,
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
}

func (d *dataUnloader) Unload(createTs, encodeTs time.Duration) error {
	intervalIndex := int64(createTs / d.unloadInterval)

	if d.unloadedIntervalIndex == math.MinInt64 {
		d.unloadedIntervalIndex = intervalIndex

		createTs = encodeTs
		intervalIndex = int64(createTs / d.unloadInterval)
	}

	if intervalIndex > d.unloadedIntervalIndex {
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
	unloader *dataUnloader,
) (uint32, error) {
	numberOfSegments := uint32(0)

	var segment DecodedSegment
	for {
		_, err := ReadSegment(reader, &segment)
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

		if createTs != 0 && unloader != nil {
			if err = unloader.Unload(time.Duration(createTs), time.Duration(encodeTs)); err != nil {
				return 0, fmt.Errorf("failed to unload data: %w", err)
			}
		}
	}
}

func (l *ShardLoader) loadQueriedSeries(result *ShardLoadResult) (bool, error) {
	file1 := NewFileStorage(getQueriedSeriesStorageFilename(l.dir, l.shardID, 0))
	file2 := NewFileStorage(getQueriedSeriesStorageFilename(l.dir, l.shardID, 1))

	result.QueriedSeriesStorage = NewQueriedSeriesStorage(file1, file2)

	if queriedSeries, err := result.QueriedSeriesStorage.Read(); err != nil {
		if file1.IsEmpty() && file2.IsEmpty() {
			return true, nil
		}

		logger.Warnf("error loading queried series: %v", err)
	} else {
		if !result.DataStorage.dataStorage.SetQueriedSeriesBitset(queriedSeries) {
			logger.Warnf("error set queried series in storage: %v", err)
		}
	}

	return false, nil
}
