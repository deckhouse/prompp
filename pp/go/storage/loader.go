package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/reader"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/writer"
	"github.com/prometheus/prometheus/pp/go/storage/logger"
	"github.com/prometheus/prometheus/pp/go/util/optional"
)

// Loader loads [HeadOnDisk] or [ShardOnDisk] from [WalOnDisk].
type Loader struct {
	dataDir        string
	maxSegmentSize uint32
	registerer     prometheus.Registerer
}

// NewLoader init new [Loader].
func NewLoader(dataDir string, maxSegmentSize uint32, registerer prometheus.Registerer) *Loader {
	return &Loader{
		dataDir:        dataDir,
		maxSegmentSize: maxSegmentSize,
		registerer:     registerer,
	}
}

// UploadHead upload [HeadOnDisk] from [WalOnDisk] by head ID.
func (l *Loader) UploadHead(
	headRecord *catalog.Record,
	generation uint64,
) (_ *HeadOnDisk, _ uint32, corrupted bool) {
	headID := headRecord.ID()
	headDir := filepath.Join(l.dataDir, headID)
	numberOfShards := headRecord.NumberOfShards()
	shardLoadResults := make([]ShardLoadResult, numberOfShards)

	wg := &sync.WaitGroup{}
	swn := writer.NewSegmentWriteNotifier(numberOfShards, headRecord.SetLastAppendedSegmentID)
	for shardID := range numberOfShards {
		wg.Add(1)
		go func(shardID uint16, shardWalFilePath string) {
			defer wg.Done()
			shardLoadResults[shardID] = l.UploadShard(shardWalFilePath, swn, shardID)
		}(shardID, filepath.Join(headDir, fmt.Sprintf("shard_%d.wal", shardID)))
	}
	wg.Wait()

	shards := make([]*ShardOnDisk, numberOfShards)
	numberOfSegmentsRead := optional.Optional[uint32]{}
	for shardID, res := range shardLoadResults {
		shards[shardID] = res.Shard()
		if res.Corrupted() {
			corrupted = true
		}

		if numberOfSegmentsRead.IsNil() {
			numberOfSegmentsRead.Set(res.NumberOfSegments())
		} else if numberOfSegmentsRead.Value() != res.NumberOfSegments() {
			corrupted = true
			// calculating maximum number of segments (critical for remote write).
			if numberOfSegmentsRead.Value() < res.NumberOfSegments() {
				numberOfSegmentsRead.Set(res.NumberOfSegments())
			}
		}
	}

	// TODO h.MergeOutOfOrderChunks()
	return head.NewHead(
			headID,
			shards,
			shard.NewPerGoroutineShard[*WalOnDisk],
			headRecord.Acquire(),
			generation,
			numberOfShards,
			l.registerer,
		),
		numberOfSegmentsRead.Value(),
		corrupted
}

// UploadShard upload [ShardOnDisk] from [WalOnDisk].
func (l *Loader) UploadShard(
	shardFilePath string,
	swn *writer.SegmentWriteNotifier,
	shardID uint16,
) ShardLoadResult {
	res := ShardLoadResult{corrupted: true}

	//revive:disable-next-line:add-constant file permissions simple readable as octa-number
	shardFile, err := os.OpenFile(shardFilePath, os.O_RDWR, 0o600) // #nosec G304 // it's meant to be that way
	if err != nil {
		logger.Debugf("failed to open file shard id %d: %w", shardID, err)
		return res
	}
	defer func() {
		if res.corrupted {
			_ = shardFile.Close()
		}
	}()

	_, encoderVersion, _, err := reader.ReadHeader(shardFile)
	if err != nil {
		logger.Debugf("failed to read wal header: %w", err)
		return res
	}

	lss := shard.NewLSS()
	decoder := cppbridge.NewHeadWalDecoder(lss.Target(), encoderVersion)
	dataStorage := shard.NewDataStorage()

	if err = wal.NewSegmentWalReader[reader.Segment](shardFile).ForEachSegment(func(s *reader.Segment) error {
		if decodeErr := dataStorage.DecodeSegment(decoder, s.Bytes()); decodeErr != nil {
			return fmt.Errorf("failed to decode segment: %w", decodeErr)
		}

		res.numberOfSegments++

		return nil
	}); err != nil {
		logger.Debugf(err.Error())
		return res
	}

	sw, err := writer.NewBuffered(shardID, shardFile, writer.WriteSegment[*cppbridge.EncodedSegment], swn)
	if err != nil {
		logger.Debugf("failed to create buffered writer shard id %d: %w", shardID, err)
		return res
	}

	swn.Set(shardID, res.numberOfSegments)
	res.corrupted = false
	res.shard = shard.NewShard(lss, dataStorage, wal.NewWal(decoder.CreateEncoder(), sw, l.maxSegmentSize), shardID)

	return res
}

//
// ShardLoadResult
//

// ShardLoadResult the result of uploading the [ShardOnDisk] from the [WalOnDisk].
type ShardLoadResult struct {
	shard            *ShardOnDisk
	numberOfSegments uint32
	corrupted        bool
}

// Corrupted returns true if [ShardOnDisk] is corrupted.
func (sr *ShardLoadResult) Corrupted() bool {
	return sr.corrupted
}

// NumberOfSegments returns number of segments in [ShardOnDisk]s.
func (sr *ShardLoadResult) NumberOfSegments() uint32 {
	return sr.numberOfSegments
}

// Shard returns [*ShardOnDisk] or nil.
func (sr *ShardLoadResult) Shard() *ShardOnDisk {
	return sr.shard
}
