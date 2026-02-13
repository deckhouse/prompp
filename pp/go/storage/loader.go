package storage

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

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/head/services"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/reader"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/writer"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/prometheus/pp/go/util/optional"
)

// ErrNonContinuableHead error when head is not continuable.
var ErrNonContinuableHead = errors.New("head is not continuable")

// Loader loads [Head] or [shard.Shard] from [Wal].
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

//revive:disable-next-line:flag-parameter this is not a flag, but a parameter
func (l *Loader) loadHead(
	headRecord *catalog.Record,
	generation uint64,
	readOnly bool,
) (*Head, error) {
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
				headRecord,
				l.registerer,
				l.unloadDataStorageInterval,
				readOnly,
			)
		}(shardID)
	}
	wg.Wait()

	shards := make([]*shard.Shard, numberOfShards)
	numberOfSegmentsRead := optional.Optional[uint32]{}
	errs := make([]error, numberOfShards)
	for shardID, res := range shardLoadResults {
		shards[shardID] = res.shard
		errs[shardID] = res.err

		if numberOfSegmentsRead.IsNil() {
			numberOfSegmentsRead.Set(res.numberOfSegments)
		} else if numberOfSegmentsRead.Value() != res.numberOfSegments {
			errs = append(errs,
				fmt.Errorf(
					"corrupted shard %d: segment count mismatch, expected: %d, got: %d",
					shardID,
					numberOfSegmentsRead.Value(),
					res.numberOfSegments,
				))
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
		// numberOfSegments here is actual number of segments.
		if numberOfSegmentsRead.Value() > 0 {
			headRecord.SetLastAppendedSegmentID(numberOfSegmentsRead.Value() - 1)
		}

		lastAppendedSegmentID := uint32(0)
		if headRecord.LastAppendedSegmentID() != nil {
			lastAppendedSegmentID = *headRecord.LastAppendedSegmentID()
		}

		logger.Errorf(
			"head: %s number of segments mismatched: last appended=%d, number of segments read=%d",
			headRecord.ID(),
			lastAppendedSegmentID,
			numberOfSegmentsRead.Value(),
		)
	}

	h := head.NewHead(
		headID,
		shards,
		shard.NewPerGoroutineShard[*Wal],
		headRecord.Acquire(),
		generation,
		l.registerer,
	)

	if err := services.MergeOutOfOrderChunksWithHead(h); err != nil {
		errs = append(errs, err)
	}

	if readOnly || errs != nil {
		h.SetReadOnly()
	}

	// we must ensure that if one cppbridge.ErrInvalidEncoderVersion happened, they are all the same type,
	// otherwise it is corruption error.
	err := EnsureSameErrorTypes(errs, cppbridge.ErrInvalidEncoderVersion)

	logger.Debugf(
		"[Loader] loaded head: %s, corrupted: %t",
		headRecord.ID(),
		err != nil && !errors.Is(err, cppbridge.ErrInvalidEncoderVersion),
	)

	return h, err
}

// EnsureSameErrorTypes checks if err contains targetErr - we ensure that all errors in chain would be targetErr,
// otherwise return all errors in chain except targetErr
func EnsureSameErrorTypes(errs []error, targetErr error) error {
	if len(errs) == 0 {
		return nil
	}

	var nonTargetErrs error
	var targetErrs error
	for _, err := range errs {
		if err == nil {
			continue
		}

		if errors.Is(err, targetErr) {
			targetErrs = errors.Join(targetErrs, err)
		} else {
			nonTargetErrs = errors.Join(nonTargetErrs, err)
		}
	}

	if nonTargetErrs != nil {
		return nonTargetErrs
	}

	return targetErrs
}

// Load [Head] from [Wal] by head ID.
// CAUTION: Always returns head, even if err != nil.
//
//revive:disable-next-line:cognitive-complexity // function is not complicated
//revive:disable-next-line:function-length // long but readable.
//revive:disable-next-line:cyclomatic // but readable
func (l *Loader) Load(
	headRecord *catalog.Record,
	generation uint64,
) (*Head, error) {
	return l.loadHead(headRecord, generation, false)
}

// LoadReadOnly [Head] from [Wal] by head ID. Head is read only, and there is corrupted wal in each shard,
// so any append will fail.
// Cannot return ErrInvalidEncoderVersion.
// CAUTION: Always returns head, even if err != nil.
//
//revive:disable-next-line:cognitive-complexity // function is not complicated
//revive:disable-next-line:function-length // long but readable.
//revive:disable-next-line:cyclomatic // but readable
func (l *Loader) LoadReadOnly(
	headRecord *catalog.Record,
	generation uint64,
) (*Head, error) {
	return l.loadHead(headRecord, generation, true)
}

func (*Loader) loadShard(
	shardID uint16,
	dir string,
	maxSegmentSize uint32,
	notifier *writer.SegmentWriteNotifier,
	headRecord *catalog.Record,
	registerer prometheus.Registerer,
	unloadDataStorageInterval time.Duration,
	readOnly bool,
) ShardLoadResult {
	shardDataLoader := NewShardDataLoader(
		shardID,
		dir,
		maxSegmentSize,
		notifier,
		headRecord,
		registerer,
		unloadDataStorageInterval,
	)
	err := shardDataLoader.Load(readOnly)
	return ShardLoadResult{
		numberOfSegments: shardDataLoader.shardData.numberOfSegments,
		err:              err,
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

// ShardLoadResult the result of loading a shard from a wal file.
type ShardLoadResult struct {
	shard            *shard.Shard
	numberOfSegments uint32
	err              error
}

// ShardData data for creating a shard.
type ShardData struct {
	lss                  *shard.LSS
	dataStorage          *shard.DataStorage
	wal                  *Wal
	unloadedDataStorage  *shard.UnloadedDataStorage
	queriedSeriesStorage *shard.QueriedSeriesStorage
	numberOfSegments     uint32
}

// ShardDataLoader loads shard data from a file and creates a shard.
type ShardDataLoader struct {
	dir                       string
	notifier                  *writer.SegmentWriteNotifier
	headRecord                *catalog.Record
	registerer                prometheus.Registerer
	unloadDataStorageInterval time.Duration
	shardData                 ShardData
	maxSegmentSize            uint32
	shardID                   uint16
}

// NewShardDataLoader init new [ShardDataLoader].
func NewShardDataLoader(
	shardID uint16,
	dir string,
	maxSegmentSize uint32,
	notifier *writer.SegmentWriteNotifier,
	headRecord *catalog.Record,
	registerer prometheus.Registerer,
	unloadDataStorageInterval time.Duration,
) ShardDataLoader {
	return ShardDataLoader{
		dir:                       dir,
		notifier:                  notifier,
		headRecord:                headRecord,
		registerer:                registerer,
		unloadDataStorageInterval: unloadDataStorageInterval,
		maxSegmentSize:            maxSegmentSize,
		shardID:                   shardID,
	}
}

// Load loads shard data from a file and creates a shard.
func (l *ShardDataLoader) Load(readOnly bool) error {
	l.shardData = ShardData{
		lss:         shard.NewLSS(),
		dataStorage: shard.NewDataStorage(),
		wal: wal.NewCorruptedWal[
			*cppbridge.HeadEncodedSegment,
			*writer.Buffered[*cppbridge.HeadEncodedSegment],
		](),
	}

	shardWalFile, err := os.OpenFile( //nolint:gosec // need this permissions
		GetShardWalFilename(l.dir, l.shardID),
		os.O_RDONLY,
		0o666, //revive:disable-line:add-constant // file permissions simple readable as octa-number
	)
	if err != nil {
		return err
	}

	queriedSeriesStorageIsEmpty := true
	if l.unloadDataStorageInterval > 0 {
		l.shardData.unloadedDataStorage = shard.NewUnloadedDataStorage(
			shard.NewAppendFileStorage(GetUnloadedDataStorageFilename(l.dir, l.shardID)),
		)
		queriedSeriesStorageIsEmpty, _ = l.loadQueriedSeries()
	}

	shardWalFileName := shardWalFile.Name()
	decoder, err := l.loadWalFile(bufio.NewReaderSize(shardWalFile, 1024*1024*10), queriedSeriesStorageIsEmpty)
	_ = shardWalFile.Close()
	if err != nil {
		return err
	}

	if readOnly {
		return nil
	}

	encoder, err := decoder.CreateEncoder()
	if err != nil {
		return err
	}

	return l.createShardWal(shardWalFileName, encoder)
}

// loadWalFile loads and decode wal file.
//
//revive:disable-next-line:flag-parameter this is a flag, but it's more convenient this way
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

// createShardWal creates a wal for a shard.
func (l *ShardDataLoader) createShardWal(
	fileName string,
	walEncoder *cppbridge.HeadWalEncoder,
) error {
	//revive:disable-next-line:add-constant // file permissions simple readable as octa-number
	shardWalFile, err := util.OpenFileAppender(fileName, 0o666)
	if err != nil {
		return err
	}

	sw, err := writer.NewBuffered(
		l.shardID,
		shardWalFile,
		writer.WriteSegment[*cppbridge.HeadEncodedSegment],
		l.notifier,
		l.headRecord,
	)
	if err != nil {
		_ = shardWalFile.Close()
		return err
	}

	l.notifier.Set(l.shardID, l.shardData.numberOfSegments)
	l.shardData.wal = wal.NewWal(walEncoder, sw, l.maxSegmentSize, l.shardID, l.registerer)

	return nil
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
		header, err := d.unloadedDataStorage.WriteSnapshot(d.unloader.CreateSnapshot())
		if err != nil {
			return fmt.Errorf("failed to write unloaded data: %w", err)
		}

		d.unloadedDataStorage.WriteIndex(header)
		d.unloader.Unload()
		d.unloadedIntervalIndex = intervalIndex
	}

	return nil
}

// loadSegments loads and decode segments from wal file.
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
		l.headRecord.SetSegmentIDByShard(0, l.shardID)

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
	} else if !l.shardData.dataStorage.SetQueriedSeriesBitset(queriedSeries) {
		logger.Warnf("error set queried series in storage: %v", err)
	}

	return false, nil
}

// GetShardWalFilename returns shard's Wal file name.
func GetShardWalFilename(dir string, shardID uint16) string {
	return filepath.Join(dir, fmt.Sprintf("shard_%d.wal", shardID))
}

// GetUnloadedDataStorageFilename returns unloaded DataStorage file name.
func GetUnloadedDataStorageFilename(dir string, shardID uint16) string {
	return filepath.Join(dir, fmt.Sprintf("unloaded_%d.ds", shardID))
}

// GetQueriedSeriesStorageFilename returns queried series storage file name.
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
