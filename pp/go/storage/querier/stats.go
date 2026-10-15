package querier

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/util/stagestats"
)

const (
	// stageSemaphoreWait waits for head query access, including failed acquisition attempts.
	stageSemaphoreWait stagestats.Stage = iota
	// stagePrepare prepares pooled buffers, matchers, and the metadata deduplicator.
	stagePrepare
	// stageLSSQuery queries snapshots for Select, or waits for metadata tasks and checks their context.
	stageLSSQuery
	// stageBuildResult builds the returned series set, chunk series set, or metadata values.
	stageBuildResult
	// stageCleanup releases temporary query resources and the head query semaphore.
	stageCleanup
	// stageLSSQuerySelector creates, enqueues, and waits for selector tasks on all shards.
	stageLSSQuerySelector
	// stageDataStorageQuery prepares, enqueues, and waits for primary data queries, including load registration.
	stageDataStorageQuery
	// stageDataLoadWait waits for registered data loads after primary queries have completed.
	stageDataLoadWait
)

const (
	// shardStageLSS accounts the maximum duration of selector or metadata queries across shards.
	shardStageLSS stagestats.Stage = iota
	// shardStageDataStorageQuery accounts the maximum duration of primary data queries across shards.
	shardStageDataStorageQuery
)

const (
	// slotLSS stores each shard's selector or metadata query duration.
	slotLSS = iota
	// slotDataStorageQuery stores each shard's primary data query duration, excluding asynchronous loads.
	slotDataStorageQuery
)

const (
	// operationSelectInstant identifies the instant path selected by [Querier.Select].
	operationSelectInstant queryOperation = iota
	// operationSelectRange identifies the range path selected by [Querier.Select], including function optimizations.
	operationSelectRange
	// operationLabelNames identifies [Querier.LabelNames].
	operationLabelNames
	// operationLabelValues identifies [Querier.LabelValues].
	operationLabelValues
	// operationSelectChunks identifies [ChunkQuerier.Select].
	operationSelectChunks
	// operationChunkLabelNames identifies [ChunkQuerier.LabelNames].
	operationChunkLabelNames
	// operationChunkLabelValues identifies [ChunkQuerier.LabelValues].
	operationChunkLabelValues
	// operationCount sizes the per-operation recorder arrays and is not a query operation.
	operationCount
)

// stageNames maps caller stage indices to metric label values; metadata queries use the prefix through stageCleanup.
var stageNames = []string{
	"semaphore_wait", "prepare", "lss_query", "build_result", "cleanup",
	"lss_query_selector", "data_storage_query", "data_load_wait",
}

// operationNames maps operation indices to the fixed operation label values shared by both metric families.
var operationNames = [operationCount]string{
	"select_instant", "select_range", "label_names", "label_values",
	"select_chunks", "chunk_label_names", "chunk_label_values",
}

// queryOperation indexes the fixed set of instrumented query operations.
type queryOperation uint8

// queryStats holds the timer, counters, and shard durations for one invocation. Its zero value disables measurement.
type queryStats struct {
	// lap accounts consecutive stages on the caller goroutine.
	lap stagestats.Lap
	// shards records the maximum stage duration across shards after their tasks have completed.
	shards stagestats.Stripe
	// slots is a pooled buffer with a separate duration row for each shard; nil disables shard timing.
	slots stagestats.ShardSlots
}

// queryLSSResources holds the completed selector task and pooled buffers until the caller's cleanup stage.
type queryLSSResources[TTask Task] struct {
	// task is returned to the head's task pool during cleanup.
	task TTask
	// selectors is the pooled buffer used for per-shard selector pointers.
	selectors []uintptr
	// errors accumulates per-shard snapshot query errors in a pooled buffer.
	errors []error
}

// queryRecorders shares stage counters across invocations and heads for one source and operation.
type queryRecorders struct {
	// stages accounts elapsed time on the caller goroutine, including waits.
	stages *stagestats.Recorder
	// shards accounts one maximum duration per executed shard stage and invocation.
	shards *stagestats.Recorder
}

// newQueryRecorders registers or reuses caller and shard recorders with the stages supported by the operation.
func newQueryRecorders(r prometheus.Registerer, source string, operation queryOperation) queryRecorders {
	stages := stageNames[:stageCleanup+1]
	shardStages := []string{"lss_query"}

	if operation == operationSelectInstant || operation == operationSelectRange || operation == operationSelectChunks {
		stages = stageNames
		shardStages = []string{"lss_query_selector", "data_storage_query"}
	}

	constLabels := prometheus.Labels{"source": source, "operation": operationNames[operation]}

	return queryRecorders{
		stages: stagestats.NewRecorder(r, stagestats.Opts{
			Name:        "prompp_querier_query_stage",
			Help:        "Head query stages on the caller goroutine, including unsuccessful attempts.",
			ConstLabels: constLabels,
		}, stages),
		shards: stagestats.NewRecorder(r, stagestats.Opts{
			Name: "prompp_querier_query_shard_stage",
			Help: "Head query stages on shards, maximum per invocation, " +
				"including unsuccessful attempts.",
			ConstLabels: constLabels,
		}, shardStages),
	}
}
