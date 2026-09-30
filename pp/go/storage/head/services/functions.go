package services

import (
	"errors"
	"fmt"
	"sync"
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
// Shards are independent, so they are processed concurrently. The sync is done here too,
// so that it does not slow down the first commit of the new segment on the hot path.
func LongCFSViaRange[
	TShard Shard,
	THead RangeHead[TShard],
](h THead) error {
	shards := h.Shards()
	errs := make([]error, len(shards))

	var wg sync.WaitGroup
	for i := range shards {
		wg.Go(func() {
			errs[i] = cfsShard(shards[i], func(s TShard) error { return s.WalLongCommit() })
		})
	}
	wg.Wait()

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
