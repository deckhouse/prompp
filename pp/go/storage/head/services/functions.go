package services

import (
	"errors"
	"fmt"
	"runtime"

	"golang.org/x/sync/errgroup"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

const (
	// dsMergeOutOfOrderChunks name of task.
	dsMergeOutOfOrderChunks = "data_storage_merge_out_of_order_chunks"

	// dsUnloadUnusedSeriesData name of task.
	dsUnloadUnusedSeriesData = "data_storage_unload_unused_series_data"
)

//
// Commit, Flush, Sync
//

// CFViaRange finalize segment from encoder and add to wal
// and flush wal segment writer, write all buffered data to storage without sync, do via range.
func CFViaRange[
	TShard Shard,
	THead RangeHead[TShard],
](h THead) error {
	// we hope that there will be no mistakes, positive expectations
	var errs []error
	for _, shard := range h.Shards() {
		if err := shard.WalCommit(); err != nil {
			errs = append(errs, fmt.Errorf("commit shard id %d: %w", shard.ShardID(), err))
		}

		if err := shard.WalFlush(); err != nil {
			errs = append(errs, fmt.Errorf("flush shard id %d: %w", shard.ShardID(), err))
		}
	}

	return errors.Join(errs...)
}

// LongCFSViaRange same as [CFSViaRange], but finalize segment from encoder via long commit,
// intended for a long-running finalization (e.g. after copying all added series into a new head).
// Shards are independent, so they are processed concurrently, but at most GOMAXPROCS at a time:
// every long commit is a blocking cgo call occupying an OS thread and competing with ingestion.
// The sync is done here too, so that it does not slow down the first commit of the new segment on the hot path.
func LongCFSViaRange[
	TShard Shard,
	THead RangeHead[TShard],
](h THead) error {
	shards := h.Shards()
	errs := make([]error, len(shards))

	var g errgroup.Group
	g.SetLimit(runtime.GOMAXPROCS(0))
	for i := range shards {
		g.Go(func() error {
			errs[i] = cfsShard(shards[i], func(s TShard) error { return s.WalLongCommit() })
			// errors are collected per shard, so that one failed shard does not hide the others
			return nil
		})
	}
	_ = g.Wait()

	return errors.Join(errs...)
}

// CFSViaRange finalize segment from encoder and add to wal
// and flush wal segment writer, write all buffered data to storage and sync, do via range.
func CFSViaRange[
	TShard Shard,
	THead RangeHead[TShard],
](h THead) error {
	// we hope that there will be no mistakes, positive expectations
	var errs []error
	for _, shard := range h.Shards() {
		errs = append(errs, cfsShard(shard, func(s TShard) error { return s.WalCommit() }))
	}

	return errors.Join(errs...)
}

// cfsShard finalize segment from encoder via commit and add to wal
// and flush wal segment writer, write all buffered data to storage and sync for one [Shard].
func cfsShard[TShard Shard](shard TShard, commit func(TShard) error) error {
	var errs []error
	if err := commit(shard); err != nil {
		errs = append(errs, fmt.Errorf("commit shard id %d: %w", shard.ShardID(), err))
	}

	if err := shard.WalFlush(); err != nil {
		errs = append(errs, fmt.Errorf("flush shard id %d: %w", shard.ShardID(), err))

		// if the flush operation fails, skip the Sync
		return errors.Join(errs...)
	}

	if err := shard.WalSync(); err != nil {
		errs = append(errs, fmt.Errorf("sync shard id %d: %w", shard.ShardID(), err))
	}

	return errors.Join(errs...)
}

// CloseWals closes all WALs of the [Head].
func CloseWals[
	TShard Shard,
	THead RangeHead[TShard],
](h THead) error {
	var errs []error
	for _, shard := range h.Shards() {
		if err := shard.CloseWal(); err != nil {
			errs = append(errs, fmt.Errorf("close wal shard id %d: %w", shard.ShardID(), err))
		}
	}

	return errors.Join(errs...)
}

// ReleaseIngestionStructures releases label set -> ls id hash set of lss and drops input lss for all shards.
// Attention: works only with QueryableEncodingBimap type of LSS. After release lss can't find or add label sets,
// so it's allowed only for read-only head.
func ReleaseIngestionStructures[
	TShard Shard,
	THead RangeHead[TShard],
](h THead) {
	for _, shard := range h.Shards() {
		shard.LSSReleaseIngestionStructures()
	}
}

// ReleaseLSIDSet releases sorted ls id set and label set -> ls id hash set of lss for all shards,
// sorting index is built beforehand.
// Attention: works only with QueryableEncodingBimap type of LSS. After release lss can't find or add label sets and
// ls id set is empty, so it's allowed only for read-only head after chunk recoding and data loading are done.
func ReleaseLSIDSet[
	TShard Shard,
	THead RangeHead[TShard],
](h THead) {
	for _, shard := range h.Shards() {
		shard.LSSReleaseLSIDSet()
	}
}

//
// UnloadUnusedSeriesDataWithHead
//

// UnloadUnusedSeriesDataWithHead unload unused series data for [Head].
func UnloadUnusedSeriesDataWithHead[
	TTask Task,
	TShard, TGShard Shard,
	THead Head[TTask, TShard, TGShard],
](h THead) error {
	t := h.CreateTask(
		dsUnloadUnusedSeriesData,
		func(shard TGShard) error {
			return shard.UnloadUnusedSeriesData()
		},
	)
	defer h.PutTask(t)
	h.Enqueue(t)

	return t.Wait()
}

//
// MergeOutOfOrderChunksWithHead
//

// MergeOutOfOrderChunksWithHead merge chunks with out of order data chunks for [Head].
func MergeOutOfOrderChunksWithHead[
	TTask Task,
	TShard, TGShard Shard,
	THead Head[TTask, TShard, TGShard],
](h THead) error {
	t := h.CreateTask(
		dsMergeOutOfOrderChunks,
		func(shard TGShard) error {
			shard.MergeOutOfOrderChunks()

			return nil
		},
	)
	defer h.PutTask(t)
	h.Enqueue(t)

	return t.Wait()
}

//
// HeadTimeInterval
//

// HeadTimeInterval returns the time interval of the [Head] data across all shards,
// the interval is invalid if the [Head] has no data.
func HeadTimeInterval[
	TShard Shard,
	THead RangeHead[TShard],
](h THead) cppbridge.TimeInterval {
	timeInterval := cppbridge.NewInvalidTimeInterval()
	for shard := range h.RangeShards() {
		interval := shard.TimeInterval(false)
		timeInterval.MinT = min(interval.MinT, timeInterval.MinT)
		timeInterval.MaxT = max(interval.MaxT, timeInterval.MaxT)
	}

	return timeInterval
}
