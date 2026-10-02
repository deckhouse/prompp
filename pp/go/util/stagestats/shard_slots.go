package stagestats

//
// ShardSlots
//

// SlotsPerShard is the number of duration slots of a shard in [ShardSlots], a row is one cache line.
const SlotsPerShard = 8

// ShardSlots holds the durations measured on each shard within one pipeline execution: every shard writes its own
// row without synchronization, and the caller reads them after waiting for the shards.
type ShardSlots [][SlotsPerShard]int64

// NewShardSlots init new [ShardSlots] for the number of shards.
func NewShardSlots(numberOfShards uint16) ShardSlots {
	return make(ShardSlots, numberOfShards)
}

// Set stores the duration d of the slot of the shard.
func (s ShardSlots) Set(shardID uint16, slot int, d int64) {
	s[shardID][slot] = d
}

// Start returns the current time from [Now] to measure a slot, or 0 without reading the clock for nil [ShardSlots].
func (s ShardSlots) Start() int64 {
	if s == nil {
		return 0
	}

	return Now()
}

// Since stores the time since start, obtained from [ShardSlots.Start], to the slot of the shard and returns
// the current time to measure the next slot. Nil [ShardSlots] store nothing and do not read the clock.
func (s ShardSlots) Since(shardID uint16, slot int, start int64) int64 {
	if s == nil {
		return 0
	}

	now := Now()
	s[shardID][slot] = now - start

	return now
}

// Max returns the maximum duration of the slot over the shards, 0 if no shard has set it.
func (s ShardSlots) Max(slot int) int64 {
	var maxDuration int64
	for i := range s {
		maxDuration = max(maxDuration, s[i][slot])
	}

	return maxDuration
}

// Reset zeroes all slots.
func (s ShardSlots) Reset() {
	clear(s)
}
