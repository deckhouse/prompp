package storage

import (
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/writer"
)

// WalOnDisk wal on disk.
type WalOnDisk = wal.Wal[
	*cppbridge.HeadEncodedSegment,
	*writer.Buffered[*cppbridge.HeadEncodedSegment],
]

// ShardOnDisk [shard.Shard].
type ShardOnDisk = shard.Shard

// PerGoroutineShard [shard.PerGoroutineShard].
type PerGoroutineShard = shard.PerGoroutineShard

// HeadOnDisk [head.Head] with [ShardOnDisk].
type HeadOnDisk = head.Head[*ShardOnDisk, *PerGoroutineShard]
