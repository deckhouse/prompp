package querier

import (
	"runtime"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

// BenchmarkStartQuery measures timer and stripe selection without query execution.
func BenchmarkStartQuery(b *testing.B) {
	for _, enabled := range []bool{false, true} {
		name := "off"
		var metrics *Metrics

		if enabled {
			name = "on"
			metrics = NewMetrics(prometheus.NewRegistry(), QueryableAppenderSource)
		}

		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()

			for b.Loop() {
				stats := metrics.startQuery(operationSelectRange)
				runtime.KeepAlive(stats)
			}
		})
	}
}
