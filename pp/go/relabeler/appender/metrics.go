package appender

import (
	"context"
	"time"

	"github.com/prometheus/prometheus/pp/go/util"
)

const (
	// DefaultMetricWriteInterval default metric scrape interval.
	DefaultMetricWriteInterval = time.Second * 15
)

// MetricWriterTarget - something that can write metrics.
type MetricWriterTarget interface {
	WriteMetrics(ctx context.Context)
}

// MetricsWriteTrigger - metrics write trigger.
type MetricsWriteTrigger struct {
	interval time.Duration
	targets  []MetricWriterTarget
	closer   *util.Closer
}

// NewMetricsWriteTrigger - MetricsWriteTrigger constructor.
func NewMetricsWriteTrigger(ctx context.Context, interval time.Duration, targets ...MetricWriterTarget) *MetricsWriteTrigger {
	t := &MetricsWriteTrigger{
		interval: interval,
		targets:  targets,
		closer:   util.NewCloser(),
	}

	go t.loop(ctx)

	return t
}

func (t *MetricsWriteTrigger) loop(ctx context.Context) {
	defer t.closer.Done()
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			for _, target := range t.targets {
				target.WriteMetrics(ctx)
			}
		case <-t.closer.Signal():
			return
		}
	}
}

// Close - io.Closer interface implementation.
func (t *MetricsWriteTrigger) Close() error {
	return t.closer.Close()
}
