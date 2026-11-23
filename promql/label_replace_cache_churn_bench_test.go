package promql

import (
	"strconv"
	"testing"
	"unsafe"

	"github.com/prometheus/prometheus/model/labels"
)

// BenchmarkLabelReplaceCacheChurn measures lookup and insertion when the
// working set exceeds capacity and every measured insertion evicts an entry.
func BenchmarkLabelReplaceCacheChurn(b *testing.B) {
	const capacity = 64
	const workingSet = 1024
	cache := newLabelReplaceCache(capacity, -1, nil)
	result := labels.FromStrings("instance", "value", "dst", "value")
	operation := cache.acquireOperation(labelReplaceOperationKey{
		dst: "dst", repl: "$1", src: "instance", regexStr: "(.*)",
	})
	defer cache.releaseOperation(operation)
	key := labelReplaceCacheKey{operation: operation}
	bases := make([]string, workingSet)
	for i := range bases {
		bases[i] = string(labels.FromStrings("instance", strconv.Itoa(i)).Bytes(nil))
	}
	for i := range capacity {
		key.base = bases[i]
		cache.addLabels(key, result, true)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := range b.N {
		key.base = bases[(i+capacity)%workingSet]
		cache.getLabels(key)
		cache.addLabels(key, result, true)
	}
}

// BenchmarkLabelReplaceCacheWarmLookup includes serialization into a reused
// buffer and lookup through a temporary string view.
func BenchmarkLabelReplaceCacheWarmLookup(b *testing.B) {
	cache := newLabelReplaceCache(64, -1, nil)
	operation := cache.acquireOperation(labelReplaceOperationKey{
		dst: "dst", repl: "$1", src: "instance", regexStr: "(.*)",
	})
	defer cache.releaseOperation(operation)
	source := labels.FromStrings("instance", "value", "job", "example")
	buffer := source.Bytes(make([]byte, 0, 128))
	key := labelReplaceCacheKey{operation: operation, base: unsafe.String(unsafe.SliceData(buffer), len(buffer))}
	cache.addLabels(key, source, true)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		buffer = source.Bytes(buffer)
		key.base = unsafe.String(unsafe.SliceData(buffer), len(buffer))
		if _, hit := cache.getLabels(key); !hit {
			b.Fatal("expected a warm cache hit")
		}
	}
}
