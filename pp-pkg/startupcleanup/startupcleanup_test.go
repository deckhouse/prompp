package startupcleanup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/suite"
)

// Block dirs are named by ULID and head dirs by UUID, so the phase under test
// can only recognize the former.
const (
	blockULID         = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	creationBlockULID = "01ARZ3NDEKTSV4RRFFQ69G5FAW"
	deletionBlockULID = "01ARZ3NDEKTSV4RRFFQ69G5FAX"
	legacyBlockULID   = "01ARZ3NDEKTSV4RRFFQ69G5FAY"
	headDirUUID       = "9f8b2a1e-0c3d-4e5f-8a7b-6c5d4e3f2a1b"
)

type StartupCleanupSuite struct {
	suite.Suite
}

func TestStartupCleanupSuite(t *testing.T) {
	suite.Run(t, new(StartupCleanupSuite))
}

// enable turns the feature flag on for the duration of a single test.
func (s *StartupCleanupSuite) enable() {
	previous := Enabled
	Enabled = true
	s.T().Cleanup(func() { Enabled = previous })
}

// mkdir creates a directory inside dir and returns its full path.
func (s *StartupCleanupSuite) mkdir(dir, name string) string {
	path := filepath.Join(dir, name)
	s.Require().NoError(os.MkdirAll(path, 0o777))
	return path
}

func (s *StartupCleanupSuite) requireExists(path string) {
	_, err := os.Stat(path)
	s.Require().NoError(err, "expected %s to exist", path)
}

func (s *StartupCleanupSuite) requireNotExists(path string) {
	_, err := os.Stat(path)
	s.Require().True(os.IsNotExist(err), "expected %s to be removed", path)
}

func (s *StartupCleanupSuite) TestRemoveLeftoverTmpDirsDisabled() {
	dir := s.T().TempDir()
	tmpDir := s.mkdir(dir, creationBlockULID+".tmp-for-creation")

	RemoveLeftoverTmpDirs(log.NewNopLogger(), dir)

	s.requireExists(tmpDir)
}

func (s *StartupCleanupSuite) TestRemoveLeftoverTmpDirs() {
	s.enable()

	dir := s.T().TempDir()
	tmpForCreation := s.mkdir(dir, creationBlockULID+".tmp-for-creation")
	tmpForDeletion := s.mkdir(dir, deletionBlockULID+".tmp-for-deletion")
	tmpLegacy := s.mkdir(dir, legacyBlockULID+".tmp")
	block := s.mkdir(dir, blockULID)
	head := s.mkdir(dir, headDirUUID)

	RemoveLeftoverTmpDirs(log.NewNopLogger(), dir)

	s.requireNotExists(tmpForCreation)
	s.requireNotExists(tmpForDeletion)
	s.requireNotExists(tmpLegacy)

	// Live data must survive: this phase runs without the catalog, so it may only
	// touch dirs it can recognize as temporary on its own.
	s.requireExists(block)
	s.requireExists(head)
}

func (s *StartupCleanupSuite) TestRemoveLeftoverTmpDirsMissingDir() {
	s.enable()

	s.Require().NotPanics(func() {
		RemoveLeftoverTmpDirs(log.NewNopLogger(), filepath.Join(s.T().TempDir(), "absent"))
	})
}

func (s *StartupCleanupSuite) TestCollectHeadsDisabled() {
	gc := &fakeHeadsGC{}

	CollectHeads(gc, log.NewNopLogger())

	s.Equal(0, gc.iterations)
}

func (s *StartupCleanupSuite) TestCollectHeads() {
	s.enable()

	gc := &fakeHeadsGC{}

	CollectHeads(gc, log.NewNopLogger())

	s.Equal(1, gc.iterations)
}

func (s *StartupCleanupSuite) TestCollectHeadsNilGC() {
	s.enable()

	s.Require().NotPanics(func() {
		CollectHeads(nil, log.NewNopLogger())
	})
}

//
// fakeHeadsGC
//

// fakeHeadsGC is a [HeadsGC] counting the passes it was asked to run.
type fakeHeadsGC struct {
	iterations int
}

// Iterate implements the [HeadsGC] interface.
func (gc *fakeHeadsGC) Iterate() {
	gc.iterations++
}
