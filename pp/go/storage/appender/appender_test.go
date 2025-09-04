package appender_test

import (
	"testing"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/appender"
	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/head/services"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
)

func TestXxx(t *testing.T) {
	lss := &shard.LSS{}
	ds := shard.NewDataStorage()
	wl := &testWal{}
	sd := shard.NewShard(lss, ds, wl, 0)
	id := "test-head-id"
	generation := uint64(0)

	h := head.NewHead(
		id,
		[]*shard.Shard[*testWal]{sd},
		shard.NewPerGoroutineShard[*testWal],
		nil,
		generation,
		nil,
	)

	app := appender.New(h, services.CommitAndFlushViaRange)

	_ = app

	t.Log("end")
}

// testWal test implementation wal.
type testWal struct{}

// Close test implementation wal.
func (*testWal) Close() error {
	return nil
}

// Commit test implementation wal.
func (*testWal) Commit() error {
	return nil
}

// Flush test implementation wal.
func (*testWal) Flush() error {
	return nil
}

// Write test implementation wal.
func (*testWal) Write(_ []*cppbridge.InnerSeries) (bool, error) {
	return false, nil
}
