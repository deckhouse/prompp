package localstorageobserver

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/util"
)

//
// objectType
//

const (
	// unknownType is the type of the unknown object.
	unknownType objectType = 0
	// dirType is the type of the directory object.
	dirType objectType = 1
	// fileType is the type of the file object.
	fileType objectType = 2
)

// objectType is the type of the object.
type objectType int64

// String returns the string representation of the object type.
func (t objectType) String() string {
	switch t {
	case unknownType:
		return "unknown"
	case dirType:
		return "dir"
	case fileType:
		return "file"
	default:
		return fmt.Sprintf("unknown object type: %d", t)
	}
}

//
// object
//

// object is the object in the local storage.
type object struct {
	name string
	t    objectType
}

//
// LocalStorage
//

// LocalStorageObserver is the observer of the local storage.
type LocalStorageObserver struct {
	dir         string
	logger      log.Logger
	lastObjects map[object]int64
	objectsSize *prometheus.GaugeVec
}

// NewLocalStorageObserver init a new [LocalStorageObserver].
func NewLocalStorageObserver(dir string, logger log.Logger, reg prometheus.Registerer) *LocalStorageObserver {
	factory := util.NewUnconflictRegisterer(reg)
	return &LocalStorageObserver{
		dir:         dir,
		logger:      logger,
		lastObjects: make(map[object]int64),
		objectsSize: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_localstorage_unknown_bytes",
				Help: "The size of objects in the local storage in bytes.",
			},
			[]string{"name", "type"},
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

	newObjects := make(map[object]int64, len(lso.lastObjects))
	for _, f := range files {
		if lso.isAcceptableDir(f) || lso.isAcceptableFile(f) {
			continue
		}

		name := f.Name()
		info, err := f.Info()
		if err != nil {
			// revive:disable-next-line:add-constant // this is logger
			_ = level.Error(lso.logger).Log("msg", "failed to get file info", "dir", lso.dir, "file", name, "err", err)
			continue
		}

		if !info.IsDir() {
			newObjects[object{name: name, t: fileType}] = info.Size()
			continue
		}

		lsSize, err := lso.getDirSize(ctx, filepath.Join(lso.dir, name))
		if err != nil {
			_ = level.Error(lso.logger).Log(
				// revive:disable-next-line:add-constant // this is logger
				"msg", "failed to get directory size", "dir", lso.dir, "file", name, "err", err,
			)
			continue
		}

		newObjects[object{name: name, t: dirType}] = lsSize
	}

	for obj, size := range newObjects {
		lso.objectsSize.WithLabelValues(obj.name, obj.t.String()).Set(float64(size))
	}

	for obj := range lso.lastObjects {
		if _, ok := newObjects[obj]; !ok {
			lso.objectsSize.DeleteLabelValues(obj.name, obj.t.String())
		}
	}

	lso.lastObjects = newObjects
}

// isAcceptableDir to check if the directory is acceptable.
func (*LocalStorageObserver) isAcceptableDir(fi fs.DirEntry) bool {
	if !fi.IsDir() {
		return false
	}

	if _, err := ulid.ParseStrict(fi.Name()); err == nil {
		return true
	}

	if _, err := uuid.Parse(fi.Name()); err == nil {
		return true
	}

	return false
}

// isAcceptableFile to check if the file is acceptable.
func (*LocalStorageObserver) isAcceptableFile(fi fs.DirEntry) bool {
	if fi.IsDir() {
		return false
	}

	return fi.Name() == "head.log" || fi.Name() == "queries.active" || fi.Name() == "client_id.uuid"
}

// getDirSize to get the size of the directory.
func (lso *LocalStorageObserver) getDirSize(ctx context.Context, dirName string) (int64, error) {
	var totalSize int64
	walkFn := func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			_ = level.Error(lso.logger).Log(
				"msg", "failed to walk directory",
				"dir", lso.dir,
				"path", path,
				"err", walkErr,
			)
			return filepath.SkipDir
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
