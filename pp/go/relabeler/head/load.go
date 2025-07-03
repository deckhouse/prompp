package head

import (
	"bufio"
	"errors"
	"fmt"
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
		lsses[shardID], wals[shardID], dataStorages[shardID], err = createShard(dir, shardID, swn, maxSegmentSize)
		if err != nil {
			return nil, fmt.Errorf("failed to create shard: %w", err)
		}
	}

	return New(id, generation, configs, lsses, wals, dataStorages, numberOfShards, registerer)
}

// createShard create shard for head.
func createShard(
	dir string,
	shardID uint16,
	swn *segmentWriteNotifier,
	maxSegmentSize uint32,
) (*LSS, *ShardWal, *DataStorage, error) {
	shardFile, err := os.Create(filepath.Join(filepath.Clean(dir), fmt.Sprintf("shard_%d.wal", shardID)))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create shard wal file: %w", err)
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
		return nil, nil, nil, fmt.Errorf("failed to write header: %w", err)
	}

	sw, err := newSegmentWriter(shardID, shardFile, swn)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to init segmentWriter: %w", err)
	}

	shardWal := newShardWal(shardWalEncoder, maxSegmentSize, sw)

	return lss, shardWal, NewDataStorage(), nil
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
		shardWalFilePath := filepath.Join(dir, fmt.Sprintf("shard_%d.wal", shardID))
		go func(shardID uint16, shardWalFilePath string, notifier *segmentWriteNotifier) {
			defer wg.Done()
			shardLoadResults[shardID] = NewShardLoader(shardID, shardWalFilePath, maxSegmentSize, notifier).Load()
		}(shardID, shardWalFilePath, swn)
	}
	wg.Wait()

	lsses := make([]*LSS, numberOfShards)
	wals := make([]*ShardWal, numberOfShards)
	dataStorages := make([]*DataStorage, numberOfShards)
	numberOfSegmentsRead := optional.Optional[uint32]{}

	for shardID, shardLoadResult := range shardLoadResults {
		lsses[shardID] = shardLoadResult.Lss
		wals[shardID] = shardLoadResult.Wal
		dataStorages[shardID] = shardLoadResult.DataStorage
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

	h, err := New(id, generation, configs, lsses, wals, dataStorages, numberOfShards, registerer)
	if err != nil {
		return nil, corrupted, numberOfSegmentsRead.Value(), fmt.Errorf("failed to create head: %w", err)
	}

	h.MergeOutOfOrderChunks()

	return h, corrupted, numberOfSegmentsRead.Value(), nil
}

type ShardLoader struct {
	shardID        uint16
	shardFilePath  string
	maxSegmentSize uint32
	notifier       *segmentWriteNotifier
}

func NewShardLoader(shardID uint16, shardFilePath string, maxSegmentSize uint32, notifier *segmentWriteNotifier) *ShardLoader {
	return &ShardLoader{
		shardID:        shardID,
		shardFilePath:  shardFilePath,
		maxSegmentSize: maxSegmentSize,
		notifier:       notifier,
	}
}

type ShardLoadResult struct {
	Lss              *LSS
	DataStorage      *DataStorage
	Wal              *ShardWal
	NumberOfSegments uint32
	Corrupted        bool
	Err              error
}

func (l *ShardLoader) Load() (result ShardLoadResult) {
	targetLss := cppbridge.NewQueryableLssStorage()
	dataStorage := NewDataStorage()

	result.Lss = &LSS{
		input:  cppbridge.NewLssStorage(),
		target: targetLss,
	}
	result.DataStorage = dataStorage
	result.Wal = newCorruptedShardWal()
	result.Corrupted = true

	shardWalFile, err := os.OpenFile(l.shardFilePath, os.O_RDWR, 0o600)
	if err != nil {
		result.Err = err
		return
	}

	defer func() {
		if result.Corrupted {
			_ = shardWalFile.Close()
		}
	}()

	reader := bufio.NewReaderSize(shardWalFile, 1024*1024*4)
	_, encoderVersion, offset, err := ReadHeader(reader)
	if err != nil {
		result.Err = fmt.Errorf("failed to read wal header: %w", err)
		return
	}

	decoder := cppbridge.NewHeadWalDecoder(targetLss, encoderVersion)
	lastReadSegmentID := -1

	var bytesRead int
	for {
		var segment DecodedSegment
		segment, bytesRead, err = ReadSegment(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			result.Err = fmt.Errorf("failed to read segment: %w", err)
			break
		}

		err = decoder.DecodeToDataStorage(segment.data, dataStorage.encoder)
		if err != nil {
			result.Err = fmt.Errorf("failed to decode segment: %w", err)
			break
		}

		offset += bytesRead
		lastReadSegmentID++
	}

	numberOfSegments := lastReadSegmentID + 1
	result.NumberOfSegments = uint32(numberOfSegments) // #nosec G115 // no overflow
	sw, err := newSegmentWriter(l.shardID, shardWalFile, l.notifier)
	if err != nil {
		result.Err = err
		return
	}

	l.notifier.Set(l.shardID, uint32(numberOfSegments)) // #nosec G115 // no overflow
	result.Wal = newShardWal(decoder.CreateEncoder(), l.maxSegmentSize, sw)
	if result.Err == nil {
		result.Corrupted = false
	}
	return result
}
