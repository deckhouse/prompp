package querier_test

import (
	"testing"

	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
)

func TestXxx(t *testing.T) {
	lss := &shard.LSS{}
	ds := shard.NewDataStorage()
	wl := struct{}{}
	sd := shard.NewShard(lss, ds, wl, 0)

	h := head.NewHead([]*shard.Shard[struct{}]{sd})

	querier.NewQuerier(
		h,
		querier.NewNoOpShardedDeduplicator,
		0,
		0,
		nil,
		querier.NewMetrics(nil, "test"),
	)
}
