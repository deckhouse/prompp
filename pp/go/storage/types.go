package storage

import (
	"errors"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/writer"
)

// Wal alias for [wal.Wal] based on [cppbridge.HeadEncodedSegment] and [writer.Buffered].
type Wal = wal.Wal[*cppbridge.HeadEncodedSegment, *writer.Buffered[*cppbridge.HeadEncodedSegment]]

// Head alias for [head.Head] with [shard.Shard] and [shard.PerGoroutineShard].
type Head = head.Head[*shard.Shard, *shard.PerGoroutineShard]

// ErrInvalidEncoderVersion migration error.
var ErrInvalidEncoderVersion = errors.New("invalid encoder version")
