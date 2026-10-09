package stagestats

import (
	"fmt"
	"math/rand/v2"
	"testing"
)

var benchStages = []string{"first", "second", "third", "fourth", "fifth", "sixth", "seventh"}

// BenchmarkLapMark measures a lap of all stages per iteration from parallel goroutines,
// like concurrent Appends, for different numbers of stripes.
func BenchmarkLapMark(b *testing.B) {
	for _, stripes := range []int{1, 8, 16, 32, 64} {
		b.Run(fmt.Sprintf("stripes=%d", stripes), func(b *testing.B) {
			r := newRecorder(Opts{Name: "bench"}, benchStages, stripes)
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					lap := r.Start()
					for stage := range Stage(len(benchStages)) {
						lap.Mark(stage)
					}
				}
			})
		})
	}
}

// BenchmarkStripeObserve measures only the counters update from parallel goroutines on random stripes,
// for different numbers of stripes.
func BenchmarkStripeObserve(b *testing.B) {
	for _, stripes := range []int{1, 8, 16, 32, 64} {
		b.Run(fmt.Sprintf("stripes=%d", stripes), func(b *testing.B) {
			r := newRecorder(Opts{Name: "bench"}, benchStages, stripes)
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					r.Stripe(rand.Uint32()).Observe(0, 100) // #nosec G404 // benchmark
				}
			})
		})
	}
}

// BenchmarkObserveMax measures the maximum over shards and its observation.
func BenchmarkObserveMax(b *testing.B) {
	for _, shards := range []uint16{4, 16, 64} {
		b.Run(fmt.Sprintf("shards=%d", shards), func(b *testing.B) {
			r := newRecorder(Opts{Name: "bench"}, benchStages, defaultStripes())
			slots := NewShardSlots(shards)
			for shardID := range shards {
				slots.Set(shardID, 0, int64(shardID)+1)
			}
			stripe := r.Stripe(0)
			b.ReportAllocs()
			for b.Loop() {
				stripe.ObserveMax(0, slots, 0)
			}
		})
	}
}

// BenchmarkNow measures one clock reading.
func BenchmarkNow(b *testing.B) {
	var sink int64
	for b.Loop() {
		sink += Now()
	}
	_ = sink
}

// BenchmarkNowParallel measures the clock readings of a lap without the counters update,
// the baseline of [BenchmarkLapMark].
func BenchmarkNowParallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		var sink int64
		for pb.Next() {
			for range len(benchStages) + 1 {
				sink += Now()
			}
		}
		_ = sink
	})
}

// work imitates the stage work between marks.
func work(n int) uint64 {
	x := uint64(n)
	for i := range n {
		x = x*6364136223846793005 + uint64(i) // #nosec G115 // benchmark
	}

	return x
}

// BenchmarkLapMarkWithWork measures a lap with ~0.5 µs of work per stage from parallel goroutines,
// closer to a real pipeline than [BenchmarkLapMark]; stripes=0 is a nil [Recorder] baseline.
func BenchmarkLapMarkWithWork(b *testing.B) {
	for _, stripes := range []int{0, 1, 8, 64} {
		b.Run(fmt.Sprintf("stripes=%d", stripes), func(b *testing.B) {
			var r *Recorder
			if stripes > 0 {
				r = newRecorder(Opts{Name: "bench"}, benchStages, stripes)
			}
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				var sink uint64
				for pb.Next() {
					lap := r.Start()
					for stage := range Stage(len(benchStages)) {
						sink += work(2000)
						lap.Mark(stage)
					}
				}
				_ = sink
			})
		})
	}
}
