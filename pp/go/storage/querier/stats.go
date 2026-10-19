package querier

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/util/stagestats"
)

const (
	// stageSemaphoreWait waits for head query access, including failed acquisition attempts.
	stageSemaphoreWait stagestats.Stage = iota
	// stagePrepare prepares pooled buffers and matchers for Select.
	stagePrepare
	// stageLSSQuerySelector creates, enqueues, and waits for selector tasks on all shards.
	stageLSSQuerySelector
	// stageLSSQuery queries the snapshots selected by [Querier.Select] or [ChunkQuerier.Select].
	stageLSSQuery
	// stageDataStorageQuery prepares, enqueues, and waits for primary data queries, including load registration.
	stageDataStorageQuery
	// stageDataLoadWait waits for registered data loads after primary queries have completed.
	stageDataLoadWait
	// stageBuildResult builds the returned series set or chunk series set.
	stageBuildResult
	// stageCount sizes the Select stage names and is not a measured stage.
	stageCount
)

const (
	// metadataStageSemaphoreWait waits for head query access, including failed acquisition attempts.
	metadataStageSemaphoreWait stagestats.Stage = iota
	// metadataStagePrepare prepares matchers and the metadata deduplicator.
	metadataStagePrepare
	// metadataStageLSSQuery waits for metadata tasks, handles annotations, and checks the context.
	metadataStageLSSQuery
	// metadataStageBuildResult collects, sorts, and limits the metadata values.
	metadataStageBuildResult
	// metadataStageCount sizes the metadata stage names and is not a measured stage.
	metadataStageCount
)

const (
	// shardStageLSSQuerySelector accounts the maximum duration of Select selector queries across shards.
	shardStageLSSQuerySelector stagestats.Stage = iota
	// shardStageDataStorageQuery accounts the maximum duration of primary data queries across shards.
	shardStageDataStorageQuery
	// shardStageCount sizes the Select shard stage names and is not a measured stage.
	shardStageCount
)

const (
	// metadataShardStageLSSQuery accounts the maximum duration of metadata queries across shards.
	metadataShardStageLSSQuery stagestats.Stage = iota
	// metadataShardStageCount sizes the metadata shard stage names and is not a measured stage.
	metadataShardStageCount
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

// stageNames maps Select stage indices to metric labels in execution order.
var stageNames = [stageCount]string{
	stageSemaphoreWait:    "semaphore_wait",
	stagePrepare:          "prepare",
	stageLSSQuerySelector: "lss_query_selector",
	stageLSSQuery:         "lss_query",
	stageDataStorageQuery: "data_storage_query",
	stageDataLoadWait:     "data_load_wait",
	stageBuildResult:      "build_result",
}

// metadataStageNames maps metadata stage indices to metric labels independently of Select stages.
var metadataStageNames = [metadataStageCount]string{
	metadataStageSemaphoreWait: "semaphore_wait",
	metadataStagePrepare:       "prepare",
	metadataStageLSSQuery:      "lss_query",
	metadataStageBuildResult:   "build_result",
}

// shardStageNames maps Select shard stage indices to metric labels.
var shardStageNames = [shardStageCount]string{
	shardStageLSSQuerySelector: "lss_query_selector",
	shardStageDataStorageQuery: "data_storage_query",
}

// metadataShardStageNames maps metadata shard stage indices to metric labels.
var metadataShardStageNames = [metadataShardStageCount]string{
	metadataShardStageLSSQuery: "lss_query",
}

// operationNames maps operation indices to the fixed operation label values shared by both metric families.
var operationNames = [operationCount]string{
	operationSelectInstant:    "select_instant",
	operationSelectRange:      "select_range",
	operationLabelNames:       "label_names",
	operationLabelValues:      "label_values",
	operationSelectChunks:     "select_chunks",
	operationChunkLabelNames:  "chunk_label_names",
	operationChunkLabelValues: "chunk_label_values",
}

// queryOperation indexes the fixed set of instrumented query operations.
type queryOperation uint8

// queryStats holds the timer, counters, shard durations, and legacy observer for one query invocation.
// Stage counters and the legacy histogram can be enabled independently; its zero value disables measurement.
type queryStats struct {
	// lap accounts consecutive stages on the caller goroutine.
	lap stagestats.Lap
	// shards records the maximum stage duration across shards after their tasks have completed.
	shards stagestats.Stripe
	// slots is a pooled buffer with a separate duration row for each shard; nil disables shard timing.
	slots stagestats.ShardSlots
	// duration observes the legacy histogram; nil for chunk operations or disabled metrics.
	duration prometheus.Observer
	// durationStart is the monotonic start time for the histogram when stage timing is disabled.
	durationStart int64
}

// queryLSSResources holds the completed selector task and pooled buffers until the caller releases its resources.
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
	// duration is the legacy histogram selected during [NewMetrics], or nil when the operation has none.
	duration prometheus.Observer
}

// observeDuration records the legacy histogram in fractional microseconds independently of stage metrics.
// Its clock reading includes temporary resource cleanup and semaphore release.
func (s *queryStats) observeDuration() {
	if s.duration == nil {
		return
	}

	if s.lap.Enabled() {
		s.duration.Observe(s.lap.SinceMicroseconds())

		return
	}

	s.duration.Observe(float64(stagestats.Now()-s.durationStart) / 1e3)
}

// newQueryRecorders registers or reuses caller and shard recorders with the stages supported by the operation.
func newQueryRecorders(r prometheus.Registerer, source string, operation queryOperation) queryRecorders {
	stages := metadataStageNames[:]
	shardStages := metadataShardStageNames[:]

	if operation == operationSelectInstant || operation == operationSelectRange || operation == operationSelectChunks {
		stages = stageNames[:]
		shardStages = shardStageNames[:]
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
