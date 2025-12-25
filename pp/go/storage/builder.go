package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/writer"
	"github.com/prometheus/prometheus/pp/go/storage/head/transactionhead"
	"github.com/prometheus/prometheus/pp/go/util"
)

//
// Builder
//

// Builder building new [Head] with parameters.
type Builder struct {
	catalog                   *catalog.Catalog
	dataDir                   string
	maxSegmentSize            uint32
	registerer                prometheus.Registerer
	unloadDataStorageInterval time.Duration
	// stat
	events *prometheus.CounterVec
}

// NewBuilder init new [Builder].
func NewBuilder(
	hcatalog *catalog.Catalog,
	dataDir string,
	maxSegmentSize uint32,
	registerer prometheus.Registerer,
	unloadDataStorageInterval time.Duration,
) *Builder {
	factory := util.NewUnconflictRegisterer(registerer)
	return &Builder{
		catalog:                   hcatalog,
		dataDir:                   dataDir,
		maxSegmentSize:            maxSegmentSize,
		registerer:                registerer,
		unloadDataStorageInterval: unloadDataStorageInterval,
		events: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "prompp_head_event_count",
				Help: "Number of head events",
			},
			[]string{"type"},
		),
	}
}

// Build new [Head] - [head.Head] with [shard.Shard] with [wal.Wal] which is written to disk.
func (b *Builder) Build(generation uint64, numberOfShards uint16) (*Head, error) {
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

	shards := make([]*shard.Shard, numberOfShards)
	swn := writer.NewSegmentWriteNotifier(numberOfShards, headRecord.SetLastAppendedSegmentID)
	for shardID := range numberOfShards {
		s, err := b.createShardOnDisk(headDir, swn, shardID)
		if err != nil {
			return nil, err
		}

		shards[shardID] = s
	}

	b.events.With(prometheus.Labels{"type": "created"}).Inc()
	logger.Debugf("[Builder] builded head: %s", headRecord.ID())
	return head.NewHead(
		headRecord.ID(),
		shards,
		shard.NewPerGoroutineShard[*Wal],
		headRecord.Acquire(),
		generation,
		b.registerer,
	), nil
}

// BuildTransactionHead new [TransactionHead] - [transactionhead.Head]
// with [shard.Shard] with [wal.NoopWal] which is written to disk.
func (b *Builder) BuildTransactionHead() *TransactionHead {
	sd := shard.NewShard(
		shard.NewLSS(),
		shard.NewDataStorage(),
		nil,
		nil,
		wal.NewNoopWal(),
		0,
	)

	th := transactionhead.NewHead(
		catalog.DefaultIDGenerator{}.Generate().String(),
		sd,
		shard.NewPerGoroutineShard[*wal.NoopWal](sd, 1),
	)

	b.events.With(prometheus.Labels{"type": "created_transaction_head"}).Inc()
	logger.Debugf("[Builder] builded head: %s", th.String())

	return th
}

// createShardOnDisk create [shard.Shard] with [wal.Wal] which is written to disk.
//
//revive:disable-next-line:function-length // long but readable.
func (b *Builder) createShardOnDisk(
	headDir string,
	swn *writer.SegmentWriteNotifier,
	shardID uint16,
) (*shard.Shard, error) {
	headDir = filepath.Clean(headDir)
	//revive:disable-next-line:add-constant // file permissions simple readable as octa-number
	shardFile, err := util.CreateFileAppender(GetShardWalFilename(headDir, shardID), 0o666)
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

	sw, err := writer.NewBuffered(shardID, shardFile, writer.WriteSegment[*cppbridge.HeadEncodedSegment], swn)
	if err != nil {
		return nil, fmt.Errorf("failed to create buffered writer shard id %d: %w", shardID, err)
	}

	var unloadedDataStorage *shard.UnloadedDataStorage
	var queriedSeriesStorage *shard.QueriedSeriesStorage
	if b.unloadDataStorageInterval != 0 {
		unloadedDataStorage = shard.NewUnloadedDataStorage(
			shard.NewAppendFileStorage(GetUnloadedDataStorageFilename(headDir, shardID)),
		)

		queriedSeriesStorage = shard.NewQueriedSeriesStorage(
			shard.NewFileStorage(GetQueriedSeriesStorageFilename(headDir, shardID, 0)),
			shard.NewFileStorage(GetQueriedSeriesStorageFilename(headDir, shardID, 1)),
		)
	}

	return shard.NewShard(
		lss,
		shard.NewDataStorage(),
		unloadedDataStorage,
		queriedSeriesStorage,
		wal.NewWal(shardWalEncoder, sw, b.maxSegmentSize, shardID, b.registerer),
		shardID,
	), nil
}
