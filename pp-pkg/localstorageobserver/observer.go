package localstorageobserver

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/util"
)

const (
	// cutSuffix is the suffix to cut the name.
	cutSuffix = "..."

	// defaultMaxLength is the default max length of the name.
	defaultMaxLength = 128
)

//
// LocalStorageObserver
//

// LocalStorageObserver is the observer of the local storage.
type LocalStorageObserver struct {
	dir         string
	c           *catalog.Catalog
	logger      log.Logger
	objectsSize prometheus.Gauge
}

// NewLocalStorageObserver init a new [LocalStorageObserver].
func NewLocalStorageObserver(
	dir string,
	c *catalog.Catalog,
	logger log.Logger,
	reg prometheus.Registerer,
) *LocalStorageObserver {
	factory := util.NewUnconflictRegisterer(reg)
	return &LocalStorageObserver{
		dir:    dir,
		c:      c,
		logger: logger,
		objectsSize: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "prompp_localstorage_unknown_bytes",
				Help: "The summary size of unknown or unexpected objects in the local storage in bytes.",
			},
		),
	}
}

// Observe to observe the objects in the local storage.
//
//revive:disable-next-line:cyclomatic // this is not a complex function
func (lso *LocalStorageObserver) Observe(ctx context.Context) {
	files, err := os.ReadDir(lso.dir)
	if err != nil {
		_ = level.Error(lso.logger).Log("msg", "failed to read directory", "dir", lso.dir, "err", err)
		return
	}

	names, totalSize := lso.observeObjects(ctx, files)
	lso.objectsSize.Set(float64(totalSize))
	_ = level.Debug(lso.logger).Log(
		"msg", "observed objects",
		"names", names,
		"totalSize", totalSize,
	)
}

// getSizeDir to get the size of the directory.
func (lso *LocalStorageObserver) getSizeDir(ctx context.Context, dirName string) (int64, error) {
	var totalSize int64
	walkFn := func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			_ = level.Error(lso.logger).Log(
				"msg", "failed to walk directory", // revive:disable-line:add-constant // this is logger
				"dir", lso.dir,
				"path", path,
				"err", walkErr,
			)
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if path == dirName {
			return nil
		}

		entryInfo, err := d.Info()
		if err != nil {
			return nil
		}

		if entryInfo.Mode()&fs.ModeSymlink != 0 {
			return nil
		}

		if !entryInfo.IsDir() {
			totalSize += entryInfo.Size()
		}

		return nil
	}

	err := filepath.WalkDir(dirName, walkFn)

	return totalSize, err
}

// getSizeObject to get the size of the object.
func (lso *LocalStorageObserver) getSizeObject(ctx context.Context, info fs.FileInfo) int64 {
	if info.Mode()&fs.ModeSymlink != 0 {
		return 0 // skip symlink
	}

	if !info.IsDir() {
		return info.Size()
	}

	lsSize, err := lso.getSizeDir(ctx, filepath.Join(lso.dir, info.Name()))
	if err != nil {
		_ = level.Error(lso.logger).Log(
			// revive:disable-next-line:add-constant // this is logger
			"msg", "failed to get directory size", "dir", lso.dir, "file", info.Name(), "err", err,
		)

		return 0
	}

	return lsSize
}

// isKnownHead to check if the head is known.
func (lso *LocalStorageObserver) isKnownHead(de fs.DirEntry) bool {
	if !de.IsDir() {
		return false
	}

	name := de.Name()
	if _, err := uuid.Parse(name); err != nil {
		return false
	}

	if lso.c != nil {
		if _, err := lso.c.Get(name); err != nil {
			return false
		}
	}

	return true
}

// observeObjects to list the objects in the local storage.
//
//revive:disable-next-line:function-length // this is not a complex function
//nolint:gocritic // unnamedResult // returns names as string and total size as int64.
func (lso *LocalStorageObserver) observeObjects(ctx context.Context, files []os.DirEntry) (string, int64) {
	var totalSize int64
	names := make([]string, 0, len(files))
	for _, f := range files {
		select {
		case <-ctx.Done():
			return convertNamesToString(names), totalSize
		default:
		}

		if isAcceptableBlockName(f) || lso.isKnownHead(f) || isAcceptableFile(f) {
			continue
		}

		name := f.Name()
		info, err := f.Info()
		if err != nil {
			_ = level.Error(lso.logger).Log("msg", "failed to get file info", "dir", lso.dir, "file", name, "err", err)
			continue
		}

		totalSize += lso.getSizeObject(ctx, info)
		names = append(names, cutName(name))
	}

	return convertNamesToString(names), totalSize
}

// convertNamesToString to convert the names to a string.
func convertNamesToString(names []string) string {
	if len(names) > defaultMaxLength {
		return strings.Join(names[:defaultMaxLength], ";") + cutSuffix
	}

	return strings.Join(names, ";")
}

// cutName to cut the name to the max length.
func cutName(name string) string {
	if len(name) > defaultMaxLength {
		return name[:defaultMaxLength-len(cutSuffix)] + cutSuffix
	}

	return name
}

// isAcceptableBlockName to check if the directory is acceptable.
func isAcceptableBlockName(de fs.DirEntry) bool {
	if !de.IsDir() {
		return false
	}

	_, err := ulid.ParseStrict(trimTMPSuffix(de.Name()))
	return err == nil
}

// isAcceptableFile to check if the file is acceptable.
func isAcceptableFile(de fs.DirEntry) bool {
	if de.IsDir() {
		return false
	}

	return de.Name() == "head.log" || de.Name() == "queries.active" || de.Name() == "client_id.uuid"
}

// trimTMPSuffix to trim the .tmp-for-creation, .tmp-for-deletion, and .tmp suffix from the name.
func trimTMPSuffix(name string) string {
	if before, ok := strings.CutSuffix(name, ".tmp-for-creation"); ok {
		return before
	}

	if before, ok := strings.CutSuffix(name, ".tmp-for-deletion"); ok {
		return before
	}

	if before, ok := strings.CutSuffix(name, ".tmp"); ok {
		return before
	}

	return name
}
