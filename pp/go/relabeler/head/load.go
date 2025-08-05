package head

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"io"
	"os"
	"path/filepath"
	"sync"

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
	unloadedDataStorages := make([]*cppbridge.UnloadedDataStorage, numberOfShards)

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
		lsses[shardID], wals[shardID], dataStorages[shardID], unloadedDataStorages[shardID], err = createShard(dir, shardID, swn, maxSegmentSize)
		if err != nil {
			return nil, fmt.Errorf("failed to create shard: %w", err)
		}
	}

	return New(id, generation, configs, lsses, wals, dataStorages, unloadedDataStorages, numberOfShards, registerer)
}

func getShardWalFilename(dir string, shardID uint16) string {
	return filepath.Join(dir, fmt.Sprintf("shard_%d.wal", shardID))
}

func getUnloadedDataStorageFilename(dir string, shardID uint16) string {
	return filepath.Join(dir, fmt.Sprintf("unloaded_%d.ds", shardID))
}

// createShard create shard for head.
func createShard(
	dir string,
	shardID uint16,
	swn *segmentWriteNotifier,
	maxSegmentSize uint32,
) (*LSS, *ShardWal, *DataStorage, *cppbridge.UnloadedDataStorage, error) {
	dir = filepath.Clean(dir)
	shardFile, err := os.Create(getShardWalFilename(dir, shardID))
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create shard wal file: %w", err)
	}

	defer func() {
		if err == nil {
			return
		}
		_ = shardFile.Close()
	}()

	lss := &LSS{
		input:  cppbridge.NewLssStorage(),
		target: cppbridge.NewQueryableLssStorage(),
	}

	shardWalEncoder := cppbridge.NewHeadWalEncoder(shardID, HeadWalEncoderDecoderLogShards, lss.target)
	_, err = WriteHeader(shardFile, FileFormatVersion, shardWalEncoder.Version())
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to write header: %w", err)
	}

	sw, err := newSegmentWriter(shardID, shardFile, swn)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to init segmentWriter: %w", err)
	}

	shardWal := newShardWal(shardWalEncoder, maxSegmentSize, sw)

	unloadedDataStorageFile, err := os.Create(getUnloadedDataStorageFilename(dir, shardID))
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create unloaded data storage file: %w", err)
	}

	return lss, shardWal, NewDataStorage(), cppbridge.NewUnloadedDataStorage(unloadedDataStorageFile), nil
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
) (_ *Head, corrupted bool, numberOfSegments uint32, err error) {
	shardLoadResults := make([]ShardLoadResult, numberOfShards)
	wg := &sync.WaitGroup{}
	swn := newSegmentWriteNotifier(numberOfShards, lastAppendedSegmentIDSetter)
	for shardID := uint16(0); shardID < numberOfShards; shardID++ {
		wg.Add(1)
		go func(shardID uint16, shardWalFilePath, unloadedDataStorageFilePath string, notifier *segmentWriteNotifier) {
			defer wg.Done()
			var err error
			shardLoadResults[shardID], err = NewShardLoader(shardID, shardWalFilePath, unloadedDataStorageFilePath, maxSegmentSize, notifier).Load()
			if err != nil {
				logger.Warnf("load shard error: %v", err)
			}
		}(shardID, getShardWalFilename(dir, shardID), getUnloadedDataStorageFilename(dir, shardID), swn)
	}
	wg.Wait()

	lsses := make([]*LSS, numberOfShards)
	wals := make([]*ShardWal, numberOfShards)
	dataStorages := make([]*DataStorage, numberOfShards)
	unloadedDataStorages := make([]*cppbridge.UnloadedDataStorage, numberOfShards)
	numberOfSegmentsRead := optional.Optional[uint32]{}

	for shardID, shardLoadResult := range shardLoadResults {
		lsses[shardID] = shardLoadResult.Lss
		wals[shardID] = shardLoadResult.Wal
		dataStorages[shardID] = shardLoadResult.DataStorage
		unloadedDataStorages[shardID] = shardLoadResult.UnloadedDataStorage
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

	h, err := New(id, generation, configs, lsses, wals, dataStorages, unloadedDataStorages, numberOfShards, registerer)
	if err != nil {
		return nil, corrupted, numberOfSegmentsRead.Value(), fmt.Errorf("failed to create head: %w", err)
	}

	h.MergeOutOfOrderChunks()

	return h, corrupted, numberOfSegmentsRead.Value(), nil
}

type ShardLoader struct {
	shardID                     uint16
	shardFilePath               string
	unloadedDataStorageFilePath string
	maxSegmentSize              uint32
	notifier                    *segmentWriteNotifier
}

func NewShardLoader(shardID uint16, shardFilePath, unloadedDataStorageFilePath string, maxSegmentSize uint32, notifier *segmentWriteNotifier) *ShardLoader {
	return &ShardLoader{
		shardID:                     shardID,
		shardFilePath:               shardFilePath,
		unloadedDataStorageFilePath: unloadedDataStorageFilePath,
		maxSegmentSize:              maxSegmentSize,
		notifier:                    notifier,
	}
}

type ShardLoadResult struct {
	Lss                 *LSS
	DataStorage         *DataStorage
	Wal                 *ShardWal
	UnloadedDataStorage *cppbridge.UnloadedDataStorage
	NumberOfSegments    uint32
	Corrupted           bool
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

	shardWalFile, err := os.OpenFile(l.shardFilePath, os.O_RDWR, 0o600)
	if err != nil {
		return result, err
	}

	defer func() {
		if result.Corrupted {
			_ = shardWalFile.Close()
			if result.UnloadedDataStorage != nil {
				_ = result.UnloadedDataStorage.Close()
			}
		}
	}()

	unloadedDataStorageFile, err := os.Create(l.unloadedDataStorageFilePath)
	if err != nil {
		return result, err
	}
	result.UnloadedDataStorage = cppbridge.NewUnloadedDataStorage(unloadedDataStorageFile)

	decoder, err := l.loadWalFile(bufio.NewReaderSize(shardWalFile, 1024*1024*4), &result)
	if err != nil {
		return result, err
	}

	if err = l.createShardWal(shardWalFile, decoder, &result); err != nil {
		return result, err
	}

	result.Corrupted = false
	return result, nil
}

func (l *ShardLoader) loadWalFile(reader io.Reader, result *ShardLoadResult) (*cppbridge.HeadWalDecoder, error) {
	_, encoderVersion, _, err := ReadHeader(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read wal header: %w", err)
	}

	decoder := cppbridge.NewHeadWalDecoder(result.Lss.target, encoderVersion)
	result.NumberOfSegments, err = loadSegments(reader, decoder, result.DataStorage.encoder)
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

func loadSegments(reader io.Reader, walDecoder *cppbridge.HeadWalDecoder, encoder *cppbridge.HeadEncoder) (uint32, error) {
	numberOfSegments := uint32(0)

	for {
		segment, _, err := ReadSegment(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return numberOfSegments, nil
			}
			return 0, fmt.Errorf("failed to read segment: %w", err)
		}

		if _, err = walDecoder.DecodeToDataStorage(segment.data, encoder); err != nil {
			return 0, fmt.Errorf("failed to decode segment: %w", err)
		}

		numberOfSegments++
	}
}
