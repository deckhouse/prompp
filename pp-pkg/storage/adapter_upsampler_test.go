package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

//
// mockQuerier
//

// mockQuerier is a simple mock Querier for testing wouldDownsample.
type mockQuerier struct {
	wouldDownsample bool
}

// Select implements the [storage.Querier] interface for mockQuerier.
func (*mockQuerier) Select(context.Context, bool, *storage.SelectHints, ...*labels.Matcher) storage.SeriesSet {
	return &mockSeriesSet{}
}

// LabelValues implements the [storage.Querier] interface for mockQuerier.
func (*mockQuerier) LabelValues(
	context.Context,
	string,
	*storage.LabelHints,
	...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

// LabelNames implements the [storage.Querier] interface for mockQuerier.
func (*mockQuerier) LabelNames(
	context.Context,
	*storage.LabelHints,
	...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

// Close implements the [storage.Querier] interface for mockQuerier.
func (*mockQuerier) Close() error {
	return nil
}

// WouldDownsample implements the [downsampler] interface for mockQuerier.
func (m *mockQuerier) WouldDownsample() bool {
	return m.wouldDownsample
}

//
// plainQuerier
//

// plainQuerier is a [storage.Querier] that does not implement the [downsampler] interface.
type plainQuerier struct{}

// Select implements the [storage.Querier] interface for plainQuerier.
func (*plainQuerier) Select(context.Context, bool, *storage.SelectHints, ...*labels.Matcher) storage.SeriesSet {
	return &mockSeriesSet{}
}

// LabelValues implements the [storage.Querier] interface for plainQuerier.
func (*plainQuerier) LabelValues(
	context.Context,
	string,
	*storage.LabelHints,
	...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

// LabelNames implements the [storage.Querier] interface for plainQuerier.
func (*plainQuerier) LabelNames(
	context.Context,
	*storage.LabelHints,
	...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return nil, nil, nil
}

// Close implements the [storage.Querier] interface for plainQuerier.
func (*plainQuerier) Close() error {
	return nil
}

//
// mockSeriesSet
//

// mockSeriesSet is a simple mock [storage.SeriesSet] for testing wouldDownsample.
type mockSeriesSet struct{}

// Select implements the [storage.SeriesSet] interface for mockSeriesSet.
func (*mockSeriesSet) Next() bool {
	return false
}

// At implements the [storage.SeriesSet] interface for mockSeriesSet.
func (*mockSeriesSet) At() storage.Series {
	return nil
}

// Err implements the [storage.SeriesSet] interface for mockSeriesSet.
func (*mockSeriesSet) Err() error {
	return nil
}

// Warnings implements the [storage.SeriesSet] interface for mockSeriesSet.
func (*mockSeriesSet) Warnings() annotations.Annotations {
	return nil
}

//
// AdapterUpsamplerSuite
//

type AdapterUpsamplerSuite struct {
	suite.Suite
}

func TestAdapterSuite(t *testing.T) {
	suite.Run(t, new(AdapterUpsamplerSuite))
}

func (s *AdapterUpsamplerSuite) TestWouldDownsampleReportsTrue() {
	// Arrange
	adapter := &Adapter{
		opts: &AdapterOptions{
			RetentionMS:    10000,
			DownsamplingMS: 60000,
		},
	}
	mockQ := &mockQuerier{wouldDownsample: true}

	// Act
	wouldDownsample := adapter.wouldDownsample(mockQ)

	// Assert
	s.Require().True(wouldDownsample)
}

func (s *AdapterUpsamplerSuite) TestWouldDownsampleReportsFalse() {
	// Arrange
	adapter := &Adapter{
		opts: &AdapterOptions{
			RetentionMS:    10000,
			DownsamplingMS: cppbridge.NoDownsampling,
		},
	}
	mockQ := &mockQuerier{wouldDownsample: false}

	// Act
	wouldDownsample := adapter.wouldDownsample(mockQ)

	// Assert
	s.Require().False(wouldDownsample)
}

func (s *AdapterUpsamplerSuite) TestWouldDownsampleHandlesNonDownsampler() {
	// Arrange
	adapter := &Adapter{
		opts: &AdapterOptions{
			RetentionMS:    10000,
			DownsamplingMS: 60000,
		},
	}
	// A simple storage.Querier without the WouldDownsample method.
	var plainQ storage.Querier = &plainQuerier{}

	// Act
	wouldDownsample := adapter.wouldDownsample(plainQ)

	// Assert
	s.Require().False(wouldDownsample)
}
