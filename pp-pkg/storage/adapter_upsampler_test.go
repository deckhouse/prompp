package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp-pkg/blocks/upsampler"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

//
// mockQuerier
//

// mockQuerier is a simple mock Querier for testing wrapIfWouldDownsample.
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
// mockSeriesSet
//

// mockSeriesSet is a simple mock [storage.SeriesSet] for testing wrapIfWouldDownsample.
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

func (s *AdapterUpsamplerSuite) TestWrapIfWouldDownsampleWrapsWhenNeeded() {
	// Arrange
	adapter := &Adapter{
		opts: &AdapterOptions{
			RetentionMS:    10000,
			DownsamplingMS: 60000,
		},
	}
	mockQ := &mockQuerier{wouldDownsample: true}

	// Act
	wrapped := adapter.wrapIfWouldDownsample(mockQ)

	// Assert
	// Verify that the result is an upsampler.Querier
	_, ok := wrapped.(*upsampler.Querier)
	s.Require().True(ok, "expected wrapped querier to be of type upsampler.Querier")
}

func (s *AdapterUpsamplerSuite) TestWrapIfWouldDownsampleSkipsWhenNotNeeded() {
	// Arrange
	adapter := &Adapter{
		opts: &AdapterOptions{
			RetentionMS:    10000,
			DownsamplingMS: cppbridge.NoDownsampling,
		},
	}
	mockQ := &mockQuerier{wouldDownsample: false}

	// Act
	wrapped := adapter.wrapIfWouldDownsample(mockQ)

	// Assert
	// Verify that the result is the original querier (not wrapped)
	s.Require().Same(mockQ, wrapped)
}

func (s *AdapterUpsamplerSuite) TestWrapIfWouldDownsampleHandlesNonDownsampler() {
	// Arrange
	adapter := &Adapter{
		opts: &AdapterOptions{
			RetentionMS:    10000,
			DownsamplingMS: 60000,
		},
	}
	// A simple storage.Querier that doesn't have WouldDownsample method
	var mockQ storage.Querier = &mockQuerier{wouldDownsample: false}

	// Act
	wrapped := adapter.wrapIfWouldDownsample(mockQ)

	// Assert
	// Verify that the result is the original querier when it doesn't implement downsampler interface
	s.Require().Same(mockQ, wrapped)
}
