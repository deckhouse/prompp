package writer

import (
	"slices"
	"sync"
)

// SegmentWriteNotifier notifies that the segment has been written.
type SegmentWriteNotifier struct {
	locker                   sync.Mutex
	written                  []uint32
	synced                   []uint32
	setLastAppendedSegmentID func(segmentID uint32)
}

// NewSegmentWriteNotifier init new [SegmentWriteNotifier].
func NewSegmentWriteNotifier(
	numberOfShards uint16,
	setLastAppendedSegmentID func(segmentID uint32),
) *SegmentWriteNotifier {
	return &SegmentWriteNotifier{
		written:                  make([]uint32, numberOfShards),
		synced:                   make([]uint32, numberOfShards),
		setLastAppendedSegmentID: setLastAppendedSegmentID,
	}
}

// NotifySegmentIsWritten notify that the segment has been flushed for shard.
func (swn *SegmentWriteNotifier) NotifySegmentIsWritten(shardID uint16) {
	swn.locker.Lock()
	defer swn.locker.Unlock()
	swn.synced[shardID] = swn.written[shardID]
	minNumberOfSegments := slices.Min(swn.synced)
	if minNumberOfSegments > 0 {
		swn.setLastAppendedSegmentID(minNumberOfSegments - 1)
	}
}

// NotifySegmentWrite notify that the segment is being written for shard.
func (swn *SegmentWriteNotifier) NotifySegmentWrite(shardID uint16) {
	swn.locker.Lock()
	defer swn.locker.Unlock()
	swn.written[shardID]++
}

// Set for shard number of segments.
func (swn *SegmentWriteNotifier) Set(shardID uint16, numberOfSegments uint32) {
	swn.written[shardID] = numberOfSegments
	swn.synced[shardID] = numberOfSegments
}
