package startupcleanup

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp-pkg/blocks/block"
	"github.com/prometheus/prometheus/pp-pkg/blocks/expirationpolicy"
	"github.com/prometheus/prometheus/tsdb/fileutil"
)

// tmpForDeletionBlockDirSuffix mirrors the suffix the block manager renames a
// block to before deleting it, so a crash between the rename and the removal
// leaves behind a dir that [RemoveLeftoverTmpDirs] deletes on the next start.
const tmpForDeletionBlockDirSuffix = ".tmp-for-deletion"

//
// BlocksOptions
//

// BlocksOptions is the retention configuration applied to the persisted blocks,
// mirroring the relevant tsdb.Options fields.
type BlocksOptions struct {
	// RetentionDuration is the time retention in milliseconds, 0 means disabled.
	RetentionDuration int64
	// MaxBytes is the maximum number of bytes the local storage may occupy, 0 means disabled.
	MaxBytes int64
}

// RemoveExpiredBlocks deletes the persisted blocks that do not fit the
// configured time and size retention. It only needs the data dir and the
// retention config, so it runs before the heads catalog is opened and before any
// background goroutine starts: on a full disk the blocks it frees may be the
// only reason the rest of the startup can proceed.
//
// The decision is taken by the very same [expirationpolicy.ExpirationPolicy]
// the block manager applies on every reload, so this phase never deletes more
// than the manager would - except that the size budget here is charged with
// everything on disk that is not a block (see extraSize below), while the
// manager only knows about the catalog and its heads.
func RemoveExpiredBlocks(logger log.Logger, dir string, opts *BlocksOptions, r prometheus.Registerer) {
	if !Enabled || opts == nil {
		return
	}

	logger = log.With(logger, "component", component)
	_ = level.Info(logger).Log("msg", "Removing blocks beyond retention before start", "dir", dir)

	blocks, extraBytes, err := scanDir(logger, dir, opts.MaxBytes > 0)
	if err != nil {
		_ = level.Warn(logger).Log("msg", "failed to scan local storage dir", "dir", dir, "err", err)
		return
	}

	if len(blocks) == 0 {
		return
	}

	policy := expirationpolicy.NewExpirationPolicy[diskBlock](
		dir,
		nil,
		&expirationpolicy.Options{
			RetentionDuration: opts.RetentionDuration,
			MaxBytes:          opts.MaxBytes,
			ExtraSize:         func() int64 { return extraBytes },
		},
		r,
	)

	removeBlocks(logger, dir, blocks, policy.BlocksToDelete(blocks))
}

// removeBlocks deletes the given blocks from disk and reports the freed space.
func removeBlocks(logger log.Logger, dir string, blocks []diskBlock, deletable map[ulid.ULID]struct{}) {
	if len(deletable) == 0 {
		return
	}

	var (
		removed int
		freed   int64
	)
	for _, blk := range blocks {
		if _, ok := deletable[blk.ulid]; !ok {
			continue
		}

		if !removeBlock(logger, dir, blk.ulid) {
			continue
		}

		removed++
		freed += blk.size
	}

	_ = level.Info(logger).Log(
		"msg", "Removed blocks beyond retention before start",
		"blocks", removed,
		"freed_bytes", freed,
	)
}

// removeBlock renames the block dir out of the way and only then deletes it, so
// an interrupted deletion cannot leave a partial block that would later be
// loaded as a healthy one.
func removeBlock(logger log.Logger, dir string, uid ulid.ULID) bool {
	from := filepath.Join(dir, uid.String())
	tmpToDelete := filepath.Join(dir, uid.String()+tmpForDeletionBlockDirSuffix)
	if err := fileutil.Replace(from, tmpToDelete); err != nil {
		_ = level.Warn(logger).Log(
			"msg", "replace of block beyond retention for deletion failed",
			"block", uid,
			"err", err,
		)
		return false
	}

	if err := os.RemoveAll(tmpToDelete); err != nil {
		_ = level.Warn(logger).Log("msg", "delete of block beyond retention failed", "block", uid, "err", err)
		return false
	}

	_ = level.Info(logger).Log("msg", "Deleting block beyond retention", "block", uid)

	return true
}

// scanDir splits the top-level entries of the local storage dir into blocks,
// described by their meta.json, and everything else, whose bytes are charged
// against the size retention budget. The extra size is only computed when the
// size retention is enabled, as walking the heads is by far the costlier part.
//
// A block whose meta.json cannot be read is skipped entirely: it is neither
// deleted nor counted. Telling a corrupted block from a healthy one is left to
// the block manager, which has the corrupted-block retention for that.
func scanDir(logger log.Logger, dir string, withExtraSize bool) (blocks []diskBlock, extraBytes int64, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, 0, err
	}

	blocks = make([]diskBlock, 0, len(entries))
	for _, entry := range entries {
		if !isBlockDir(entry) {
			if withExtraSize {
				extraBytes += entrySize(logger, dir, entry)
			}

			continue
		}

		blk, ok := readBlock(logger, filepath.Join(dir, entry.Name()))
		if !ok {
			continue
		}

		blocks = append(blocks, blk)
	}

	return blocks, extraBytes, nil
}

// entrySize returns the on-disk size of a single top-level entry of the local
// storage dir. Symlinks are not followed, so their target is never counted.
func entrySize(logger log.Logger, dir string, entry fs.DirEntry) int64 {
	path := filepath.Join(dir, entry.Name())
	if entry.IsDir() {
		size, err := fileutil.DirSize(path)
		if err != nil {
			_ = level.Warn(logger).Log("msg", "failed to get dir size", "dir", path, "err", err)
		}

		return size
	}

	info, err := entry.Info()
	if err != nil {
		_ = level.Warn(logger).Log("msg", "failed to get file info", "file", path, "err", err)
		return 0
	}

	if !info.Mode().IsRegular() {
		return 0
	}

	return info.Size()
}

// isBlockDir reports whether the entry is a block dir, i.e. a dir named by a ULID.
func isBlockDir(entry fs.DirEntry) bool {
	if !entry.IsDir() {
		return false
	}

	_, err := ulid.ParseStrict(entry.Name())

	return err == nil
}

// readBlock describes the block in the given dir from its meta.json and its
// on-disk size, without opening the index and chunk readers.
func readBlock(logger log.Logger, dir string) (diskBlock, bool) {
	meta, _, err := block.ReadFromDir(dir)
	if err != nil {
		_ = level.Warn(logger).Log("msg", "failed to read block meta, skipping block", "dir", dir, "err", err)
		return diskBlock{}, false
	}

	size, err := fileutil.DirSize(dir)
	if err != nil {
		_ = level.Warn(logger).Log("msg", "failed to get block size", "dir", dir, "err", err)
	}

	return diskBlock{
		ulid:        meta.ULID,
		maxTime:     meta.MaxTime,
		size:        size,
		deletable:   meta.Compaction.Deletable,
		downsampled: meta.Thanos.Downsample.Resolution > 0,
	}, true
}

//
// diskBlock
//

// diskBlock is a persisted block described by its meta.json and its on-disk
// size. It implements [expirationpolicy.Block] without the block being open.
type diskBlock struct {
	ulid        ulid.ULID
	maxTime     int64
	size        int64
	deletable   bool
	downsampled bool
}

// Deletable returns true if the block is deletable.
func (b diskBlock) Deletable() bool { return b.deletable }

// IsDownsamplingBlock returns true if the block is a downsampling block.
func (b diskBlock) IsDownsamplingBlock() bool { return b.downsampled }

// MaxTime returns the maximum time of the block.
func (b diskBlock) MaxTime() int64 { return b.maxTime }

// Size returns the on-disk size of the block.
func (b diskBlock) Size() int64 { return b.size }

// ULID returns the ULID of the block.
func (b diskBlock) ULID() ulid.ULID { return b.ulid }
