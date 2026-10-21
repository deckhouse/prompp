package querier

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func TestQueryStageLabels(t *testing.T) {
	for _, tc := range []struct {
		name      string
		operation queryOperation
		observe   func(queryRecorders)
	}{
		{"select_instant", operationSelectInstant, observeSelectStages},
		{"select_range", operationSelectRange, observeSelectStages},
		{"select_chunks", operationSelectChunks, observeSelectStages},
		{"label_names", operationLabelNames, observeMetadataStages},
		{"label_values", operationLabelValues, observeMetadataStages},
		{"chunk_label_names", operationChunkLabelNames, observeMetadataStages},
		{"chunk_label_values", operationChunkLabelValues, observeMetadataStages},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			registry := prometheus.NewRegistry()
			recorders := newQueryRecorders(registry, QueryableAppenderSource, tc.operation)
			expected, err := os.ReadFile(filepath.Join("testdata", "query_stage_labels", tc.name+".prom"))
			require.NoError(t, err)

			// Act
			tc.observe(recorders)

			// Assert
			require.NoError(t, testutil.CollectAndCompare(registry, bytes.NewReader(expected)))
		})
	}
}

// observeMetadataStages records distinct durations for each caller and shard metadata stage.
func observeMetadataStages(recorders queryRecorders) {
	caller := recorders.stages.Stripe(0)
	caller.Observe(metadataStageSemaphoreWait, 1)
	caller.Observe(metadataStagePrepare, 2)
	caller.Observe(metadataStageLSSQuery, 3)
	caller.Observe(metadataStageBuildResult, 4)

	shards := recorders.shards.Stripe(0)
	shards.Observe(metadataShardStageLSSQuery, 1)
}

// observeSelectStages records distinct durations for each caller and shard Select stage.
func observeSelectStages(recorders queryRecorders) {
	caller := recorders.stages.Stripe(0)
	caller.Observe(stageSemaphoreWait, 1)
	caller.Observe(stagePrepare, 2)
	caller.Observe(stageLSSQuerySelector, 3)
	caller.Observe(stageLSSQuery, 4)
	caller.Observe(stageDataStorageQuery, 5)
	caller.Observe(stageDataLoadWait, 6)
	caller.Observe(stageBuildResult, 7)

	shards := recorders.shards.Stripe(0)
	shards.Observe(shardStageLSSQuerySelector, 1)
	shards.Observe(shardStageDataStorageQuery, 2)
}
