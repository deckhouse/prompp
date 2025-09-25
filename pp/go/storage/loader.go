package storage

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/head/services"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/reader"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/writer"
	"github.com/prometheus/prometheus/pp/go/storage/logger"
	"github.com/prometheus/prometheus/pp/go/util/optional"
)

// Loader loads [HeadOnDisk] or [ShardOnDisk] from [WalOnDisk].
type Loader struct {
	dataDir                   string
	maxSegmentSize            uint32
	registerer                prometheus.Registerer
	unloadDataStorageInterval time.Duration
}

// NewLoader init new [Loader].
func NewLoader(
	dataDir string,
	maxSegmentSize uint32,
	registerer prometheus.Registerer,
	unloadDataStorageInterval time.Duration,
) *Loader {
	return &Loader{
		dataDir:                   dataDir,
		maxSegmentSize:            maxSegmentSize,
		registerer:                registerer,
		unloadDataStorageInterval: unloadDataStorageInterval,
	}
}

// Load [HeadOnDisk] from [WalOnDisk] by head ID.
func (l *Loader) Load(
	headRecord *catalog.Record,
	generation uint64,
) (_ *HeadOnDisk, corrupted bool) {
	headID := headRecord.ID()
	headDir := filepath.Join(l.dataDir, headID)
	numberOfShards := headRecord.NumberOfShards()
	shardLoadResults := make([]ShardLoadResult, numberOfShards)

	wg := &sync.WaitGroup{}
	swn := writer.NewSegmentWriteNotifier(numberOfShards, headRecord.SetLastAppendedSegmentID)
	for shardID := range numberOfShards {
		wg.Add(1)
		go func(shardID uint16) {
			defer wg.Done()
			shardLoadResults[shardID] = l.loadShard(
				shardID,
				headDir,
				l.maxSegmentSize,
				swn,
				l.unloadDataStorageInterval,
			)
		}(shardID)
	}
	wg.Wait()

	shards := make([]*ShardOnDisk, numberOfShards)
	numberOfSegmentsRead := optional.Optional[uint32]{}
	for shardID, res := range shardLoadResults {
		shards[shardID] = res.shard
		if res.corrupted {
			corrupted = true
		}

		if numberOfSegmentsRead.IsNil() {
			numberOfSegmentsRead.Set(res.numberOfSegments)
		} else if numberOfSegmentsRead.Value() != res.numberOfSegments {
			corrupted = true
			// calculating maximum number of segments (critical for remote write).
			if numberOfSegmentsRead.Value() < res.numberOfSegments {
				numberOfSegmentsRead.Set(res.numberOfSegments)
			}
		}
	}

	switch {
	case headRecord.Status() == catalog.StatusActive:
		// numberOfSegments here is actual number of segments.
		if numberOfSegmentsRead.Value() > 0 {
			headRecord.SetLastAppendedSegmentID(numberOfSegmentsRead.Value() - 1)
		}
	case isNumberOfSegmentsMismatched(headRecord, numberOfSegmentsRead.Value()):
		corrupted = true
		// numberOfSegments here is actual number of segments.
		if numberOfSegmentsRead.Value() > 0 {
			headRecord.SetLastAppendedSegmentID(numberOfSegmentsRead.Value() - 1)
		}
		logger.Errorf("head: %s number of segments mismatched", headRecord.ID())
	}

	h := head.NewHead(
		headID,
		shards,
		shard.NewPerGoroutineShard[*WalOnDisk],
		headRecord.Acquire(),
		generation,
		l.registerer,
	)

	if err := services.MergeOutOfOrderChunksWithHead(h); err != nil {
		corrupted = true
	}

	return h, corrupted
}

func (l *Loader) loadShard(
	shardID uint16,
	dir string,
	maxSegmentSize uint32,
	notifier *writer.SegmentWriteNotifier,
	unloadDataStorageInterval time.Duration,
) ShardLoadResult {
	shardDataLoader := NewShardDataLoader(shardID, dir, maxSegmentSize, notifier, unloadDataStorageInterval)
	err := shardDataLoader.Load()
	return ShardLoadResult{
		corrupted:        err != nil,
		numberOfSegments: shardDataLoader.shardData.numberOfSegments,
		shard: shard.NewShard(
			shardDataLoader.shardData.lss,
			shardDataLoader.shardData.dataStorage,
			shardDataLoader.shardData.unloadedDataStorage,
			shardDataLoader.shardData.queriedSeriesStorage,
			shardDataLoader.shardData.wal,
			shardID,
		),
	}
}

type ShardLoadResult struct {
	shard            *ShardOnDisk
	numberOfSegments uint32
	corrupted        bool
}

type ShardData struct {
	notifier             *writer.SegmentWriteNotifier
	lss                  *shard.LSS
	dataStorage          *shard.DataStorage
	wal                  *WalOnDisk
	unloadedDataStorage  *shard.UnloadedDataStorage
	queriedSeriesStorage *shard.QueriedSeriesStorage
	numberOfSegments     uint32
}

type ShardDataLoader struct {
	shardID                   uint16
	dir                       string
	maxSegmentSize            uint32
	shardData                 ShardData
	notifier                  *writer.SegmentWriteNotifier
	unloadDataStorageInterval time.Duration
}

func NewShardDataLoader(
	shardID uint16,
	dir string,
	maxSegmentSize uint32,
	notifier *writer.SegmentWriteNotifier,
	unloadDataStorageInterval time.Duration,
) ShardDataLoader {
	return ShardDataLoader{
		shardID:                   shardID,
		dir:                       dir,
		maxSegmentSize:            maxSegmentSize,
		notifier:                  notifier,
		unloadDataStorageInterval: unloadDataStorageInterval,
	}
}

func (l *ShardDataLoader) Load() (err error) {
	l.shardData = ShardData{
		lss:         shard.NewLSS(),
		dataStorage: shard.NewDataStorage(),
		wal: wal.NewCorruptedWal[
			*cppbridge.EncodedSegment,
			cppbridge.WALEncoderStats,
			*writer.Buffered[*cppbridge.EncodedSegment],
		](),
	}

	shardWalFile, err := os.OpenFile(GetShardWalFilename(l.dir, l.shardID), os.O_RDWR, 0o666)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			_ = shardWalFile.Close()
		}
	}()

	queriedSeriesStorageIsEmpty := true
	if l.unloadDataStorageInterval > 0 {
		l.shardData.unloadedDataStorage = shard.NewUnloadedDataStorage(shard.NewFileStorage(GetUnloadedDataStorageFilename(l.dir, l.shardID)))
		queriedSeriesStorageIsEmpty, _ = l.loadQueriedSeries()
	}

	decoder, err := l.loadWalFile(bufio.NewReaderSize(shardWalFile, 1024*1024*4), queriedSeriesStorageIsEmpty)
	if err != nil {
		return err
	}

	if err = l.createShardWal(shardWalFile, decoder); err != nil {
		return err
	}

	return nil
}

func (l *ShardDataLoader) loadWalFile(
	rd io.Reader,
	queriedSeriesStorageIsEmpty bool,
) (*cppbridge.HeadWalDecoder, error) {
	_, encoderVersion, _, err := reader.ReadHeader(rd)
	if err != nil {
		return nil, fmt.Errorf("failed to read wal header: %w", err)
	}

	var unloader *dataUnloader
	if !queriedSeriesStorageIsEmpty {
		unloader = &dataUnloader{
			unloadedDataStorage:   l.shardData.unloadedDataStorage,
			unloadedIntervalIndex: math.MinInt64,
			unloadInterval:        l.unloadDataStorageInterval,
			unloader:              l.shardData.dataStorage.CreateUnusedSeriesDataUnloader(),
		}
	}

	decoder := cppbridge.NewHeadWalDecoder(l.shardData.lss.Target(), encoderVersion)
	l.shardData.numberOfSegments, err = l.loadSegments(
		rd,
		decoder,
		l.shardData.dataStorage,
		unloader,
	)
	return decoder, err
}

func (l *ShardDataLoader) createShardWal(shardWalFile *os.File, walDecoder *cppbridge.HeadWalDecoder) error {
	if sw, err := writer.NewBuffered(l.shardID, shardWalFile, writer.WriteSegment[*cppbridge.EncodedSegment], l.notifier); err != nil {
		return err
	} else {
		l.notifier.Set(l.shardID, l.shardData.numberOfSegments)
		l.shardData.wal = wal.NewWal(walDecoder.CreateEncoder(), sw, l.maxSegmentSize)
		return nil
	}
}

type dataUnloader struct {
	unloader              *cppbridge.UnusedSeriesDataUnloader
	unloadedDataStorage   *shard.UnloadedDataStorage
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

func (l *ShardDataLoader) loadSegments(
	rd io.Reader,
	walDecoder *cppbridge.HeadWalDecoder,
	dataStorage *shard.DataStorage,
	unloader *dataUnloader,
) (uint32, error) {
	numberOfSegments := uint32(0)

	if err := wal.NewSegmentWalReader(rd, reader.NewSegment).ForEachSegment(func(segment *reader.Segment) error {
		createTs, encodeTs, decodeErr := dataStorage.DecodeSegment(walDecoder, segment.Bytes())
		if decodeErr != nil {
			return fmt.Errorf("failed to decode segment: %w", decodeErr)
		}

		numberOfSegments++

		if createTs != 0 && unloader != nil {
			if err := unloader.Unload(time.Duration(createTs), time.Duration(encodeTs)); err != nil {
				return fmt.Errorf("failed to unload data: %w", err)
			}
		}

		return nil
	}); err != nil {
		logger.Debugf(err.Error())
		return 0, err
	}

	return numberOfSegments, nil
}

func (l *ShardDataLoader) loadQueriedSeries() (bool, error) {
	file1 := shard.NewFileStorage(GetQueriedSeriesStorageFilename(l.dir, l.shardID, 0))
	file2 := shard.NewFileStorage(GetQueriedSeriesStorageFilename(l.dir, l.shardID, 1))

	l.shardData.queriedSeriesStorage = shard.NewQueriedSeriesStorage(file1, file2)

	if queriedSeries, err := l.shardData.queriedSeriesStorage.Read(); err != nil {
		if file1.IsEmpty() && file2.IsEmpty() {
			return true, nil
		}

		logger.Warnf("error loading queried series: %v", err)
	} else {
		if !l.shardData.dataStorage.SetQueriedSeriesBitset(queriedSeries) {
			logger.Warnf("error set queried series in storage: %v", err)
		}
	}

	return false, nil
}

func GetShardWalFilename(dir string, shardID uint16) string {
	return filepath.Join(dir, fmt.Sprintf("shard_%d.wal", shardID))
}

func GetUnloadedDataStorageFilename(dir string, shardID uint16) string {
	return filepath.Join(dir, fmt.Sprintf("unloaded_%d.ds", shardID))
}

func GetQueriedSeriesStorageFilename(dir string, shardID uint16, index uint8) string {
	return filepath.Join(dir, fmt.Sprintf("queried_series_%d_%d.ds", shardID, index))
}

// isNumberOfSegmentsMismatched check number of segments loaded and last appended to record.
func isNumberOfSegmentsMismatched(record *catalog.Record, loadedSegments uint32) bool {
	if record.LastAppendedSegmentID() == nil {
		return loadedSegments != 0
	}

	return *record.LastAppendedSegmentID()+1 != loadedSegments
}
