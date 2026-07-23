package localstorageobserver

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-kit/log"
	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/storage/catalog"
)

type LocalStorageObserverSuite struct {
	suite.Suite
}

func TestLocalStorageObserverSuite(t *testing.T) {
	suite.Run(t, new(LocalStorageObserverSuite))
}

func (*LocalStorageObserverSuite) newObserver(
	dir string,
	c *catalog.Catalog,
) (*LocalStorageObserver, *prometheus.Registry) {
	reg := prometheus.NewRegistry()
	observer := NewLocalStorageObserver(dir, c, log.NewNopLogger(), reg)
	return observer, reg
}

func (s *LocalStorageObserverSuite) gatherUnknownBytes(reg *prometheus.Registry) float64 {
	mfs, err := reg.Gather()
	s.Require().NoError(err)

	var totalSize float64
	for _, mf := range mfs {
		if mf.GetName() == "prompp_localstorage_unknown_bytes" {
			for _, m := range mf.GetMetric() {
				totalSize += m.GetGauge().GetValue()
			}
		}
	}

	return totalSize
}

func (s *LocalStorageObserverSuite) writeFile(path string, size int) {
	err := os.WriteFile(path, make([]byte, size), 0o644)
	s.Require().NoError(err)
}

func (s *LocalStorageObserverSuite) createCatalog() *catalog.Catalog {
	l, err := catalog.NewFileLogV2(filepath.Join(s.T().TempDir(), "catalog.log"))
	s.Require().NoError(err)

	c, err := catalog.New(
		clockwork.NewFakeClock(),
		l,
		catalog.DefaultIDGenerator{},
		catalog.DefaultMaxLogFileSize,
		nil,
	)
	s.Require().NoError(err)

	return c
}

func (s *LocalStorageObserverSuite) TestHappyPath() {
	dir := s.T().TempDir()
	size := 1024
	observer, reg := s.newObserver(dir, nil)

	s.writeFile(filepath.Join(dir, "some-file"), size)
	observer.Observe(s.T().Context())

	s.Equal(float64(size), s.gatherUnknownBytes(reg))
}

func (s *LocalStorageObserverSuite) TestObserveIgnoresAcceptableFiles() {
	dir := s.T().TempDir()
	observer, reg := s.newObserver(dir, nil)

	s.writeFile(filepath.Join(dir, "head.log"), 50)
	s.writeFile(filepath.Join(dir, "queries.active"), 50)
	s.writeFile(filepath.Join(dir, "client_id.uuid"), 50)
	s.writeFile(filepath.Join(dir, "unknown"), 100)

	observer.Observe(s.T().Context())

	s.Equal(float64(100), s.gatherUnknownBytes(reg))
}

func (s *LocalStorageObserverSuite) TestObserveIgnoresBlockDirectory() {
	dir := s.T().TempDir()
	observer, reg := s.newObserver(dir, nil)

	blockDir := filepath.Join(dir, ulid.MustNew(ulid.Now(), nil).String())
	s.Require().NoError(os.Mkdir(blockDir, 0o755))
	s.writeFile(filepath.Join(dir, "unknown"), 200)

	observer.Observe(s.T().Context())

	s.Equal(float64(200), s.gatherUnknownBytes(reg))
}

func (s *LocalStorageObserverSuite) TestObserveIgnoresBlockWithTmpSuffix() {
	dir := s.T().TempDir()
	observer, reg := s.newObserver(dir, nil)

	blockDir := filepath.Join(dir, "01ARZ3NDEKTSV4RRFFQ69G5FAV.tmp-for-creation")
	s.Require().NoError(os.Mkdir(blockDir, 0o755))
	s.writeFile(filepath.Join(dir, "unknown"), 150)

	observer.Observe(s.T().Context())

	s.Equal(float64(150), s.gatherUnknownBytes(reg))
}

func (s *LocalStorageObserverSuite) TestObserveIgnoresUUIDHeadWithoutCatalog() {
	dir := s.T().TempDir()
	observer, reg := s.newObserver(dir, nil)

	headDir := filepath.Join(dir, uuid.New().String())
	s.Require().NoError(os.Mkdir(headDir, 0o755))
	s.writeFile(filepath.Join(headDir, "data"), 500)
	s.writeFile(filepath.Join(dir, "unknown"), 100)

	observer.Observe(s.T().Context())

	s.Equal(float64(100), s.gatherUnknownBytes(reg))
}

func (s *LocalStorageObserverSuite) TestObserveIgnoresKnownHeadInCatalog() {
	dir := s.T().TempDir()
	c := s.createCatalog()
	record, err := c.Create(1)
	s.Require().NoError(err)

	headDir := filepath.Join(dir, record.ID())
	s.Require().NoError(os.Mkdir(headDir, 0o755))
	s.writeFile(filepath.Join(headDir, "data"), 500)

	observer, reg := s.newObserver(dir, c)
	s.writeFile(filepath.Join(dir, "unknown"), 100)

	observer.Observe(s.T().Context())

	s.Equal(float64(100), s.gatherUnknownBytes(reg))
}

func (s *LocalStorageObserverSuite) TestObserveCountsUnknownUUIDHeadWithCatalog() {
	dir := s.T().TempDir()
	c := s.createCatalog()

	unknownHeadDir := filepath.Join(dir, uuid.New().String())
	s.Require().NoError(os.Mkdir(unknownHeadDir, 0o755))
	s.writeFile(filepath.Join(unknownHeadDir, "data"), 300)

	observer, reg := s.newObserver(dir, c)

	observer.Observe(s.T().Context())

	s.Equal(float64(300), s.gatherUnknownBytes(reg))
}

func (s *LocalStorageObserverSuite) TestObserveSumsDirectorySize() {
	dir := s.T().TempDir()
	observer, reg := s.newObserver(dir, nil)

	unknownDir := filepath.Join(dir, "unknown-dir")
	s.Require().NoError(os.Mkdir(unknownDir, 0o755))
	s.writeFile(filepath.Join(unknownDir, "a"), 100)
	s.writeFile(filepath.Join(unknownDir, "b"), 200)

	observer.Observe(s.T().Context())

	s.Equal(float64(300), s.gatherUnknownBytes(reg))
}

func (s *LocalStorageObserverSuite) TestObserveSkipsSymlink() {
	dir := s.T().TempDir()
	observer, reg := s.newObserver(dir, nil)

	target := filepath.Join(dir, "target")
	s.writeFile(target, 100)
	s.Require().NoError(os.Symlink(target, filepath.Join(dir, "link")))

	observer.Observe(s.T().Context())

	s.Equal(float64(100), s.gatherUnknownBytes(reg))
}

func (s *LocalStorageObserverSuite) TestObserveReadDirError() {
	observer, reg := s.newObserver(filepath.Join(s.T().TempDir(), "missing"), nil)

	observer.Observe(s.T().Context())

	s.Equal(float64(0), s.gatherUnknownBytes(reg))
}

func (s *LocalStorageObserverSuite) TestObserveContextCancelled() {
	dir := s.T().TempDir()
	observer, reg := s.newObserver(dir, nil)

	s.writeFile(filepath.Join(dir, "unknown"), 100)

	ctx, cancel := context.WithCancel(s.T().Context())
	cancel()

	observer.Observe(ctx)

	s.Equal(float64(0), s.gatherUnknownBytes(reg))
}

//
// HelpFuncsSuite
//

type HelpFuncsSuite struct {
	suite.Suite
}

func TestHelpFuncsSuite(t *testing.T) {
	suite.Run(t, new(HelpFuncsSuite))
}

func (s *HelpFuncsSuite) TestTrimTMPSuffix() {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "no_suffix", input: "01ARZ3NDEKTSV4RRFFQ69G5FAV", expected: "01ARZ3NDEKTSV4RRFFQ69G5FAV"},
		{name: "tmp_suffix", input: "01ARZ3NDEKTSV4RRFFQ69G5FAV.tmp", expected: "01ARZ3NDEKTSV4RRFFQ69G5FAV"},
		{
			name:     "tmp_for_creation",
			input:    "01ARZ3NDEKTSV4RRFFQ69G5FAV.tmp-for-creation",
			expected: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		},
		{
			name:     "tmp_for_deletion",
			input:    "01ARZ3NDEKTSV4RRFFQ69G5FAV.tmp-for-deletion",
			expected: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Require().Equal(tt.expected, trimTMPSuffix(tt.input))
		})
	}
}

func (s *HelpFuncsSuite) TestCutName() {
	short := "short-name"
	long := strings.Repeat("a", defaultMaxLength+10)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "short_name", input: short, expected: short},
		{
			name:     "long_name",
			input:    long,
			expected: long[:defaultMaxLength-len(cutSuffix)] + cutSuffix,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Require().Equal(tt.expected, cutName(tt.input))
		})
	}
}

func (s *HelpFuncsSuite) TestConvertNamesToString() {
	overflowNames := make([]string, defaultMaxLength+1)
	for i := range overflowNames {
		overflowNames[i] = "name"
	}

	tests := []struct {
		name     string
		names    []string
		expected string
	}{
		{name: "empty", names: nil, expected: ""},
		{name: "two_names", names: []string{"a", "b"}, expected: "a;b"},
		{
			name:     "more_than_max_names",
			names:    overflowNames,
			expected: strings.Join(overflowNames[:defaultMaxLength], ";") + cutSuffix,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Require().Equal(tt.expected, convertNamesToString(tt.names))
		})
	}
}

func (s *HelpFuncsSuite) TestIsAcceptableFile() {
	tests := []struct {
		name     string
		entry    mockDirEntry
		expected bool
	}{
		{name: "head.log", entry: mockDirEntry{name: "head.log"}, expected: true},
		{name: "queries.active", entry: mockDirEntry{name: "queries.active"}, expected: true},
		{name: "client_id.uuid", entry: mockDirEntry{name: "client_id.uuid"}, expected: true},
		{name: "other file", entry: mockDirEntry{name: "other"}, expected: false},
		{name: "directory", entry: mockDirEntry{name: "head.log", isDir: true}, expected: false},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Require().Equal(tt.expected, isAcceptableFile(tt.entry))
		})
	}
}

func (s *HelpFuncsSuite) TestIsAcceptableBlockName() {
	validULID := ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FAV").String()

	tests := []struct {
		name     string
		entry    mockDirEntry
		expected bool
	}{
		{name: "valid ulid dir", entry: mockDirEntry{name: validULID, isDir: true}, expected: true},
		{
			name:     "ulid with tmp for creation",
			entry:    mockDirEntry{name: validULID + ".tmp-for-creation", isDir: true},
			expected: true,
		},
		{name: "not ulid dir", entry: mockDirEntry{name: "not-a-block", isDir: true}, expected: false},
		{name: "ulid file", entry: mockDirEntry{name: validULID}, expected: false},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Require().Equal(tt.expected, isAcceptableBlockName(tt.entry))
		})
	}
}

//
// mockDirEntry
//

// mockDirEntry is a mock implementation of [fs.DirEntry].
type mockDirEntry struct {
	name  string
	isDir bool
}

// Info returns the information about the directory entry.
func (mockDirEntry) Info() (fs.FileInfo, error) { return nil, os.ErrNotExist }

// IsDir returns true if the directory entry is a directory.
func (m mockDirEntry) IsDir() bool { return m.isDir }

// Name returns the name of the directory entry.
func (m mockDirEntry) Name() string { return m.name }

// Type returns the type of the directory entry.
func (mockDirEntry) Type() fs.FileMode { return 0 }
