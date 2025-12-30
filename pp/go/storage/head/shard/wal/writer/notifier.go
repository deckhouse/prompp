package writer

import (
	"math"
	"sync/atomic"
)

// SegmentWriteNotifier notifies that the segment has been written.
type SegmentWriteNotifier struct {
	writtenSegments          []uint32
	syncedSegments           []uint32
	setLastAppendedSegmentID func(segmentID uint32)
}

// NewSegmentWriteNotifier init new [SegmentWriteNotifier].
func NewSegmentWriteNotifier(
	numberOfShards uint16,
	setLastAppendedSegmentID func(segmentID uint32),
) *SegmentWriteNotifier {
	if numberOfShards == 0 {
		panic("numberOfShards must be greater than 0")
	}
	return &SegmentWriteNotifier{
		writtenSegments:          make([]uint32, numberOfShards),
		syncedSegments:           make([]uint32, numberOfShards),
		setLastAppendedSegmentID: setLastAppendedSegmentID,
	}
}

// NotifySegmentIsWritten notify that the segment has been flushed for shard.
//
// This method is thread-safe between different shards.
// For the same shard, it is a caller responsibility to ensure thread-safety.
// Also it is not thread-safe with [NotifySegmentWrite] for the same shard.
//
// The callback may be called several times with the same segmentID.
func (swn *SegmentWriteNotifier) NotifySegmentIsWritten(shardID uint16) {
	atomic.StoreUint32(&swn.syncedSegments[shardID], swn.writtenSegments[shardID])
	var minNumberOfSegments uint32 = math.MaxUint32
	for i := range swn.syncedSegments {
		x := atomic.LoadUint32(&swn.syncedSegments[i])
		if x < minNumberOfSegments {
			minNumberOfSegments = x
		}
	}
	if minNumberOfSegments > 0 {
		swn.setLastAppendedSegmentID(minNumberOfSegments - 1)
	}
}

// NotifySegmentWrite notify that the segment is being written for shard.
//
// This method is thread-safe between different shards.
// For the same shard, it is a caller responsibility to ensure thread-safety.
// Also it is not thread-safe with [NotifySegmentIsWritten] for the same shard.
func (swn *SegmentWriteNotifier) NotifySegmentWrite(shardID uint16) {
	swn.writtenSegments[shardID]++
}

// Set for shard number of segments.
func (swn *SegmentWriteNotifier) Set(shardID uint16, numberOfSegments uint32) {
	swn.writtenSegments[shardID] = numberOfSegments
	swn.syncedSegments[shardID] = numberOfSegments
}
