package tblock

import (
	"os"
	"path/filepath"

	"github.com/prometheus/prometheus/pp-pkg/blocks/block"
)

//
// Functions
//

// GetSegmentFiles returns list of segment files for given block. Paths are relative to the chunks directory.
// In case of errors, nil is returned.
func GetSegmentFiles(blockDir string) []string {
	files, err := os.ReadDir(filepath.Join(blockDir, block.ChunksDirname))
	if err != nil {
		return nil
	}

	// ReadDir returns files in sorted order already.
	var result []string
	for _, f := range files {
		result = append(result, f.Name())
	}
	return result
}
