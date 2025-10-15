package writer

import (
	"slices"
	"sync"
)

// SegmentWriteNotifier notifies that the segment has been written.
type SegmentWriteNotifier struct {
	locker                   sync.Mutex
	shards                   []uint32
	setLastAppendedSegmentID func(segmentID uint32)
}

// NewSegmentWriteNotifier init new [SegmentWriteNotifier].
func NewSegmentWriteNotifier(
	numberOfShards uint16,
	setLastAppendedSegmentID func(segmentID uint32),
) *SegmentWriteNotifier {
	return &SegmentWriteNotifier{
		shards:                   make([]uint32, numberOfShards),
		setLastAppendedSegmentID: setLastAppendedSegmentID,
	}
}

// NotifySegmentIsWritten notify that the segment has been written for shard.
func (swn *SegmentWriteNotifier) NotifySegmentIsWritten() {
	swn.locker.Lock()
	defer swn.locker.Unlock()
	minNumberOfSegments := slices.Min(swn.shards)
	if minNumberOfSegments > 0 {
		swn.setLastAppendedSegmentID(minNumberOfSegments - 1)
	}
}

func (swn *SegmentWriteNotifier) NotifySegmentWrite(shardID uint16) {
	swn.locker.Lock()
	defer swn.locker.Unlock()
	swn.shards[shardID]++
}

// Set for shard number of segments.
func (swn *SegmentWriteNotifier) Set(shardID uint16, numberOfSegments uint32) {
	swn.shards[shardID] = numberOfSegments
}
