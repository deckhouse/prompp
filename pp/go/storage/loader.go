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
		headRecord.SetLastSegmentID(res.maxSegmentID)

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

	switch checkWalVersion(shardLoadResults) {
	case wal.FileFormatVersion:
		setLastAppendedSegmentID(headRecord, numberOfSegmentsRead)
	case wal.FileFormatVersionV2:
		if headRecord.IsMissingSegmentsByShard() {
			errs = append(errs, fmt.Errorf("missing segments by shard"))
		}
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
		shard: shard.NewShard(
			shardDataLoader.shardData.lss,
			shardDataLoader.shardData.dataStorage,
			shardDataLoader.shardData.unloadedDataStorage,
			shardDataLoader.shardData.queriedSeriesStorage,
			shardDataLoader.shardData.wal,
			shardID,
		),
		numberOfSegments: shardDataLoader.shardData.numberOfSegments,
		maxSegmentID:     shardDataLoader.shardData.maxSegmentID,
		err:              err,
		walVersion:       shardDataLoader.shardData.walVersion,
	}
}

// ShardLoadResult the result of loading a shard from a wal file.
type ShardLoadResult struct {
	shard            *shard.Shard
	numberOfSegments uint32
	// maximum through ID of a segment read from WAL
	maxSegmentID uint32
	err          error
	walVersion   uint8
}

// ShardData data for creating a shard.
type ShardData struct {
	lss                  *shard.LSS
	dataStorage          *shard.DataStorage
	wal                  *Wal
	unloadedDataStorage  *shard.UnloadedDataStorage
	queriedSeriesStorage *shard.QueriedSeriesStorage
	writeSegment         func(io.Writer, *cppbridge.HeadEncodedSegment) (int, error)
	numberOfSegments     uint32
	maxSegmentID         uint32
	walVersion           uint8
}

//
// SegmentWriteNotifier
//

// SegmentWriteNotifier notifies that the segment has been written.
type SegmentWriteNotifier interface {
	// NotifySegmentIsWritten notify that the segment has been flushed for shard.
	NotifySegmentIsWritten(shardID uint16)

	// NotifySegmentWrite notify that the segment is being written for shard.
	NotifySegmentWrite(shardID uint16)

	// Set for shard number of segments.
	Set(shardID uint16, numberOfSegments uint32)
}

// NoopSegmentWriteNotifier notify when new segment write. [SegmentWriteNotifier] of the implementation.
type NoopSegmentWriteNotifier struct{}

// NotifySegmentIsWritten notify that the segment has been flushed for shard.
// [SegmentWriteNotifier] of the implementation.
func (NoopSegmentWriteNotifier) NotifySegmentIsWritten(uint16) {}

// NotifySegmentWrite notify that the segment is being written for shard. [SegmentWriteNotifier] of the implementation.
func (NoopSegmentWriteNotifier) NotifySegmentWrite(uint16) {}

// Set for shard number of segments. [SegmentWriteNotifier] of the implementation.
func (NoopSegmentWriteNotifier) Set(uint16, uint32) {}

//
// ShardDataLoader
//

// ShardDataLoader loads shard data from a file and creates a shard.
type ShardDataLoader struct {
	dir                       string
	notifier                  SegmentWriteNotifier
	segmentMarkup             writer.SegmentMarkup
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
		segmentMarkup:             headRecord,
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
	walVersion, encoderVersion, _, err := reader.ReadHeader(rd)
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

	switch walVersion {
	case wal.FileFormatVersion:
		l.segmentMarkup = writer.NoopSegmentMarkup{}
		l.shardData.writeSegment = writer.WriteSegment[*cppbridge.HeadEncodedSegment]
		l.shardData.walVersion = walVersion
		l.shardData.numberOfSegments, err = l.loadSegments(
			rd,
			decoder,
			l.shardData.dataStorage,
			unloader,
		)
	case wal.FileFormatVersionV2:
		l.notifier = NoopSegmentWriteNotifier{}
		l.shardData.writeSegment = writer.WriteSegmentV2[*cppbridge.HeadEncodedSegment]
		l.shardData.walVersion = walVersion
		l.shardData.numberOfSegments, err = l.loadSegmentsV2(
			rd,
			decoder,
			l.shardData.dataStorage,
			unloader,
		)
	default:
		return decoder, fmt.Errorf("unknown wal file format: %d", walVersion)
	}

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
		l.shardData.writeSegment,
		l.notifier,
		l.segmentMarkup,
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
func (*ShardDataLoader) loadSegments(
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

func (l *ShardDataLoader) loadSegmentsV2(
	rd io.Reader,
	walDecoder *cppbridge.HeadWalDecoder,
	dataStorage *shard.DataStorage,
	unloader *dataUnloader,
) (uint32, error) {
	numberOfSegments := uint32(0)

	if err := wal.NewSegmentWalReader(rd, reader.NewSegmentV2).ForEachSegment(func(segment *reader.SegmentV2) error {
		createTs, encodeTs, decodeErr := dataStorage.DecodeSegment(walDecoder, segment.Bytes())
		if decodeErr != nil {
			return fmt.Errorf("failed to decode segment: %w", decodeErr)
		}

		numberOfSegments++
		l.segmentMarkup.SetSegmentIDByShard(segment.ID(), l.shardID)
		l.shardData.maxSegmentID = segment.ID()

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

// checkWalVersion checks wal version of all shards.
func checkWalVersion(shardLoadResults []ShardLoadResult) uint8 {
	walVersion := uint8(0)

	for i := range shardLoadResults {
		// wal version is the same
		if walVersion != 0 && walVersion == shardLoadResults[i].walVersion {
			continue
		}

		// wal version is not set
		if walVersion == 0 {
			walVersion = shardLoadResults[i].walVersion
			continue
		}

		// wal version is different, unlikely
		logger.Warnf("wal version mismatch: %d != %d", walVersion, shardLoadResults[i].walVersion)
		walVersion = shardLoadResults[i].walVersion
	}

	return walVersion
}

// setLastAppendedSegmentID sets last appended segment id to record.
func setLastAppendedSegmentID(rec *catalog.Record, numberOfSegmentsRead optional.Optional[uint32]) {
	// wal format v1
	switch {
	case rec.Status() == catalog.StatusActive:
		// numberOfSegments here is actual number of segments.
		if numberOfSegmentsRead.Value() > 0 {
			rec.SetLastAppendedSegmentID(numberOfSegmentsRead.Value() - 1)
		}
	case isNumberOfSegmentsMismatched(rec, numberOfSegmentsRead.Value()):
		// numberOfSegments here is actual number of segments.
		if numberOfSegmentsRead.Value() > 0 {
			rec.SetLastAppendedSegmentID(numberOfSegmentsRead.Value() - 1)
		}

		lastAppendedSegmentID := uint32(0)
		if rec.LastAppendedSegmentID() != nil {
			lastAppendedSegmentID = *rec.LastAppendedSegmentID()
		}

		logger.Errorf(
			"head: %s number of segments mismatched: last appended=%d, number of segments read=%d",
			rec.ID(),
			lastAppendedSegmentID,
			numberOfSegmentsRead.Value(),
		)
	}
}
