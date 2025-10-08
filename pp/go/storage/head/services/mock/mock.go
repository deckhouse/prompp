package mock

import (
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
)

//go:generate -command moq go run github.com/matryer/moq --rm --skip-ensure --pkg mock --out
//go:generate moq segment_writer.go . SegmentWriter

// SegmentWriter alias for [wal.SegmentWriter] with [cppbridge.HeadEncodedSegment].
type SegmentWriter = wal.SegmentWriter[*cppbridge.HeadEncodedSegment]
