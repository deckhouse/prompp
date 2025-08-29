package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/writer"
)

//
// Builder
//

// Builder building new [HeadOnDisk] with parameters.
type Builder struct {
	catalog        *catalog.Catalog
	dataDir        string
	maxSegmentSize uint32
	registerer     prometheus.Registerer
}

// NewBuilder init new [Builder].
func NewBuilder(
	hcatalog *catalog.Catalog,
	dataDir string,
	maxSegmentSize uint32,
	registerer prometheus.Registerer,
) *Builder {
	return &Builder{
		catalog:        hcatalog,
		dataDir:        dataDir,
		maxSegmentSize: maxSegmentSize,
		registerer:     registerer,
	}
}

// Build new [HeadOnDisk] - [head.Head] with [shard.Shard] with [wal.Wal] which is written to disk.
func (b *Builder) Build(generation uint64, numberOfShards uint16) (*HeadOnDisk, error) {
	headRecord, err := b.catalog.Create(numberOfShards)
	if err != nil {
		return nil, err
	}

	headDir := filepath.Join(b.dataDir, headRecord.ID())
	//revive:disable-next-line:add-constant // this is already a constant
	if err = os.Mkdir(headDir, 0o777); err != nil { //nolint:gosec // need this permissions
		return nil, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, os.RemoveAll(headDir))
		}
	}()

	shards := make([]*ShardOnDisk, numberOfShards)
	swn := writer.NewSegmentWriteNotifier(numberOfShards, headRecord.SetLastAppendedSegmentID)
	for shardID := range numberOfShards {
		s, err := b.createShardOnDisk(headDir, swn, shardID)
		if err != nil {
			return nil, err
		}

		shards[shardID] = s
	}

	return head.NewHead(
		headRecord.ID(),
		shards,
		shard.NewPerGoroutineShard[*WalOnDisk],
		headRecord.Acquire(),
		generation,
		numberOfShards,
		b.registerer,
	), nil
}

// createShardOnDisk create [shard.Shard] with [wal.Wal] which is written to disk.
func (b *Builder) createShardOnDisk(
	headDir string,
	swn *writer.SegmentWriteNotifier,
	shardID uint16,
) (*ShardOnDisk, error) {
	shardFile, err := os.Create(filepath.Join(filepath.Clean(headDir), fmt.Sprintf("shard_%d.wal", shardID)))
	if err != nil {
		return nil, fmt.Errorf("failed to create shard wal file id %d: %w", shardID, err)
	}

	defer func() {
		if err == nil {
			return
		}
		_ = shardFile.Close()
	}()

	lss := shard.NewLSS()
	// logShards is 0 for single encoder
	shardWalEncoder := cppbridge.NewHeadWalEncoder(shardID, 0, lss.Target())

	_, err = writer.WriteHeader(shardFile, wal.FileFormatVersion, shardWalEncoder.Version())
	if err != nil {
		return nil, fmt.Errorf("failed to write header: %w", err)
	}

	sw, err := writer.NewBuffered(shardID, shardFile, writer.WriteSegment[*cppbridge.EncodedSegment], swn)
	if err != nil {
		return nil, fmt.Errorf("failed to create buffered writer shard id %d: %w", shardID, err)
	}

	return shard.NewShard(
		lss,
		shard.NewDataStorage(),
		wal.NewWal(shardWalEncoder, sw, b.maxSegmentSize),
		shardID,
	), nil
}
