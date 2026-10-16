package querier

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pp/go/util/stagestats"
)

func TestQueryStageLabels(t *testing.T) {
	for _, group := range []struct {
		operations map[queryOperation]string
		caller     map[stagestats.Stage]string
		shards     map[stagestats.Stage]string
	}{
		{
			operations: map[queryOperation]string{
				operationSelectInstant: "select_instant",
				operationSelectRange:   "select_range",
				operationSelectChunks:  "select_chunks",
			},
			caller: map[stagestats.Stage]string{
				stageSemaphoreWait:    "semaphore_wait",
				stagePrepare:          "prepare",
				stageLSSQuerySelector: "lss_query_selector",
				stageLSSQuery:         "lss_query",
				stageDataStorageQuery: "data_storage_query",
				stageDataLoadWait:     "data_load_wait",
				stageBuildResult:      "build_result",
				stageCleanup:          "cleanup",
			},
			shards: map[stagestats.Stage]string{
				shardStageLSSQuerySelector: "lss_query_selector",
				shardStageDataStorageQuery: "data_storage_query",
			},
		},
		{
			operations: map[queryOperation]string{
				operationLabelNames:       "label_names",
				operationLabelValues:      "label_values",
				operationChunkLabelNames:  "chunk_label_names",
				operationChunkLabelValues: "chunk_label_values",
			},
			caller: map[stagestats.Stage]string{
				metadataStageSemaphoreWait: "semaphore_wait",
				metadataStagePrepare:       "prepare",
				metadataStageLSSQuery:      "lss_query",
				metadataStageBuildResult:   "build_result",
				metadataStageCleanup:       "cleanup",
			},
			shards: map[stagestats.Stage]string{
				metadataShardStageLSSQuery: "lss_query",
			},
		},
	} {
		for operation, name := range group.operations {
			t.Run(name, func(t *testing.T) {
				registry := prometheus.NewRegistry()
				recorders := newQueryRecorders(registry, QueryableAppenderSource, operation)
				expected := make(map[string]map[string]float64)

				for _, pipeline := range []struct {
					name     string
					recorder *stagestats.Recorder
					labels   map[stagestats.Stage]string
				}{
					{"prompp_querier_query_stage", recorders.stages, group.caller},
					{"prompp_querier_query_shard_stage", recorders.shards, group.shards},
				} {
					durations := make(map[string]float64)
					counts := make(map[string]float64)
					stripe := pipeline.recorder.RandomStripe()

					for stage, label := range pipeline.labels {
						// Distinct durations reveal swapped labels even when each stage executes once.
						duration := int64(stage) + 1
						stripe.Observe(stage, duration)
						durations[label] = float64(duration)
						counts[label] = 1
					}

					expected[pipeline.name+"_duration_nanoseconds_total"] = durations
					expected[pipeline.name+"_executions_total"] = counts
				}

				families, err := registry.Gather()
				require.NoError(t, err)
				actual := make(map[string]map[string]float64)

				for _, family := range families {
					values := make(map[string]float64)

					for _, metric := range family.GetMetric() {
						labels := make(map[string]string)

						for _, label := range metric.GetLabel() {
							labels[label.GetName()] = label.GetValue()
						}

						require.Equal(t, map[string]string{
							"operation": name, "source": QueryableAppenderSource, "stage": labels["stage"],
						}, labels)
						values[labels["stage"]] = metric.GetCounter().GetValue()
					}

					actual[family.GetName()] = values
				}

				require.Equal(t, expected, actual)
			})
		}
	}
}
