package tblock

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp-pkg/blocks/block"
	"github.com/thanos-io/thanos/pkg/block/metadata"
)

//
// Worker
//

// Worker is a worker that can work with blocks.
type Worker interface {
	// Exists checks if the given object exists in the destination.
	Exists(ctx context.Context, name string) (bool, error)

	// Upload uploads the given object to the destination.
	Upload(ctx context.Context, name string, r io.Reader) error
}

//
// Functions
//

// MarkForNoCompact creates a file which marks block to be not compacted.
func MarkForNoCompact(
	ctx context.Context,
	logger log.Logger,
	bw Worker,
	id ulid.ULID,
	reason metadata.NoCompactReason,
	details string,
	markedForNoCompact prometheus.Counter,
) error {
	m := path.Join(id.String(), metadata.NoCompactMarkFilename)
	noCompactMarkExists, err := bw.Exists(ctx, m)
	if err != nil {
		return fmt.Errorf("check exists %s in destination: %w", m, err)
	}

	if noCompactMarkExists {
		_ = level.Warn(logger).Log(
			"msg", "requested to mark for no compaction, but file already exists; this should not happen; investigate",
			"err", fmt.Errorf("file %s already exists in bucket", m),
		)
		return nil
	}

	noCompactMark, err := json.Marshal(metadata.NoCompactMark{
		ID:            id,
		Version:       metadata.NoCompactMarkVersion1,
		NoCompactTime: time.Now().Unix(),
		Reason:        reason,
		Details:       details,
	})
	if err != nil {
		return fmt.Errorf("json encode no compact mark: %w", err)
	}

	if err := bw.Upload(ctx, m, bytes.NewBuffer(noCompactMark)); err != nil {
		return fmt.Errorf("upload file %s to destination: %w", m, err)
	}

	markedForNoCompact.Inc()
	_ = level.Info(logger).Log("msg", "block has been marked for no compaction", "block", id)

	return nil
}

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
