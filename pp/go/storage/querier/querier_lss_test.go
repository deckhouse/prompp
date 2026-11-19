package querier

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
	"github.com/prometheus/prometheus/pp/go/storage/head/task"
)

// stagedQueryHead dispatches the second shard only after inspecting the first shard's result.
type stagedQueryHead struct {
	*head.Head[*shard.Shard, *shard.Shard]
	betweenShards func()
}

// Enqueue finishes the first shard before letting the second start, without relying on worker timing.
func (h *stagedQueryHead) Enqueue(t *task.Generic[*shard.Shard]) {
	t.SetShardsNumber(h.NumberOfShards())
	shards := h.Shards()
	t.ExecuteOnShard(shards[0])
	h.betweenShards()
	t.ExecuteOnShard(shards[1])
}

func TestQueryLssFinishesShardBeforeOtherSelectors(t *testing.T) {
	t.Parallel()

	// Arrange
	const metricName = "snapshot_probe"
	const shardCount uint16 = 2
	shards := make([]*shard.Shard, shardCount)
	ids := make([]uint32, shardCount)
	for i := range shardCount {
		lss := shard.NewLSS()
		require.NoError(t, lss.WithLock(func(target, _ *cppbridge.LabelSetStorage) error {
			ids[i] = target.FindOrEmplace(model.NewLabelSetBuilder().Set(labels.MetricName, metricName).Build()).LabelSetID

			return nil
		}))
		shards[i] = shard.NewShard(lss, nil, nil, nil, wal.NewNoopWal(), i)
	}

	raw := head.NewHead("snapshot-query", shards, func(s *shard.Shard, _ uint16) *shard.Shard { return s }, nil, 0, nil)
	t.Cleanup(func() { require.NoError(t, raw.Close()) })
	h := &stagedQueryHead{Head: raw}
	resources := newQueryResources(raw.PoolProvider(), raw, queryStats{}, func() {})
	defer resources.close()

	var firstResult *cppbridge.LSSQueryResult
	h.betweenShards = func() { firstResult = resources.lssQueryResults[0] }
	matcher := labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, metricName)

	// Act
	err := queryLss(lssQueryRangeQuerySelector, h, []*labels.Matcher{matcher}, true, &resources)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, firstResult)
	require.Equal(t, ids[:1], firstResult.IDs())
	require.Equal(t, ids[1:], resources.lssQueryResults[1].IDs())
}
