package appender

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/util/stagestats"
)

// Stages of the [Appender.Append] on the caller goroutine, consecutive and without gaps.
const (
	// StageSemaphoreWait waiting for the semaphore of the active head, marked by the caller.
	StageSemaphoreWait stagestats.Stage = iota
	// StageInputRelabeling input relabeling on the shards, including waiting in the shard queues.
	StageInputRelabeling
	// StageAppendRelabeled appending of the relabeled series on the shards.
	StageAppendRelabeled
	// StageUpdateCache updating of the relabeler caches.
	StageUpdateCache
	// StageStaleNan tracking of the stale NaNs.
	StageStaleNan
	// StageAppendDataWal appending to the data storage and to the wal on the shards.
	StageAppendDataWal
	// StageWalCommitFlush commit and flush of the wal.
	StageWalCommitFlush
)

// stageNames names of the stages on the caller goroutine.
var stageNames = []string{
	"semaphore_wait",
	"input_relabeling",
	"append_relabeled",
	"update_cache",
	"stale_nan",
	"append_data_wal",
	"wal_commit_flush",
}

// Stages of the [Appender.Append] on the shards, the maximum over the shards per append.
const (
	// shardStageRORelabelingHit read-only relabeling from the cache succeeded on all shards.
	shardStageRORelabelingHit stagestats.Stage = iota
	// shardStageRORelabelingMiss read-only relabeling from the cache failed on at least one shard.
	shardStageRORelabelingMiss
	// shardStageRelabeling relabeling on the shards where the read-only relabeling failed.
	shardStageRelabeling
	// shardStageAppendRelabeled appending of the relabeled series.
	shardStageAppendRelabeled
	// shardStageDataStorageAppend appending to the data storage.
	shardStageDataStorageAppend
	// shardStageWalWrite appending to the wal.
	shardStageWalWrite
)

// shardStageNames names of the stages on the shards.
var shardStageNames = []string{
	"ro_relabeling_hit",
	"ro_relabeling_miss",
	"relabeling",
	"append_relabeled",
	"data_storage_append",
	"wal_write",
}

// Slots of the shard durations in [stagestats.ShardSlots].
const (
	slotRORelabeling = iota
	slotRelabeling
	slotAppendRelabeled
	slotDataStorageAppend
	slotWalWrite
)

// Recorders the stage stats recorders of the [Appender.Append] into the active head.
type Recorders struct {
	// Stages accounts the stages on the caller goroutine.
	Stages *stagestats.Recorder
	// Shards accounts the stages on the shards.
	Shards *stagestats.Recorder
}

// NewRecorders init new [Recorders] registered in r.
func NewRecorders(r prometheus.Registerer) Recorders {
	return Recorders{
		Stages: stagestats.NewRecorder(
			r,
			stagestats.Opts{
				Name: "prompp_head_append_stage",
				Help: "Append to the active head stages on the caller goroutine.",
			},
			stageNames,
		),
		Shards: stagestats.NewRecorder(
			r,
			stagestats.Opts{
				Name: "prompp_head_append_shard_stage",
				Help: "Append to the active head stages on the shards, the maximum over the shards per append.",
			},
			shardStageNames,
		),
	}
}

// Stats the stage stats of one [Appender.Append]. The zero [Stats] accounts nothing.
type Stats struct {
	// Lap accounts the stages on the caller goroutine, started by the caller before waiting for the semaphore.
	// It is held by value, so that the caller's [stagestats.Lap] does not escape to the heap.
	Lap stagestats.Lap
	// Shards accounts the stages on the shards.
	Shards *stagestats.Recorder
}
