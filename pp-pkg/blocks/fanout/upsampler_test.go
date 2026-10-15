package fanout_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp-pkg/blocks/fanout"
	"github.com/prometheus/prometheus/pp-pkg/blocks/upsampler"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/teststorage"
)

const (
	upsamplerMetricName = "fanout_upsampler_metric"
	minuteMS            = int64(60_000)
)

// sample is one float sample of the series under test.
type sample struct {
	tsMS int64
	v    float64
}

type FanoutUpsamplerSuite struct {
	suite.Suite
}

func TestFanoutUpsamplerSuite(t *testing.T) {
	suite.Run(t, new(FanoutUpsamplerSuite))
}

// TestQuerierUsesMaxResolution verifies that the interpolating wrapper is created with the
// resolution of the sparsest source, whether it comes from the primary or from a secondary.
func (s *FanoutUpsamplerSuite) TestQuerierUsesMaxResolution() {
	// Arrange
	primary := s.newSpyStorage(minuteMS)
	sparseSecondary := s.newSpyStorage(5 * minuteMS)
	rawSecondary := s.newSpyStorage(0)

	q, err := fanout.New(nil, primary, sparseSecondary, rawSecondary).Querier(0, 10*minuteMS)
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = q.Close() })

	s.Require().IsType(&upsampler.Querier{}, q)

	// Act
	s.exhaust(q, &storage.SelectHints{Start: 10 * minuteMS, End: 20 * minuteMS, Range: 2 * minuteMS, Func: "rate"})

	// Assert
	// shouldWrap moves the left border back by the resolution the wrapper was built with, so the
	// hints seen by the sources tell which resolution won.
	for _, spy := range s.spyQueriers(primary, sparseSecondary, rawSecondary) {
		s.Require().NotNil(spy.hints)
		s.Require().Equal(10*minuteMS-5*minuteMS, spy.hints.Start)
		s.Require().Equal(2*minuteMS, spy.hints.Range)
	}
}

// TestQuerierWithoutResolutionIsNotWrapped verifies that a query where no source declares a
// resolution is left completely alone: no wrapper, no shifted hints.
func (s *FanoutUpsamplerSuite) TestQuerierWithoutResolutionIsNotWrapped() {
	// Arrange
	primary := s.newSpyStorage(0)
	secondary := s.newSpyStorage(0)

	q, err := fanout.New(nil, primary, secondary).Querier(0, 10*minuteMS)
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = q.Close() })

	_, wrapped := q.(*upsampler.Querier)
	s.Require().False(wrapped)

	// Act
	s.exhaust(q, &storage.SelectHints{Start: 10 * minuteMS, End: 20 * minuteMS, Range: 2 * minuteMS, Func: "rate"})

	// Assert
	for _, spy := range s.spyQueriers(primary, secondary) {
		s.Require().NotNil(spy.hints)
		s.Require().Equal(10*minuteMS, spy.hints.Start)
	}
}

// TestQuerierSkipsNoopSecondary verifies that an empty secondary is dropped instead of joining
// the merge: with a single real source left, the merge returns that source itself.
func (s *FanoutUpsamplerSuite) TestQuerierSkipsNoopSecondary() {
	// Arrange
	primary := s.newSpyStorage(0)
	noopSecondary := &spyStorage{noop: true}

	// Act
	q, err := fanout.New(nil, primary, noopSecondary).Querier(0, 10*minuteMS)
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = q.Close() })

	// Assert
	s.Require().IsType(&spyQuerier{}, q)
}

// TestQuerierClosesOpenedOnSecondaryError verifies that a failing secondary does not leak the
// queriers opened before it.
func (s *FanoutUpsamplerSuite) TestQuerierClosesOpenedOnSecondaryError() {
	// Arrange
	errOpenQuerier := errors.New("open querier")
	primary := s.newSpyStorage(minuteMS)
	openedSecondary := s.newSpyStorage(minuteMS)
	failingSecondary := &spyStorage{err: errOpenQuerier}

	// Act
	q, err := fanout.New(nil, primary, openedSecondary, failingSecondary).Querier(0, 10*minuteMS)

	// Assert
	s.Require().ErrorIs(err, errOpenQuerier)
	s.Require().Nil(q)

	for _, spy := range s.spyQueriers(primary, openedSecondary) {
		s.Require().True(spy.closed, "querier opened before the failure must be closed")
	}
}

// TestInterpolationAcrossStorageBoundary is the regression for the reason the wrapper lives in
// fanout: the anchor pair may be split between two storages — the last sample of a persisted
// block and the first sample of the head — and interpolation must still happen there.
func (s *FanoutUpsamplerSuite) TestInterpolationAcrossStorageBoundary() {
	// Arrange
	// Blocks hold the older half of the series, the head the newer one, and the samples around
	// the boundary are 2 minutes apart — too sparse for a 2 minute query window.
	blocks := s.newSampleStorage([]sample{{tsMS: 0, v: 0}, {tsMS: minuteMS, v: 1}, {tsMS: 2 * minuteMS, v: 2}})
	head := s.newSampleStorage([]sample{{tsMS: 4 * minuteMS, v: 4}, {tsMS: 5 * minuteMS, v: 5}})

	q, err := fanout.New(nil, head, resolutionStorage{Storage: blocks, resolutionMS: minuteMS}).
		Querier(0, 5*minuteMS)
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = q.Close() })

	s.Require().IsType(&upsampler.Querier{}, q)

	// Act
	hints := &storage.SelectHints{Start: 0, End: 5 * minuteMS, Range: 2 * minuteMS, Func: "rate"}
	samples := s.collect(q.Select(s.T().Context(), true, hints, s.metricMatcher()))

	// Assert
	// A synthetic sample lands in the middle of the block-to-head gap, interpolated between the
	// real samples on both sides of the boundary.
	s.Require().Equal(
		map[int64]float64{
			0:            0,
			minuteMS:     1,
			2 * minuteMS: 2,
			3 * minuteMS: 3,
			4 * minuteMS: 4,
			5 * minuteMS: 5,
		},
		samples,
	)
}

// newSpyStorage returns an empty storage handing out queriers that record how they were used.
// A resolutionMS above zero makes those queriers declare it to fanout.
func (s *FanoutUpsamplerSuite) newSpyStorage(resolutionMS int64) *spyStorage {
	return &spyStorage{Storage: s.newStorage(), resolutionMS: resolutionMS}
}

// newSampleStorage returns a storage holding one series with the given samples, which must be
// ordered by timestamp — out-of-order samples are silently dropped by the appender.
func (s *FanoutUpsamplerSuite) newSampleStorage(samples []sample) storage.Storage {
	st := s.newStorage()
	app := st.Appender(s.T().Context())
	for _, smpl := range samples {
		_, err := app.Append(0, labels.FromStrings(labels.MetricName, upsamplerMetricName), smpl.tsMS, smpl.v)
		s.Require().NoError(err)
	}
	s.Require().NoError(app.Commit())

	return st
}

// newStorage returns an empty storage closed at the end of the test.
func (s *FanoutUpsamplerSuite) newStorage() storage.Storage {
	st := teststorage.New(s.T())
	s.T().Cleanup(func() { _ = st.Close() })

	return st
}

// metricMatcher matches the series used by the suite.
func (*FanoutUpsamplerSuite) metricMatcher() *labels.Matcher {
	return labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, upsamplerMetricName)
}

// exhaust selects with the given hints and drains the result, so that every source is reached
// even through a lazily initialized merge.
func (s *FanoutUpsamplerSuite) exhaust(q storage.Querier, hints *storage.SelectHints) {
	ss := q.Select(s.T().Context(), true, hints, s.metricMatcher())
	for ss.Next() {
		ss.At()
	}
	s.Require().NoError(ss.Err())
}

// collect drains a SeriesSet of a single series into a timestamp/value map.
func (s *FanoutUpsamplerSuite) collect(ss storage.SeriesSet) map[int64]float64 {
	samples := make(map[int64]float64)
	for ss.Next() {
		it := ss.At().Iterator(nil)
		for it.Next() == chunkenc.ValFloat {
			ts, v := it.At()
			samples[ts] = v
		}
		s.Require().NoError(it.Err())
	}
	s.Require().NoError(ss.Err())

	return samples
}

// spyQueriers returns every querier handed out by the given storages, one per storage.
func (s *FanoutUpsamplerSuite) spyQueriers(storages ...*spyStorage) []*spyQuerier {
	queriers := make([]*spyQuerier, 0, len(storages))
	for _, st := range storages {
		queriers = append(queriers, st.queriers...)
	}
	s.Require().Len(queriers, len(storages))

	return queriers
}

//
// spyStorage
//

// spyStorage is a [storage.Storage] whose queriers record hints and Close calls.
type spyStorage struct {
	storage.Storage

	resolutionMS int64 // above zero — queriers additionally declare Resolution()
	noop         bool  // hand out an empty querier instead of a spy
	err          error // fail on Querier()
	queriers     []*spyQuerier
}

// Querier implements [storage.Storage].
func (s *spyStorage) Querier(mint, maxt int64) (storage.Querier, error) {
	if s.err != nil {
		return nil, s.err
	}

	if s.noop {
		return fanout.NoopQuerier(), nil
	}

	base, err := s.Storage.Querier(mint, maxt)
	if err != nil {
		return nil, err
	}

	q := &spyQuerier{Querier: base}
	s.queriers = append(s.queriers, q)

	if s.resolutionMS > 0 {
		return &resolutionSpyQuerier{spyQuerier: q, resolutionMS: s.resolutionMS}, nil
	}

	return q, nil
}

//
// spyQuerier
//

// spyQuerier is a [storage.Querier] recording the hints it was selected with and whether it has
// been closed.
type spyQuerier struct {
	storage.Querier

	hints  *storage.SelectHints
	closed bool
}

// Select implements [storage.Querier].
func (q *spyQuerier) Select(
	ctx context.Context,
	sortSeries bool,
	hints *storage.SelectHints,
	matchers ...*labels.Matcher,
) storage.SeriesSet {
	q.hints = hints

	return q.Querier.Select(ctx, sortSeries, hints, matchers...)
}

// Close implements [storage.Querier].
func (q *spyQuerier) Close() error {
	q.closed = true

	return q.Querier.Close()
}

//
// resolutionSpyQuerier
//

// resolutionSpyQuerier is a [spyQuerier] declaring a nominal resolution to fanout.
type resolutionSpyQuerier struct {
	*spyQuerier

	resolutionMS int64
}

// Resolution returns the nominal resolution of the underlying data source.
func (q *resolutionSpyQuerier) Resolution() int64 {
	return q.resolutionMS
}

//
// resolutionStorage
//

// resolutionStorage declares a nominal resolution on its queriers, the way Manager and Adapter
// do for downsampling blocks and downsampled heads.
type resolutionStorage struct {
	storage.Storage

	resolutionMS int64
}

// Querier implements [storage.Storage].
func (s resolutionStorage) Querier(mint, maxt int64) (storage.Querier, error) {
	q, err := s.Storage.Querier(mint, maxt)
	if err != nil {
		return nil, err
	}

	return upsampler.NewResolutionQuerier(q, s.resolutionMS), nil
}
