package storage

import (
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/head/proxy"
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

// PerGoroutineShard [shard.PerGoroutineShard] with [WalOnDisk].
type PerGoroutineShard = shard.PerGoroutineShard[*WalOnDisk]

// HeadOnDisk [head.Head] with [ShardOnDisk].
type HeadOnDisk = head.Head[*ShardOnDisk, *PerGoroutineShard]

// ProxyHead [proxy.Proxy] for [HeadOnDisk]s.
type ProxyHead = proxy.Proxy[*HeadOnDisk]
