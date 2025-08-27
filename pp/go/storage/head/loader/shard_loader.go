package loader

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

type SegmentWriteNotifier interface {
	Set(shardID uint16, numberOfSegments uint32)
}

type ShardLoader struct {
	notifier       SegmentWriteNotifier
	shardFilePath  string
	maxSegmentSize uint32
	shardID        uint16
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

type ShardLoadResult struct {
	Lss              *LSS
	DataStorage      *DataStorage
	Wal              *ShardWal
	NumberOfSegments uint32
	Corrupted        bool
	Err              error
}
