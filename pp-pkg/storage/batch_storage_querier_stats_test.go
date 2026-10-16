package storage

import (
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
)

func (s *BatchStorageSuite) TestQuerierStageStats_TransactionHeadExcluded() {
	bs := s.adapter.BatchStorage().(*BatchStorage)
	_, err := bs.AppendTimeSeries(s.ctx, s.statsBatch(), s.state, false)
	s.Require().NoError(err)
	before := s.queryStageExecutionCount()
	q, err := bs.Querier(0, 2000)
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.Require().NoError(q.Close()) })
	matcher := labels.MustNewMatcher(labels.MatchEqual, "__name__", "stats_metric")
	result := q.Select(s.ctx, false, &storage.SelectHints{}, matcher)
	s.Require().NoError(result.Err())
	s.True(result.Next())
	_, _, err = q.LabelNames(s.ctx, &storage.LabelHints{}, matcher)
	s.Require().NoError(err)
	values, _, err := q.LabelValues(s.ctx, "job", &storage.LabelHints{}, matcher)
	s.Require().NoError(err)
	s.Equal([]string{"stats"}, values)
	s.Equal(before, s.queryStageExecutionCount())
}

func (s *BatchStorageSuite) queryStageExecutionCount() float64 {
	s.T().Helper()
	families, err := s.registry.Gather()
	s.Require().NoError(err)
	var count float64

	for _, family := range families {
		if family.GetName() != "prompp_querier_query_stage_executions_total" &&
			family.GetName() != "prompp_querier_query_shard_stage_executions_total" {
			continue
		}

		for _, metric := range family.GetMetric() {
			count += metric.GetCounter().GetValue()
		}
	}

	return count
}
