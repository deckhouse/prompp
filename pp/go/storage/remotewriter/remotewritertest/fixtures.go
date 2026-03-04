package remotewritertest

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/writer"
	"github.com/prometheus/prometheus/pp/go/util"
)

// defaultMaxSegmentSize default max segment size for test.
const defaultMaxSegmentSize = uint32(100)

// NoopSegmentWriteNotifier notify when new segment write. [SegmentWriteNotifier] of the implementation.
type NoopSegmentWriteNotifier struct{}

// NotifySegmentIsWritten notify that the segment has been flushed for shard.
// [SegmentWriteNotifier] of the implementation.
func (NoopSegmentWriteNotifier) NotifySegmentIsWritten(uint16) {}

// NotifySegmentWrite notify that the segment is being written for shard. [SegmentWriteNotifier] of the implementation.
func (NoopSegmentWriteNotifier) NotifySegmentWrite(uint16) {}

// Set for shard number of segments. [SegmentWriteNotifier] of the implementation.
func (NoopSegmentWriteNotifier) Set(uint16, uint32) {}

// MakeRecord makes a new [catalog.Record] with the specified number of shards.
func MakeRecord(numberOfShards uint16) *catalog.Record {
	now := time.Now().UnixMilli()
	rec := catalog.NewRecordWithData(
		catalog.DefaultIDGenerator{}.Generate(),
		numberOfShards,
		now,
		now,
		0,
		false,
		0,
		catalog.StatusNew,
		nil,
	)

	return rec
}

// WriteToShardWalFileV1 write to shard wal file v1 the specified number of segments.
//
//revive:disable-next-line:cyclomatic this is test func.
func WriteToShardWalFileV1(
	ctx context.Context,
	shardFilePath string,
	numberOfSegments uint64,
) error {
	//revive:disable-next-line:add-constant // file permissions simple readable as octa-number
	shardFile, err := util.CreateFileAppender(shardFilePath, 0o666)
	if err != nil {
		return fmt.Errorf("failed to create shard file: %w", err)
	}

	shardID := uint16(0)
	lss := shard.NewLSS()
	shardWalEncoder := cppbridge.NewHeadWalEncoder(shardID, 0, lss.Target())

	if _, err = writer.WriteHeader(shardFile, wal.FileFormatVersion, shardWalEncoder.Version()); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	sw, err := writer.NewBuffered(
		shardID,
		shardFile,
		writer.WriteSegment[*cppbridge.HeadEncodedSegment],
		NoopSegmentWriteNotifier{},
		writer.NoopSegmentMarkup{},
	)
	if err != nil {
		return fmt.Errorf("failed to create buffered writer: %w", err)
	}

	wl := wal.NewWal(shardWalEncoder, sw, defaultMaxSegmentSize, shardID, nil)
	defer wl.Close()

	state := cppbridge.NewTransitionStateV2WithoutLock()
	relabeler := cppbridge.NewPerGoroutineRelabeler(1, shardID)
	hLimits := cppbridge.DefaultWALHashdexLimits()

	for i := range numberOfSegments {
		hx, err := (cppbridge.HashdexFactory{}).GoModel([]model.TimeSeries{
			{
				LabelSet:  model.LabelSetFromPairs("__name__", "test"),
				Timestamp: i,
				Value:     float64(i),
			},
		}, hLimits)
		if err != nil {
			return fmt.Errorf("failed to create hashdex: %w", err)
		}

		innerSeries := cppbridge.NewShardedInnerSeries(1).DataByShard(shardID)
		shardsRelabeledSeries := cppbridge.NewShardedRelabeledSeries(1).DataByShard(shardID)

		if err = lss.WithLock(func(target, input *cppbridge.LabelSetStorage) error {
			_, _, rErr := relabeler.Relabeling(
				ctx,
				input,
				target,
				state,
				hx,
				innerSeries,
				shardsRelabeledSeries,
			)
			return rErr
		}); err != nil {
			return err
		}

		if _, err = wl.Write(innerSeries); err != nil {
			return fmt.Errorf("failed to write: %w", err)
		}

		if err = wl.Commit(); err != nil {
			return fmt.Errorf("failed to commit: %w", err)
		}

		if err = wl.Flush(); err != nil {
			return fmt.Errorf("failed to flush: %w", err)
		}

		if err = wl.Sync(); err != nil {
			return fmt.Errorf("failed to sync: %w", err)
		}
	}

	return nil
}
