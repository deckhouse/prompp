package promql

import (
	"testing"

	"github.com/prometheus/prometheus/model/labels"
)

// BenchmarkLabelReplaceCacheChurn measures lookup and insertion when the
// working set exceeds capacity and every measured insertion evicts an entry.
func BenchmarkLabelReplaceCacheChurn(b *testing.B) {
	const capacity = 64
	const workingSet = 1024
	cache := newLabelReplaceCache(capacity, -1, nil)
	result := labels.FromStrings("instance", "value", "dst", "value")
	key := labelReplaceCacheKey{dst: "dst", repl: "$1", src: "instance", regexStr: "(.*)", srcVal: "value"}
	for i := range capacity {
		key.baseHash = uint64(i)
		cache.addLabels(key, result)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := range b.N {
		key.baseHash = uint64((i + capacity) % workingSet)
		cache.getLabels(key)
		cache.addLabels(key, result)
	}
}
