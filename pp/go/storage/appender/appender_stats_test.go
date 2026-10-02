package appender_test

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/appender"
	"github.com/prometheus/prometheus/pp/go/storage/head/services"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
)

// stageExecutions the number of executions by stage of a stage stats family.
type stageExecutions map[string]float64

// appendWithStats appends the time series to the head accounting the stages in the recorders.
func (s *AppenderSuite) appendWithStats(
	ctx context.Context,
	recorders appender.Recorders,
	state *cppbridge.StateV2,
	timeSeries []model.TimeSeries,
	commitToWal bool,
) error {
	_, err := appender.New(
		s.head,
		services.CFViaRange[*shard.Shard, *storage.Head],
		appender.Stats{Lap: recorders.Stages.Start(), Shards: recorders.Shards},
	).Append(ctx, storagetest.NewIncomingData(&s.Suite, timeSeries), state, commitToWal)

	return err
}

// executions returns the number of executions by stage of the family in the registry, stages without
// executions are omitted.
func (s *AppenderSuite) executions(registry *prometheus.Registry, family string) stageExecutions {
	families, err := registry.Gather()
	s.Require().NoError(err)

	result := stageExecutions{}
	for _, f := range families {
		if f.GetName() != family {
			continue
		}

		for _, m := range f.GetMetric() {
			if v := m.GetCounter().GetValue(); v > 0 {
				result[m.GetLabel()[0].GetValue()] = v
			}
		}
	}

	return result
}

func (s *AppenderSuite) TestStatsAppendFromCacheWithoutCommit() {
	// Arrange
	registry := prometheus.NewRegistry()
	recorders := appender.NewRecorders(registry)
	state := s.createState([]*cppbridge.RelabelConfig{})
	_, err := s.appender.Append(context.Background(), storagetest.NewIncomingData(&s.Suite, s.metric1(1)), state, false)
	s.Require().NoError(err)

	// Act
	err = s.appendWithStats(context.Background(), recorders, state, s.metric1(2), false)

	// Assert
	s.Require().NoError(err)
	s.Equal(stageExecutions{
		"input_relabeling": 1,
		"append_data_wal":  1,
	}, s.executions(registry, "prompp_head_append_stage_executions_total"))
	s.Equal(stageExecutions{
		"ro_relabeling_hit":   1,
		"data_storage_append": 1,
		"wal_write":           1,
	}, s.executions(registry, "prompp_head_append_shard_stage_executions_total"))
}

func (s *AppenderSuite) TestStatsAppendRelabeledWithStalenessAndCommit() {
	// Arrange
	registry := prometheus.NewRegistry()
	recorders := appender.NewRecorders(registry)
	state := s.createState([]*cppbridge.RelabelConfig{{
		TargetLabel: "label_for_drop",
		Action:      cppbridge.Replace,
		Separator:   ";",
		Replacement: "keep1",
	}})
	state.EnableTrackStaleness()

	// Act
	err := s.appendWithStats(context.Background(), recorders, state, s.metric1(1), true)

	// Assert
	s.Require().NoError(err)
	s.Equal(stageExecutions{
		"input_relabeling": 1,
		"append_relabeled": 1,
		"update_cache":     1,
		"stale_nan":        1,
		"append_data_wal":  1,
		"wal_commit_flush": 1,
	}, s.executions(registry, "prompp_head_append_stage_executions_total"))
	s.Equal(stageExecutions{
		"ro_relabeling_miss":  1,
		"relabeling":          1,
		"append_relabeled":    1,
		"data_storage_append": 1,
		"wal_write":           1,
	}, s.executions(registry, "prompp_head_append_shard_stage_executions_total"))
}

func (s *AppenderSuite) TestStatsAppendFailedOnInputRelabeling() {
	// Arrange
	registry := prometheus.NewRegistry()
	recorders := appender.NewRecorders(registry)
	state := s.createState([]*cppbridge.RelabelConfig{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Act
	err := s.appendWithStats(ctx, recorders, state, s.metric1(1), false)

	// Assert
	s.Require().ErrorIs(err, context.Canceled)
	s.Empty(s.executions(registry, "prompp_head_append_stage_executions_total"))
	s.Empty(s.executions(registry, "prompp_head_append_shard_stage_executions_total"))
}

// metric1 returns one sample of the series metric1 at the timestamp.
func (*AppenderSuite) metric1(timestamp uint64) []model.TimeSeries {
	return []model.TimeSeries{{
		LabelSet:  model.NewLabelSetBuilder().Set("__name__", "metric1").Build(),
		Timestamp: timestamp,
		Value:     1.1,
	}}
}
