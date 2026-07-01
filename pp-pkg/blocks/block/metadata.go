package block

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/thanos-io/thanos/pkg/block/metadata"

	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/fileutil"
)

// WriteThanosMetaFile writes the Thanos meta file to the directory.
func WriteThanosMetaFile(logger log.Logger, dir string, tmeta *metadata.Meta) (int64, error) {
	tmeta.Version = MetaVersion1
	tmeta.Thanos.Version = metadata.ThanosVersion1

	// Make any changes to the file appear atomic.
	path := filepath.Join(dir, MetaFilename)
	tmp := path + ".tmp"
	defer func() {
		if err := os.RemoveAll(tmp); err != nil {
			_ = level.Error(logger).Log("msg", "remove tmp file", "err", err.Error())
		}
	}()

	f, err := os.Create(tmp) // #nosec G304 // it's meant to be that way
	if err != nil {
		return 0, err
	}

	jsonMeta, err := json.MarshalIndent(tmeta, "", "\t")
	if err != nil {
		return 0, errors.Join(err, f.Close())
	}

	n, err := f.Write(jsonMeta)
	if err != nil {
		return 0, errors.Join(err, f.Close())
	}

	// Force the kernel to persist the file on disk to avoid data loss if the host crashes.
	if err := f.Sync(); err != nil {
		return 0, errors.Join(err, f.Close())
	}

	if err := f.Close(); err != nil {
		return 0, err
	}

	return int64(n), fileutil.Replace(tmp, path)
}

// ReadFromDir reads the given thanos [metadata.Meta] from <dir>/meta.json.
func ReadFromDir(dir string) (*metadata.Meta, int64, error) {
	b, err := os.ReadFile(filepath.Join(dir, filepath.Clean(MetaFilename))) // #nosec G304 // open meta file
	if err != nil {
		return nil, 0, err
	}

	var m metadata.Meta
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, 0, err
	}

	if m.Version != MetaVersion1 {
		return nil, 0, fmt.Errorf("unexpected meta file version %d", m.Version)
	}

	version := m.Thanos.Version
	if version == 0 {
		// For compatibility.
		version = metadata.ThanosVersion1
	}

	if version != metadata.ThanosVersion1 {
		return nil, 0, fmt.Errorf("unexpected meta file Thanos section version %d", m.Version)
	}

	if m.Thanos.Labels == nil {
		// To avoid extra nil checks, allocate map here if empty.
		m.Thanos.Labels = make(map[string]string)
	}

	return &m, int64(len(b)), nil
}

// WriteTSDBMetaFile writes the TSDB meta file to the directory.
func WriteTSDBMetaFile(logger log.Logger, dir string, meta *tsdb.BlockMeta) (int64, error) {
	meta.Version = MetaVersion1

	// Make any changes to the file appear atomic.
	path := filepath.Join(dir, MetaFilename)
	tmp := path + ".tmp"
	defer func() {
		if err := os.RemoveAll(tmp); err != nil {
			_ = level.Error(logger).Log("msg", "remove tmp file", "err", err.Error())
		}
	}()

	f, err := os.Create(tmp) // #nosec G304 // it's meant to be that way
	if err != nil {
		return 0, err
	}

	jsonMeta, err := json.MarshalIndent(meta, "", "\t")
	if err != nil {
		return 0, errors.Join(err, f.Close())
	}

	n, err := f.Write(jsonMeta)
	if err != nil {
		return 0, errors.Join(err, f.Close())
	}

	// Force the kernel to persist the file on disk to avoid data loss if the host crashes.
	if err := f.Sync(); err != nil {
		return 0, errors.Join(err, f.Close())
	}

	if err := f.Close(); err != nil {
		return 0, err
	}

	return int64(n), fileutil.Replace(tmp, path)
}
