package startupcleanup

import (
	"os"
	"path/filepath"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/thanos-io/thanos/pkg/block/metadata"

	"github.com/prometheus/prometheus/pp-pkg/blocks/block"
	"github.com/prometheus/prometheus/tsdb"
)

// testBlockSize is the size of the chunk file of a block created by createBlock.
// It dwarfs the meta.json written next to it, so size retention limits expressed
// in whole blocks do not depend on the exact size of the meta file.
const testBlockSize = 64 * 1024

// testBlockSpec describes a block dir to be created by createBlock.
type testBlockSpec struct {
	id          ulid.ULID
	maxTime     int64
	deletable   bool
	downsampled bool
}

// createBlock creates a block dir with a valid meta.json and a chunk file of
// testBlockSize bytes, returning the path of the block dir.
func (s *StartupCleanupSuite) createBlock(dir string, spec testBlockSpec) string {
	s.T().Helper()

	blockDir := filepath.Join(dir, spec.id.String())
	s.Require().NoError(os.MkdirAll(filepath.Join(blockDir, block.ChunksDirname), 0o777))
	s.Require().NoError(os.WriteFile(
		filepath.Join(blockDir, block.ChunksDirname, "000001"),
		make([]byte, testBlockSize),
		0o600,
	))

	meta := &metadata.Meta{
		BlockMeta: tsdb.BlockMeta{
			ULID:    spec.id,
			MinTime: spec.maxTime - 1,
			MaxTime: spec.maxTime,
		},
	}
	meta.Compaction.Deletable = spec.deletable
	if spec.downsampled {
		meta.Thanos.Downsample.Resolution = 5 * 60 * 1000
	}

	_, err := block.WriteThanosMetaFile(log.NewNopLogger(), blockDir, meta)
	s.Require().NoError(err)

	return blockDir
}

// createFile creates a file of the given size inside dir and returns its path.
func (s *StartupCleanupSuite) createFile(dir, name string, size int) string {
	s.T().Helper()

	path := filepath.Join(dir, name)
	s.Require().NoError(os.WriteFile(path, make([]byte, size), 0o600))

	return path
}

// removeExpiredBlocks runs the phase under test with the given retention.
func (s *StartupCleanupSuite) removeExpiredBlocks(dir string, retentionDuration, maxBytes int64) {
	s.T().Helper()

	RemoveExpiredBlocks(log.NewNopLogger(), dir, &BlocksOptions{
		RetentionDuration: retentionDuration,
		MaxBytes:          maxBytes,
	}, nil)
}

func (s *StartupCleanupSuite) TestRemoveExpiredBlocksDisabled() {
	dir := s.T().TempDir()
	newest := s.createBlock(dir, testBlockSpec{id: ulid.MustNew(2, nil), maxTime: 2000})
	oldest := s.createBlock(dir, testBlockSpec{id: ulid.MustNew(1, nil), maxTime: 1000})

	s.removeExpiredBlocks(dir, 500, 0)

	s.requireExists(newest)
	s.requireExists(oldest)
}

func (s *StartupCleanupSuite) TestRemoveExpiredBlocksRetentionDisabled() {
	s.enable()

	dir := s.T().TempDir()
	newest := s.createBlock(dir, testBlockSpec{id: ulid.MustNew(2, nil), maxTime: 2000})
	oldest := s.createBlock(dir, testBlockSpec{id: ulid.MustNew(1, nil), maxTime: 1000})

	s.removeExpiredBlocks(dir, 0, 0)

	s.requireExists(newest)
	s.requireExists(oldest)
}

func (s *StartupCleanupSuite) TestRemoveExpiredBlocksTimeRetention() {
	s.enable()

	dir := s.T().TempDir()
	newest := s.createBlock(dir, testBlockSpec{id: ulid.MustNew(3, nil), maxTime: 2000})
	middle := s.createBlock(dir, testBlockSpec{id: ulid.MustNew(2, nil), maxTime: 1500})
	oldest := s.createBlock(dir, testBlockSpec{id: ulid.MustNew(1, nil), maxTime: 1000})
	// Downsampled blocks are exempt from the time retention, however old they are.
	downsampled := s.createBlock(dir, testBlockSpec{
		id:          ulid.MustNew(4, nil),
		maxTime:     100,
		downsampled: true,
	})

	s.removeExpiredBlocks(dir, 500, 0)

	s.requireExists(newest)
	s.requireExists(downsampled)
	s.requireNotExists(middle)
	s.requireNotExists(oldest)
}

func (s *StartupCleanupSuite) TestRemoveExpiredBlocksSizeRetention() {
	s.enable()

	dir := s.T().TempDir()
	newest := s.createBlock(dir, testBlockSpec{id: ulid.MustNew(2, nil), maxTime: 2000})
	oldest := s.createBlock(dir, testBlockSpec{id: ulid.MustNew(1, nil), maxTime: 1000})

	// Everything that is not a block is charged against the size budget: the head
	// and the catalog log below take up two block sizes on their own.
	head := s.mkdir(dir, headDirUUID)
	s.createFile(head, "data", testBlockSize)
	s.createFile(dir, "head.log", testBlockSize)

	// Two block sizes of non-block data plus room for one and a half blocks, so
	// only the newest of the two blocks fits.
	s.removeExpiredBlocks(dir, 0, 2*testBlockSize+testBlockSize/2*3)

	s.requireExists(newest)
	s.requireNotExists(oldest)
}

func (s *StartupCleanupSuite) TestRemoveExpiredBlocksDeletableBlock() {
	s.enable()

	dir := s.T().TempDir()
	deletable := s.createBlock(dir, testBlockSpec{
		id:        ulid.MustNew(1, nil),
		maxTime:   1000,
		deletable: true,
	})
	kept := s.createBlock(dir, testBlockSpec{id: ulid.MustNew(2, nil), maxTime: 1000})

	s.removeExpiredBlocks(dir, 0, 0)

	s.requireNotExists(deletable)
	s.requireExists(kept)
}

func (s *StartupCleanupSuite) TestRemoveExpiredBlocksSkipsUnreadableMeta() {
	s.enable()

	dir := s.T().TempDir()
	newest := s.createBlock(dir, testBlockSpec{id: ulid.MustNew(2, nil), maxTime: 2000})

	// A block whose meta.json cannot be read tells us nothing about its time
	// range, so it is left for the block manager to deal with.
	brokenMeta := s.mkdir(dir, blockULID)
	s.Require().NoError(os.WriteFile(filepath.Join(brokenMeta, block.MetaFilename), []byte("{"), 0o600))
	missingMeta := s.mkdir(dir, legacyBlockULID)

	s.removeExpiredBlocks(dir, 500, 0)

	s.requireExists(newest)
	s.requireExists(brokenMeta)
	s.requireExists(missingMeta)
}

func (s *StartupCleanupSuite) TestRemoveExpiredBlocksKeepsNonBlockData() {
	s.enable()

	dir := s.T().TempDir()
	s.createBlock(dir, testBlockSpec{id: ulid.MustNew(2, nil), maxTime: 2000})
	oldest := s.createBlock(dir, testBlockSpec{id: ulid.MustNew(1, nil), maxTime: 1000})

	head := s.mkdir(dir, headDirUUID)
	headLog := s.createFile(dir, "head.log", 16)
	queriesActive := s.createFile(dir, "queries.active", 16)

	// The size retention is enabled, so the non-block data is walked, but the
	// limit is high enough for only the time retention to remove anything.
	s.removeExpiredBlocks(dir, 500, 100*testBlockSize)

	s.requireNotExists(oldest)
	s.requireExists(head)
	s.requireExists(headLog)
	s.requireExists(queriesActive)
}

func (s *StartupCleanupSuite) TestRemoveExpiredBlocksLeavesNoTmpDirs() {
	s.enable()

	dir := s.T().TempDir()
	s.createBlock(dir, testBlockSpec{id: ulid.MustNew(2, nil), maxTime: 2000})
	oldest := ulid.MustNew(1, nil)
	s.createBlock(dir, testBlockSpec{id: oldest, maxTime: 1000})

	s.removeExpiredBlocks(dir, 500, 0)

	s.requireNotExists(filepath.Join(dir, oldest.String()))
	s.requireNotExists(filepath.Join(dir, oldest.String()+tmpForDeletionBlockDirSuffix))
}

func (s *StartupCleanupSuite) TestRemoveExpiredBlocksNilOptions() {
	s.enable()

	dir := s.T().TempDir()
	kept := s.createBlock(dir, testBlockSpec{id: ulid.MustNew(1, nil), maxTime: 1000})

	s.Require().NotPanics(func() {
		RemoveExpiredBlocks(log.NewNopLogger(), dir, nil, nil)
	})

	s.requireExists(kept)
}

func (s *StartupCleanupSuite) TestRemoveExpiredBlocksMissingDir() {
	s.enable()

	s.Require().NotPanics(func() {
		s.removeExpiredBlocks(filepath.Join(s.T().TempDir(), "absent"), 500, 0)
	})
}
