package tsdb

import (
	"path/filepath"
	"slices"

	"github.com/oklog/ulid"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/fileutil"
)

// PPBlocksToDelete returns a filter which decides time based and size based
// retention from the options of the db.
// This is copy of tsdb.DefaultBlocksToDelete function with modifications such as calculation prompp heads size.
func PPBlocksToDelete(db *tsdb.DB, dir string, catalog *catalog.Catalog) tsdb.BlocksToDeleteFunc {
	return func(blocks []*tsdb.Block) map[ulid.ULID]struct{} {
		return deletableBlocks(db, dir, catalog, blocks)
	}
}

// deletableBlocks returns all currently loaded blocks past retention policy or already compacted into a new block.
func deletableBlocks(db *tsdb.DB, dir string, catalog *catalog.Catalog, blocks []*tsdb.Block) map[ulid.ULID]struct{} {
	deletable := make(map[ulid.ULID]struct{})

	// Sort the blocks by time - newest to oldest (largest to smallest timestamp).
	// This ensures that the retentions will remove the oldest  blocks.
	slices.SortFunc(blocks, func(a, b *tsdb.Block) int {
		switch {
		case b.Meta().MaxTime < a.Meta().MaxTime:
			return -1
		case b.Meta().MaxTime > a.Meta().MaxTime:
			return 1
		default:
			return 0
		}
	})

	for _, block := range blocks {
		if block.Meta().Compaction.Deletable {
			deletable[block.Meta().ULID] = struct{}{}
		}
	}

	for ulid := range BeyondTimeRetention(db, blocks) {
		deletable[ulid] = struct{}{}
	}

	for ulid := range BeyondSizeRetention(db, dir, catalog, blocks) {
		deletable[ulid] = struct{}{}
	}

	return deletable
}

// BeyondTimeRetention returns those blocks which are beyond the time retention
// set in the db options.
func BeyondTimeRetention(db *tsdb.DB, blocks []*tsdb.Block) (deletable map[ulid.ULID]struct{}) {
	// Time retention is disabled or no blocks to work with.
	if len(blocks) == 0 || tsdb.DBOpts(db).RetentionDuration == 0 {
		return
	}

	deletable = make(map[ulid.ULID]struct{})
	for i, block := range blocks {
		// The difference between the first block and this block is greater than or equal to
		// the retention period so any blocks after that are added as deletable.
		if i > 0 && blocks[0].Meta().MaxTime-block.Meta().MaxTime >= tsdb.DBOpts(db).RetentionDuration {
			for _, b := range blocks[i:] {
				deletable[b.Meta().ULID] = struct{}{}
			}
			tsdb.DBTimeRetentionCount(db).Inc()
			break
		}
	}
	return deletable
}

// BeyondSizeRetention returns those blocks which are beyond the size retention
// set in the db options.
func BeyondSizeRetention(db *tsdb.DB, dir string, catalog *catalog.Catalog, blocks []*tsdb.Block) (deletable map[ulid.ULID]struct{}) {
	// Size retention is disabled or no blocks to work with.
	if len(blocks) == 0 || tsdb.DBOpts(db).MaxBytes <= 0 {
		return
	}

	deletable = make(map[ulid.ULID]struct{})

	// Initializing size counter with catalog size
	blocksSize := catalogHeadsSize(dir, catalog)
	blocksSize += db.Head().Size()
	for i, block := range blocks {
		blocksSize += block.Size()
		if blocksSize > tsdb.DBOpts(db).MaxBytes {
			// Add this and all following blocks for deletion.
			for _, b := range blocks[i:] {
				deletable[b.Meta().ULID] = struct{}{}
			}
			tsdb.DBSizeRetentionCount(db).Inc()
			break
		}
	}
	return deletable
}

func catalogHeadsSize(dir string, catalog *catalog.Catalog) (catalogSize int64) {
	catalogSize += catalog.OnDiskSize()
	heads := catalog.List(nil, nil)
	for _, h := range heads {
		catalogSize += headSize(filepath.Join(dir, h.Dir()))
	}
	return catalogSize
}

func headSize(dir string) int64 {
	size, _ := fileutil.DirSize(dir)
	return size
}
