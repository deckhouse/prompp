package services

import (
	"errors"
	"fmt"
)

const (
	// dsMergeOutOfOrderChunks name of task.
	dsMergeOutOfOrderChunks = "data_storage_merge_out_of_order_chunks"
)

// TODO appender Commit

//
// CommitAndFlushViaRange
//

// CommitAndFlushViaRange finalize segment from encoder and add to wal
// and flush wal segment writer, write all buffered data to storage, do via range.
func CommitAndFlushViaRange[
	TTask Task,
	TShard, TGoShard Shard,
	THead Head[TTask, TShard, TGoShard],
](h THead) error {
	errs := make([]error, 0, h.NumberOfShards()*2)
	for shard := range h.RangeShards() {
		if err := shard.WalCommit(); err != nil {
			errs = append(errs, fmt.Errorf("commit shard id %d: %w", shard.ShardID(), err))
		}

		if err := shard.WalFlush(); err != nil {
			errs = append(errs, fmt.Errorf("flush shard id %d: %w", shard.ShardID(), err))
		}
	}

	return errors.Join(errs...)
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
	h.Enqueue(t)

	return t.Wait()
}

// // commitAndFlushViaTask finalize segment from encoder and add to wal
// // and flush wal segment writer, write all buffered data to storage, do via task.
// func commitAndFlushViaTask[
// 	TTask Task,
// 	TDataStorage DataStorage,
// 	TLSS LSS,
// 	TShard, TGoShard Shard[TDataStorage, TLSS],
// 	THead Head[TTask, TDataStorage, TLSS, TShard, TGoShard],
// ](h THead) error {
// 	t := h.CreateTask(
// 		WalCommit,
// 		func(shard TGoShard) error {
// 			swal := shard.Wal()

// 			// wal contains LSS and it is necessary to lock the LSS for reading for the commit.
// 			if err := shard.LSS().WithRLock(func(_, _ *cppbridge.LabelSetStorage) error {
// 				return swal.Commit()
// 			}); err != nil {
// 				return err
// 			}

// 			return swal.Flush()
// 		},
// 	)
// 	h.Enqueue(t)

// 	return t.Wait()
// }
