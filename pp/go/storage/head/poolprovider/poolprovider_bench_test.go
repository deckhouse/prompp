package poolprovider_test

import (
	"fmt"
	"testing"

	"github.com/prometheus/prometheus/pp/go/storage/head/poolprovider"
)

// BenchmarkHeadPoolShardSlots measures acquiring and resetting duration rows on a warmed pool.
func BenchmarkHeadPoolShardSlots(b *testing.B) {
	for _, shards := range []uint16{2, 16, 64} {
		b.Run(fmt.Sprintf("shards=%d", shards), func(b *testing.B) {
			pool := poolprovider.NewHeadPool[*testShard](shards)
			slots := pool.GetShardSlots()
			pool.PutShardSlots(slots)
			b.ReportAllocs()

			for b.Loop() {
				slots := pool.GetShardSlots()
				pool.PutShardSlots(slots)
			}
		})
	}
}
