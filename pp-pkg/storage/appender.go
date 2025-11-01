package storage

import (
	"context"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	pp_pkg_model "github.com/prometheus/prometheus/pp-pkg/model"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/storage"
)

// timeSeriesBatch implementation buffer of [TimeSeriesBatch].
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

//
// AppenderTimeSeries
//

// AppenderTimeSeries append TimeSeries data to [Head].
type AppenderTimeSeries interface {
	// AppendTimeSeries append TimeSeries data to [Head].
	AppendTimeSeries(
		ctx context.Context,
		data pp_pkg_model.TimeSeriesBatch,
		state *cppbridge.StateV2,
		commitToWal bool,
	) (cppbridge.RelabelerStats, error)
}

//
// TimeSeriesAppender
//

// TimeSeriesAppender appender for rules, aggregates the [model.TimeSeries] batch and append to head,
// implementation [storage.Appender].
type TimeSeriesAppender struct {
	ctx      context.Context
	appender AppenderTimeSeries
	state    *cppbridge.StateV2
	batch    *timeSeriesBatch
	lsb      *model.LabelSetBuilder
}

// newTimeSeriesAppender init new [TimeSeriesAppender].
func newTimeSeriesAppender(
	ctx context.Context,
	appender AppenderTimeSeries,
	state *cppbridge.StateV2,
) *TimeSeriesAppender {
	return &TimeSeriesAppender{
		ctx:      ctx,
		appender: appender,
		state:    state,
		batch:    &timeSeriesBatch{timeSeries: make([]model.TimeSeries, 0, 10)},
		lsb:      model.NewLabelSetBuilderSize(10),
	}
}

// Append adds a sample pair for the given series, implementation [storage.Appender].
func (a *TimeSeriesAppender) Append(
	_ storage.SeriesRef,
	l labels.Labels,
	t int64,
	v float64,
) (storage.SeriesRef, error) {
	a.lsb.Reset()
	l.Range(func(label labels.Label) {
		a.lsb.Add(label.Name, label.Value)
	})

	a.batch.timeSeries = append(a.batch.timeSeries, model.TimeSeries{
		LabelSet:  a.lsb.Build(),
		Timestamp: uint64(t), // #nosec G115 // no overflow
		Value:     v,
	})
	return 0, nil
}

// AppendCTZeroSample do nothing, implementation [storage.Appender].
func (*TimeSeriesAppender) AppendCTZeroSample(
	_ storage.SeriesRef,
	_ labels.Labels,
	_, _ int64,
) (storage.SeriesRef, error) {
	return 0, nil
}

// AppendExemplar do nothing, implementation [storage.Appender].
func (*TimeSeriesAppender) AppendExemplar(
	_ storage.SeriesRef,
	_ labels.Labels,
	_ exemplar.Exemplar,
) (storage.SeriesRef, error) {
	return 0, nil
}

// AppendHistogram do nothing, implementation [storage.Appender].
func (*TimeSeriesAppender) AppendHistogram(
	_ storage.SeriesRef,
	_ labels.Labels,
	_ int64,
	_ *histogram.Histogram,
	_ *histogram.FloatHistogram,
) (storage.SeriesRef, error) {
	return 0, nil
}

// Commit adds aggregated series to the head, implementation [storage.Appender].
func (a *TimeSeriesAppender) Commit() error {
	if len(a.batch.timeSeries) == 0 {
		return nil
	}

	_, err := a.appender.AppendTimeSeries(a.ctx, a.batch, a.state, false)
	return err
}

// Rollback do nothing, implementation [storage.Appender].
func (*TimeSeriesAppender) Rollback() error {
	return nil
}

// UpdateMetadata do nothing, implementation [storage.Appender].
func (*TimeSeriesAppender) UpdateMetadata(
	_ storage.SeriesRef,
	_ labels.Labels,
	_ metadata.Metadata,
) (storage.SeriesRef, error) {
	return 0, nil
}
