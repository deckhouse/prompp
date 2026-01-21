package services

import (
	"errors"
	"fmt"
)

const (
	// dsMergeOutOfOrderChunks name of task.
	dsMergeOutOfOrderChunks = "data_storage_merge_out_of_order_chunks"

	// dsUnloadUnusedSeriesData name of task
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

// CFSViaRange finalize segment from encoder and add to wal
// and flush wal segment writer, write all buffered data to storage and sync, do via range.
func CFSViaRange[
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

			// if the flush operation fails, skip the Sync
			continue
		}

		if err := shard.WalSync(); err != nil {
			errs = append(errs, fmt.Errorf("sync shard id %d: %w", shard.ShardID(), err))
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
	defer h.ReleaseTask(t)
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
	defer h.ReleaseTask(t)
	h.Enqueue(t)

	return t.Wait()
}
