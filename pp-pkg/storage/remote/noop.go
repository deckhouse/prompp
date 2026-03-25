package remote

import (
	"context"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

//
// Storage
//

// noOpStorage implements storage.Storage.
type noOpStorage struct{}

var _ storage.Storage = (*noOpStorage)(nil)

// Appender implements storage.Storage.
func (*noOpStorage) Appender(_ context.Context) storage.Appender {
	return noOpAppender{}
}

// Querier implements storage.Storage.
func (*noOpStorage) Querier(_, _ int64) (storage.Querier, error) {
	return noOpQuerier{}, nil
}

// ChunkQuerier implements storage.Storage.
func (*noOpStorage) ChunkQuerier(_, _ int64) (storage.ChunkQuerier, error) {
	return noOpChunkQuerier{}, nil
}

// StartTime implements storage.Storage.
func (*noOpStorage) StartTime() (int64, error) {
	return int64(model.Latest), nil
}

// Close implements storage.Storage.
func (*noOpStorage) Close() error {
	return nil
}

//
// noOpAppender
//

// noOpAppender implements [storage.Appender], do nothing.
type noOpAppender struct{}

// Append implements [storage.Appender], do nothing.
func (noOpAppender) Append(_ storage.SeriesRef, _ labels.Labels, _ int64, _ float64) (storage.SeriesRef, error) {
	return 0, nil
}

// AppendCTZeroSample implements [storage.Appender], do nothing.
func (noOpAppender) AppendCTZeroSample(_ storage.SeriesRef, _ labels.Labels, _, _ int64) (storage.SeriesRef, error) {
	return 0, nil
}

// AppendExemplar implements [storage.Appender], do nothing.
func (noOpAppender) AppendExemplar(
	_ storage.SeriesRef,
	_ labels.Labels,
	_ exemplar.Exemplar,
) (storage.SeriesRef, error) {
	return 0, nil
}

// AppendHistogram implements [storage.Appender], do nothing.
func (noOpAppender) AppendHistogram(
	_ storage.SeriesRef,
	_ labels.Labels,
	_ int64,
	_ *histogram.Histogram,
	_ *histogram.FloatHistogram,
) (storage.SeriesRef, error) {
	return 0, nil
}

// Commit implements [storage.Appender], do nothing.
func (noOpAppender) Commit() error {
	return nil
}

// Rollback implements [storage.Appender], do nothing.
func (noOpAppender) Rollback() error {
	return nil
}

// UpdateMetadata implements [storage.Appender], do nothing.
func (noOpAppender) UpdateMetadata(
	_ storage.SeriesRef,
	_ labels.Labels,
	_ metadata.Metadata,
) (storage.SeriesRef, error) {
	return 0, nil
}

//
// Querier
//

// noOpQuerier implements storage.Querier.
type noOpQuerier struct{ noOpLabelQuerier }

var _ storage.Querier = (*noOpQuerier)(nil)

// Select implements storage.Querier.
func (noOpQuerier) Select(
	_ context.Context,
	_ bool,
	_ *storage.SelectHints,
	_ ...*labels.Matcher,
) storage.SeriesSet {
	return noOpSeriesSet{}
}

//
// ChunkQuerier
//

// noOpChunkQuerier implements storage.ChunkQuerier.
type noOpChunkQuerier struct{ noOpLabelQuerier }

var _ storage.ChunkQuerier = (*noOpChunkQuerier)(nil)

// Select implements storage.ChunkQuerier.
func (noOpChunkQuerier) Select(
	_ context.Context,
	_ bool,
	_ *storage.SelectHints,
	_ ...*labels.Matcher,
) storage.ChunkSeriesSet {
	return noOpChunkSeriesSet{}
}

//
// LabelQuerier
//

// noOpLabelQuerier implements storage.LabelQuerier.
type noOpLabelQuerier struct{}

var _ storage.LabelQuerier = (*noOpLabelQuerier)(nil)

// LabelValues implements storage.LabelQuerier.
func (noOpLabelQuerier) LabelValues(
	_ context.Context,
	_ string,
	_ *storage.LabelHints,
	_ ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

// LabelValues implements storage.LabelQuerier.
func (noOpLabelQuerier) LabelNames(
	_ context.Context,
	_ *storage.LabelHints,
	_ ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

// Close implements storage.LabelQuerier.
func (noOpLabelQuerier) Close() error {
	return nil
}

//
// SeriesSet
//

// noOpSeriesSet implements storage.SeriesSet.
type noOpSeriesSet struct{}

var _ storage.SeriesSet = (*noOpSeriesSet)(nil)

// Next implements storage.SeriesSet.
func (noOpSeriesSet) Next() bool {
	return false
}

// At implements storage.SeriesSet.
func (noOpSeriesSet) At() storage.Series {
	return nil
}

// Err implements storage.SeriesSet.
func (noOpSeriesSet) Err() error {
	return nil
}

// Warnings implements storage.SeriesSet.
func (noOpSeriesSet) Warnings() annotations.Annotations {
	return nil
}

//
// ChunkSeriesSet
//

// noOpChunkSeriesSet implements storage.ChunkSeriesSet.
type noOpChunkSeriesSet struct{}

var _ storage.ChunkSeriesSet = (*noOpChunkSeriesSet)(nil)

// Next implements storage.ChunkSeriesSet.
func (noOpChunkSeriesSet) Next() bool {
	return false
}

// At implements storage.ChunkSeriesSet.
func (noOpChunkSeriesSet) At() storage.ChunkSeries {
	return nil
}

// Err implements storage.ChunkSeriesSet.
func (noOpChunkSeriesSet) Err() error {
	return nil
}

// Warnings implements storage.ChunkSeriesSet.
func (noOpChunkSeriesSet) Warnings() annotations.Annotations {
	return nil
}
