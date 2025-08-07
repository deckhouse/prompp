package querier

import (
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

// NoOpShardedDeduplicator container for accumulating values without deduplication.
type NoOpShardedDeduplicator struct {
	shardedValues [][]string
	// TODO snapshots any
	snapshots []*cppbridge.LabelSetSnapshot
	count     uint64
}

// NewNoOpShardedDeduplicator int new [NoOpShardedDeduplicator].
func NewNoOpShardedDeduplicator(numberOfShards uint16) Deduplicator {
	return &NoOpShardedDeduplicator{
		shardedValues: make([][]string, numberOfShards),
		snapshots:     make([]*cppbridge.LabelSetSnapshot, numberOfShards),
	}
}

// Add values to deduplicator by shard ID.
func (d *NoOpShardedDeduplicator) Add(shardID uint16, snapshot *cppbridge.LabelSetSnapshot, values []string) {
	d.shardedValues[shardID] = make([]string, len(values))
	n := copy(d.shardedValues[shardID], values)
	atomic.AddUint64(&d.count, uint64(n)) // #nosec G115 // no overflow
	d.snapshots[shardID] = snapshot
}

// Values returns collected values.
func (d *NoOpShardedDeduplicator) Values() []string {
	values := make([]string, 0, d.count)
	for _, shardedValues := range d.shardedValues {
		for _, v := range shardedValues {
			values = append(values, strings.Clone(v))
		}
	}
	runtime.KeepAlive(d.snapshots)

	return values
}
