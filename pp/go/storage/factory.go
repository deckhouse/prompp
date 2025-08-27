package storage

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/writer"
)

// WalOnDisk wal on disk.
type WalOnDisk = wal.Wal[
	*cppbridge.EncodedSegment,
	cppbridge.WALEncoderStats,
	*writer.Buffered[*cppbridge.EncodedSegment],
]

// ShardOnDisk [shard.Shard] with [WalOnDisk].
type ShardOnDisk = shard.Shard[*WalOnDisk]

// HeadOnDisk [head.Head] with [ShardOnDisk].
type HeadOnDisk = head.Head[*ShardOnDisk, *shard.PerGoroutineShard[*WalOnDisk]]

// ShardOnDiskConstructor create [shard.Shard] with [wal.Wal] which is written to disk.
func ShardOnDiskConstructor(
	dir string,
	setLastAppendedSegmentID func(segmentID uint32),
	maxSegmentSize uint32,
	numberOfShards, shardID uint16,
) (*ShardOnDisk, error) {
	shardFile, err := os.Create(filepath.Join(filepath.Clean(dir), fmt.Sprintf("shard_%d.wal", shardID)))
	if err != nil {
		return nil, fmt.Errorf("failed to create shard wal file id %d: %w", shardID, err)
	}

	defer func() {
		if err == nil {
			return
		}
		_ = shardFile.Close()
	}()

	swn := writer.NewSegmentWriteNotifier(numberOfShards, setLastAppendedSegmentID)

	sw, err := writer.NewBuffered(shardID, shardFile, writer.WriteSegment[*cppbridge.EncodedSegment], swn)
	if err != nil {
		return nil, fmt.Errorf("failed to create buffered writer shard id %d: %w", shardID, err)
	}

	lss := shard.NewLSS()

	// logShards is 0 for single encoder
	shardWalEncoder := cppbridge.NewHeadWalEncoder(shardID, 0, lss.Target())

	return shard.NewShard(
		lss,
		shard.NewDataStorage(),
		wal.NewWal(shardWalEncoder, sw, maxSegmentSize),
		shardID,
	), nil
}

// HeadConstructor create [head.Head] with [shard.Shard] with [wal.Wal] which is written to disk.
func HeadConstructor(
	id, headDir string,
	releaseHeadFn func(),
	setLastAppendedSegmentID func(segmentID uint32),
	generation uint64,
	maxSegmentSize uint32,
	numberOfShards uint16,
	registerer prometheus.Registerer,
) (*HeadOnDisk, error) {
	shards := make([]*ShardOnDisk, numberOfShards)
	for shardID := range numberOfShards {
		s, err := ShardOnDiskConstructor(
			headDir,
			setLastAppendedSegmentID,
			maxSegmentSize,
			numberOfShards,
			shardID,
		)
		if err != nil {
			return nil, err
		}

		shards[shardID] = s
	}

	return head.NewHead(
		id,
		shards,
		shard.NewPerGoroutineShard[*WalOnDisk],
		releaseHeadFn,
		generation,
		numberOfShards,
		registerer,
	), nil
}
