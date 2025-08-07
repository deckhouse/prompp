package head_test

import (
	"testing"

	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
)

func TestXxx(t *testing.T) {
	lss := &shard.LSS{}
	ds := shard.NewDataStorage()
	wl := struct{}{}
	sd := shard.NewShard(lss, ds, wl, 0)

	h := head.NewHead([]head.Shard{sd})
	_ = h
}
