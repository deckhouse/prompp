package promql

import (
	"fmt"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
)

// BenchmarkLabelReplaceCacheSweep excludes filling the cache and measures the
// complete sweep, including eviction accounting and capacity adjustment.
func BenchmarkLabelReplaceCacheSweep(b *testing.B) {
	for _, capacity := range []int{1024, 65536} {
		for _, stalePercent := range []int{0, 25, 100} {
			b.Run(fmt.Sprintf("capacity_%d/stale_%d", capacity, stalePercent), func(b *testing.B) {
				cache := newLabelReplaceCache(capacity, -1, nil)
				operation := cache.acquireOperation(labelReplaceOperationKey{
					dst: "dst", repl: "$1", src: "instance", regexStr: "(.*)",
				})
				defer cache.releaseOperation(operation)
				result := labels.FromStrings("instance", "value", "dst", "value")
				key := labelReplaceCacheKey{operation: operation, srcVal: "value"}
				staleCount := capacity * stalePercent / 100
				const nowMs = labelReplaceCacheStaleAfterMs + labelReplaceCacheSweepIntervalMs

				b.ReportAllocs()
				b.ResetTimer()

				for range b.N {
					b.StopTimer()
					cache.resize(capacity, cache.growResizes)
					cache.nowMs.Store(0)

					for i := range capacity {
						if i == staleCount {
							cache.nowMs.Store(nowMs)
						}

						key.baseHash = uint64(i)
						cache.addLabels(key, result)
					}

					cache.nowMs.Store(nowMs)
					b.StartTimer()
					cache.sweepOnce(nowMs)
				}
			})
		}
	}
}
