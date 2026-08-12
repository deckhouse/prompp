package upsampler_test

import (
	"context"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
)

//
// mockQuerier
//

// mockQuerier implements storage.Querier for testing.
type mockQuerier struct {
	selectFunc      func(ctx context.Context, sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet
	labelValuesFunc func(ctx context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error)
	labelNamesFunc  func(ctx context.Context, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error)
	closeFunc       func() error
}

// Select returns a series set for the querier, using the provided function if set.
func (mq *mockQuerier) Select(ctx context.Context, sortSeries bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	if mq.selectFunc != nil {
		return mq.selectFunc(ctx, sortSeries, hints, matchers...)
	}
	return &mockSeriesSet{}
}

// LabelValues returns the label values for the querier, using the provided function if set.
func (mq *mockQuerier) LabelValues(ctx context.Context, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	if mq.labelValuesFunc != nil {
		return mq.labelValuesFunc(ctx, name, hints, matchers...)
	}
	return nil, nil, nil
}

// LabelNames returns the label names for the querier, using the provided function if set.
func (mq *mockQuerier) LabelNames(ctx context.Context, hints *storage.LabelHints, matchers ...*labels.Matcher) ([]string, annotations.Annotations, error) {
	if mq.labelNamesFunc != nil {
		return mq.labelNamesFunc(ctx, hints, matchers...)
	}
	return nil, nil, nil
}

// Close closes the querier and releases any resources.
func (mq *mockQuerier) Close() error {
	if mq.closeFunc != nil {
		return mq.closeFunc()
	}
	return nil
}

//
// mockSeriesSet
//

// mockSeriesSet implements storage.SeriesSet for testing.
type mockSeriesSet struct {
	nextFunc     func() bool
	atFunc       func() storage.Series
	errFunc      func() error
	warningsFunc func() annotations.Annotations
}

// Next advances the iterator to the next series in the set.
func (mss *mockSeriesSet) Next() bool {
	if mss.nextFunc != nil {
		return mss.nextFunc()
	}
	return false
}

// At returns the current series in the set.
func (mss *mockSeriesSet) At() storage.Series {
	if mss.atFunc != nil {
		return mss.atFunc()
	}
	return nil
}

// Err returns any accumulated error.
func (mss *mockSeriesSet) Err() error {
	if mss.errFunc != nil {
		return mss.errFunc()
	}
	return nil
}

// Warnings returns any accumulated warnings.
func (mss *mockSeriesSet) Warnings() annotations.Annotations {
	if mss.warningsFunc != nil {
		return mss.warningsFunc()
	}
	return nil
}

//
// mockSeries
//

// mockSeries implements storage.Series for testing.
type mockSeries struct {
	labels       labels.Labels
	iteratorFunc func(chunkenc.Iterator) chunkenc.Iterator
}

// Labels returns the labels of the series.
func (ms *mockSeries) Labels() labels.Labels {
	return ms.labels
}

// Iterator returns an iterator over samples.
func (ms *mockSeries) Iterator(it chunkenc.Iterator) chunkenc.Iterator {
	if ms.iteratorFunc != nil {
		return ms.iteratorFunc(it)
	}
	return it
}

//
// mockIterator
//

// mockIterator is a simple test double that yields a fixed sequence of float samples.
type mockIterator struct {
	samples []struct {
		t int64
		v float64
	}
	idx int
}

// newMockIterator initialize a new [mockIterator] with the given samples.
func newMockIterator(samples []struct {
	t int64
	v float64
},
) *mockIterator {
	return &mockIterator{
		samples: samples,
		idx:     -1,
	}
}

// Next advances the iterator by one and returns the type of the value.
func (m *mockIterator) Next() chunkenc.ValueType {
	m.idx++
	if m.idx < len(m.samples) {
		return chunkenc.ValFloat
	}

	return chunkenc.ValNone
}

// At returns the current timestamp/value pair for float samples.
func (m *mockIterator) At() (int64, float64) {
	if m.idx >= 0 && m.idx < len(m.samples) {
		return m.samples[m.idx].t, m.samples[m.idx].v
	}

	return 0, 0
}

// AtT returns the current timestamp.
func (m *mockIterator) AtT() int64 {
	if m.idx >= 0 && m.idx < len(m.samples) {
		return m.samples[m.idx].t
	}

	return 0
}

// AtFloatHistogram returns nil — we don't synthesize histograms in v1.
func (*mockIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	return 0, nil
}

// AtHistogram returns nil — we don't synthesize histograms in v1.
func (*mockIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	return 0, nil
}

// Err returns any accumulated error.
func (*mockIterator) Err() error {
	return nil
}

// Seek advances to the first sample with timestamp >= target.
func (m *mockIterator) Seek(target int64) chunkenc.ValueType {
	for m.idx = 0; m.idx < len(m.samples); m.idx++ {
		if m.samples[m.idx].t >= target {
			return chunkenc.ValFloat
		}
	}
	return chunkenc.ValNone
}

//
// mockHistogramIterator
//

// mockHistogramIterator yields histogram samples for testing non-float handling.
type mockHistogramIterator struct {
	samples []struct {
		t int64
		h *histogram.FloatHistogram
	}
	idx int
}

// newMockHistogramIterator initialize a new [mockHistogramIterator] with the given [histogram.FloatHistogram] samples.
func newMockHistogramIterator(samples []struct {
	t int64
	h *histogram.FloatHistogram
},
) *mockHistogramIterator {
	return &mockHistogramIterator{
		samples: samples,
		idx:     -1,
	}
}

// Next advances the iterator by one and returns the type of the value.
func (m *mockHistogramIterator) Next() chunkenc.ValueType {
	m.idx++
	if m.idx < len(m.samples) {
		return chunkenc.ValFloatHistogram
	}
	return chunkenc.ValNone
}

// At returns the current timestamp/value pair for float samples.
func (m *mockHistogramIterator) At() (int64, float64) {
	if m.idx >= 0 && m.idx < len(m.samples) {
		return m.samples[m.idx].t, 0
	}
	return 0, 0
}

// AtT returns the current timestamp.
func (m *mockHistogramIterator) AtT() int64 {
	if m.idx >= 0 && m.idx < len(m.samples) {
		return m.samples[m.idx].t
	}
	return 0
}

// AtFloatHistogram returns the current float histogram.
func (m *mockHistogramIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	if m.idx >= 0 && m.idx < len(m.samples) {
		return m.samples[m.idx].t, m.samples[m.idx].h
	}
	return 0, nil
}

// AtHistogram returns the current histogram.
func (*mockHistogramIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	return 0, nil
}

// Err returns any accumulated error.
func (*mockHistogramIterator) Err() error {
	return nil
}

// Seek advances to the first sample with timestamp >= target.
func (m *mockHistogramIterator) Seek(target int64) chunkenc.ValueType {
	for m.idx = 0; m.idx < len(m.samples); m.idx++ {
		if m.samples[m.idx].t >= target {
			return chunkenc.ValFloatHistogram
		}
	}
	return chunkenc.ValNone
}

//
// labelValuesQuerier
//

// mockLabelValuesQuerier returns specific values for LabelValues calls.
type mockLabelValuesQuerier struct {
	values []string
}

// Select returns a series set for the querier, using the provided function if set.
func (*mockLabelValuesQuerier) Select(
	context.Context,
	bool,
	*storage.SelectHints,
	...*labels.Matcher,
) storage.SeriesSet {
	return &mockSeriesSet{}
}

// LabelValues returns the label values for the querier, using the provided values.
func (q *mockLabelValuesQuerier) LabelValues(
	context.Context,
	string,
	*storage.LabelHints,
	...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return q.values, nil, nil
}

// LabelNames returns nil for the querier, as it is not used in this mock.
func (*mockLabelValuesQuerier) LabelNames(
	context.Context,
	*storage.LabelHints,
	...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

// Close closes the querier and releases any resources.
func (*mockLabelValuesQuerier) Close() error {
	return nil
}

//
// mockLabelNamesQuerier
//

// mockLabelNamesQuerier returns specific names for LabelNames calls.
type mockLabelNamesQuerier struct {
	names []string
}

// Select returns a series set for the querier, using the provided function if set.
func (*mockLabelNamesQuerier) Select(
	context.Context,
	bool,
	*storage.SelectHints,
	...*labels.Matcher,
) storage.SeriesSet {
	return &mockSeriesSet{}
}

// LabelValues returns nil for the querier, as it is not used in this mock.
func (*mockLabelNamesQuerier) LabelValues(
	context.Context,
	string,
	*storage.LabelHints,
	...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

// LabelNames returns the label names for the querier, using the provided names.
func (q *mockLabelNamesQuerier) LabelNames(
	context.Context,
	*storage.LabelHints,
	...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return q.names, nil, nil
}

// Close closes the querier and releases any resources.
func (*mockLabelNamesQuerier) Close() error {
	return nil
}

//
// closeableQuerier
//

// closeableQuerier tracks whether Close() was called.
type closeableQuerier struct {
	closed bool
}

// Select returns a series set for the querier, using the provided function if set.
func (*closeableQuerier) Select(
	context.Context,
	bool,
	*storage.SelectHints,
	...*labels.Matcher,
) storage.SeriesSet {
	return &mockSeriesSet{}
}

// LabelValues returns nil for the querier, as it is not used in this mock.
func (*closeableQuerier) LabelValues(
	context.Context,
	string,
	*storage.LabelHints,
	...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

// LabelNames returns nil for the querier, as it is not used in this mock.
func (*closeableQuerier) LabelNames(
	context.Context,
	*storage.LabelHints,
	...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

// Close closes the querier and releases any resources.
func (q *closeableQuerier) Close() error {
	q.closed = true
	return nil
}
