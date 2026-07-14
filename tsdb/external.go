package tsdb

import (
	"github.com/go-kit/log"
	"github.com/oklog/ulid"

	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// OpenBlocks loads all blocks from dir, reusing already-loaded ones (usage: pp/go/storage/block).
func OpenBlocks(l log.Logger, dir string, loaded []*Block, chunkPool chunkenc.Pool) ([]*Block, map[ulid.ULID]error, error) {
	return openBlocks(l, dir, loaded, chunkPool)
}

// RemoveBestEffortTmpDirs deletes leftover temporary block dirs (e.g. *.tmp-for-creation,
// *.tmp-for-deletion) that may remain after a crash during compaction or persist
// (usage: pp/go/storage/block).
func RemoveBestEffortTmpDirs(l log.Logger, dir string) error {
	return removeBestEffortTmpDirs(l, dir)
}
