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
	"github.com/prometheus/prometheus/pp/go/storage/head/poolprovider"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/writer"
	"github.com/prometheus/prometheus/pp/go/storage/head/transactionhead"
	"github.com/prometheus/prometheus/pp/go/util"
)

//
// WalWriter
//

// defaultWalVersion is the WAL file format version written into each shard's WAL header
// (see [writer.WriteHeader] in Builder.createShardOnDisk). It must stay in sync with
// defaultWalWriterCtor: ShardDataLoader picks its segment decoding path (loadSegments vs
// loadSegmentsV2) based on this version byte, so it has to match the segment encoding the
// active constructor actually produces. Defaults to V1; EnableWalWriterV2 switches both
// together.
var defaultWalVersion = uint8(wal.FileFormatVersion)

// defaultWalWriterCtor builds the shard WAL segment writer used by Builder.createShardOnDisk.
// It is a process-global switch between the V1 (default) and V2 ([walWriterCtorV2]) segment
// writer constructors, toggled at startup by EnableWalWriterV2 via the "enable_wal_writer_v2"
// PROMPP_FEATURES flag.
//
// The V1 constructor writes segments in the original format (writer.WriteSegment, no embedded
// segment ID) and relies on the shared *writer.SegmentWriteNotifier passed in by the caller to
// track, across all shards, the last segment durably synced to disk; it doesn't need a real
// SegmentMarkup, since segment IDs aren't assigned or stored, so it passes writer.NoopSegmentMarkup{}.
var defaultWalWriterCtor = func(
	shardID uint16,
	shardFile *util.FileAppender,
	swn *writer.SegmentWriteNotifier,
	_ *catalog.Record,
) (*writer.Buffered[*cppbridge.HeadEncodedSegment], error) {
	return writer.NewBuffered(
		shardID,
		shardFile,
		writer.WriteSegment[*cppbridge.HeadEncodedSegment],
		swn,
		writer.NoopSegmentMarkup{},
	)
}

// walWriterCtorV2 is the V2 counterpart of defaultWalWriterCtor, installed by EnableWalWriterV2.
// It writes segments with writer.WriteSegmentV2, which embeds each segment's globally assigned
// ID directly in the WAL record. Segment IDs are assigned by headRecord (*catalog.Record
// implements SegmentMarkup: NextSegmentID/SetSegmentIDByShard), so each segment's position is
// self-describing on disk and the shared cross-shard SegmentWriteNotifier passed in by the
// caller is no longer needed for that purpose — it's ignored in favor of NoopSegmentWriteNotifier{}.
var walWriterCtorV2 = func(
	shardID uint16,
	shardFile *util.FileAppender,
	_ *writer.SegmentWriteNotifier,
	headRecord *catalog.Record,
) (*writer.Buffered[*cppbridge.HeadEncodedSegment], error) {
	return writer.NewBuffered(
		shardID,
		shardFile,
		writer.WriteSegmentV2[*cppbridge.HeadEncodedSegment],
		NoopSegmentWriteNotifier{},
		headRecord,
	)
}

// EnableWalWriterV2 switches all subsequently created shard WAL writers to the V2 segment
// format (walWriterCtorV2), and updates defaultWalVersion to match so new WAL headers are
// tagged as V2 — required for ShardDataLoader to pick the matching V2 decode path on replay.
// Called once at startup when the "enable_wal_writer_v2" PROMPP_FEATURES flag is set; it does
// not affect WAL files already written with the V1 format.
func EnableWalWriterV2() {
	defaultWalVersion = uint8(wal.FileFormatVersionV2)

	defaultWalWriterCtor = walWriterCtorV2
}

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
	theadPools                *poolprovider.HeadPool[*shard.PerGoroutineShard]
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
		theadPools:                poolprovider.NewHeadPool[*shard.PerGoroutineShard](1),
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
		s, err := b.createShardOnDisk(headDir, swn, headRecord, shardID)
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
		shard.NewDataStorage(false, false),
		nil,
		nil,
		wal.NewNoopWal(),
		0,
	)

	th := transactionhead.NewHead(
		catalog.DefaultIDGenerator{}.Generate().String(),
		sd,
		shard.NewPerGoroutineShard[*wal.NoopWal](sd, 1),
		b.theadPools,
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
	headRecord *catalog.Record,
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

	_, err = writer.WriteHeader(shardFile, defaultWalVersion, shardWalEncoder.Version())
	if err != nil {
		return nil, fmt.Errorf("failed to write header: %w", err)
	}

	sw, err := defaultWalWriterCtor(
		shardID,
		shardFile,
		swn,
		headRecord,
	)
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
		shard.NewDataStorage(true, true),
		unloadedDataStorage,
		queriedSeriesStorage,
		wal.NewWal(shardWalEncoder, sw, lss, b.maxSegmentSize, shardID, b.registerer),
		shardID,
	), nil
}
