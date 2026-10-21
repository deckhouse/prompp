package catalog

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//
// HeadDirInspector
//

// HeadDirInspector supplies metadata for heads whose initial catalog record was lost.
type HeadDirInspector interface {
	NumberOfShards(id string) (uint16, error)
	BirthTime(id string) (time.Time, error)
}

// Option configures loading and recovery of a [Catalog].
type Option func(*Catalog)

// WithHeadDirInspector enables recovery of heads whose shard count was lost.
func WithHeadDirInspector(inspector HeadDirInspector) Option {
	return func(c *Catalog) { c.inspector = inspector }
}

//
// headDirInspector
//

// headDirInspector reads the head directories immediately under dataDir.
type headDirInspector struct {
	dataDir string
}

// NewHeadDirInspector reads WAL shard indices and directory creation times under dataDir.
func NewHeadDirInspector(dataDir string) HeadDirInspector {
	return &headDirInspector{dataDir: dataDir}
}

func (i *headDirInspector) BirthTime(id string) (time.Time, error) {
	return directoryBirthTime(filepath.Join(i.dataDir, id))
}

func (i *headDirInspector) NumberOfShards(id string) (uint16, error) {
	entries, err := os.ReadDir(filepath.Join(i.dataDir, id))
	if err != nil {
		return 0, err
	}

	var count uint64
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "shard_") || !strings.HasSuffix(name, ".wal") {
			continue
		}

		//revive:disable-next-line:add-constant // base and bitSize
		index, parseErr := strconv.ParseUint(strings.TrimSuffix(strings.TrimPrefix(name, "shard_"), ".wal"), 10, 16)
		if parseErr != nil {
			continue
		}

		if index >= math.MaxUint16 {
			return 0, fmt.Errorf("shard index %d exceeds supported count", index)
		}

		count = max(count, index+1)
	}

	return uint16(count), nil // #nosec G115 // bounded above by MaxUint16
}
