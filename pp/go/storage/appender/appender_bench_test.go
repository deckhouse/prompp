package appender_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/appender"
	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
)

const (
	benchNumberOfShards uint16 = 2
)

// Head type alias for benchmark.
type benchHead = head.Head[*shard.Shard, *shard.PerGoroutineShard]

var benchMetrics = []byte(`# HELP go_gc_duration_seconds A summary of the GC invocation durations.
# 	TYPE go_gc_duration_seconds summary
go_gc_duration_seconds{quantile="0"} 4.9351e-05
go_gc_duration_seconds{quantile="0.25",} 7.424100000000001e-05
go_gc_duration_seconds{quantile="0.5",a="b"} 8.3835e-05
go_gc_duration_seconds{quantile="0.8", a="b"} 8.3835e-05
go_gc_duration_seconds{ quantile="0.9", a="b"} 8.3835e-05
# Hrandom comment starting with prefix of HELP
#
wind_speed{A="2",c="3"} 12345
# comment with escaped \n newline
# comment with escaped \ escape character
# HELP nohelp1
# HELP nohelp2
go_gc_duration_seconds{ quantile="1.0", a="b" } 8.3835e-05
go_gc_duration_seconds { quantile="1.0", a="b" } 8.3835e-05
go_gc_duration_seconds { quantile= "1.0", a= "b", } 8.3835e-05
go_gc_duration_seconds { quantile = "1.0", a = "b" } 8.3835e-05
go_gc_duration_seconds { quantile = "2.0" a = "b" } 8.3835e-05
go_gc_duration_seconds_count 99
some:aggregate:rate5m{a_b="c"}	1
`)

func BenchmarkAppenderAppend(b *testing.B) {
	for _, metrics := range []struct {
		name string
		data []byte
	}{
		{name: "metrics=small", data: benchMetrics},
		{name: "metrics=1000", data: generateBenchMetrics(1000)},
	} {
		b.Run(metrics.name+"/stats=off", func(b *testing.B) {
			benchmarkAppenderAppend(b, metrics.data, func() appender.Stats { return appender.Stats{} })
		})

		b.Run(metrics.name+"/stats=on", func(b *testing.B) {
			recorders := appender.NewRecorders(nil)
			benchmarkAppenderAppend(b, metrics.data, func() appender.Stats {
				return appender.Stats{Lap: recorders.Stages.Start(), Shards: recorders.Shards}
			})
		})
	}
}

// generateBenchMetrics returns the scrape text of n series.
func generateBenchMetrics(n int) []byte {
	var buf bytes.Buffer
	for i := range n {
		fmt.Fprintf(&buf, "bench_metric{instance=\"host:9100\",job=\"node\",series=\"%d\"} %d\n", i, i)
	}

	return buf.Bytes()
}

// benchmarkAppenderAppend appends the same hashdex of the metrics in a loop with the stats from newStats.
func benchmarkAppenderAppend(b *testing.B, metrics []byte, newStats func() appender.Stats) {
	// 1. Create Head based on NoopWal
	shards := make([]*shard.Shard, benchNumberOfShards)
	for i := range benchNumberOfShards {
		shards[i] = shard.NewShard(
			shard.NewLSS(),
			shard.NewDataStorage(false, false),
			nil, // unloadedDataStorage
			nil, // queriedSeriesStorage
			wal.NewNoopWal(),
			i,
		)
	}

	h := head.NewHead(
		"bench-head",
		shards,
		shard.NewPerGoroutineShard[wal.NoopWal],
		nil, // releaseHeadFn
		0,   // generation
		nil, // registry
	)

	b.Cleanup(func() {
		_ = h.Close()
	})

	// 2. Create Hashdex from text metrics
	hashdex := cppbridge.NewPrometheusScraperHashdex()
	_, err := hashdex.Parse(metrics, -1)
	if err != nil {
		b.Fatalf("failed to parse metrics: %v", err)
	}

	incomingData := &appender.IncomingData{
		Hashdex: hashdex,
		Data:    nil,
	}

	statelessRelabeler, err := cppbridge.NewStatelessRelabeler([]*cppbridge.RelabelConfig{})
	if err != nil {
		b.Fatalf("failed to create stateless relabeler: %v", err)
	}

	state := cppbridge.NewStateV2WithoutLock()
	state.SetStatelessRelabeler(statelessRelabeler)
	state.EnableTrackStaleness()

	ctx := context.Background()

	// 3. Create Appender and perform initial Append
	ts := time.Now().UnixMilli()
	state.SetDefTimestamp(ts)
	_, err = appender.New(h, commitAndFlush, newStats()).Append(ctx, incomingData, state, false)
	if err != nil {
		b.Fatalf("initial append failed: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	// 4. In the main benchmark loop create new Appender and Append the same Hashdex
	for i := 0; i < b.N; i++ {
		incomingData.Hashdex = hashdex
		state.SetDefTimestamp(ts + int64(i+15000))
		_, err = appender.New(h, commitAndFlush, newStats()).Append(ctx, incomingData, state, false)
		if err != nil {
			b.Fatalf("append failed: %v", err)
		}
	}
}

// services.CFViaRangeByMask.
func commitAndFlush(*benchHead, []bool) error {
	return nil
}
