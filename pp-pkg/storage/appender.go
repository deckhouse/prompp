package storage

import (
	"context"

	"github.com/prometheus/prometheus/pp/go/model"
)

// timeSeriesBatch implementation buffer of [ppstorage.TimeSeriesData].
type timeSeriesBatch struct {
	timeSeries []model.TimeSeries
}

// TimeSeries returns slice [model.TimeSeries].
func (d *timeSeriesBatch) TimeSeries() []model.TimeSeries {
	return d.timeSeries
}

// Destroy buffered data.
func (d *timeSeriesBatch) Destroy() {
	d.timeSeries = nil
}

// TimeSeriesAppender
type TimeSeriesAppender struct {
	ctx context.Context
	// receiver    *Receiver
	relabelerID string
	data        *timeSeriesBatch
}
