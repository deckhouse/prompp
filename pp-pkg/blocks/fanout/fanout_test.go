package fanout_test

import (
	"context"
	"errors"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp-pkg/blocks/fanout"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/teststorage"
)

type FanoutSuite struct {
	suite.Suite
}

func TestFanoutSuite(t *testing.T) {
	suite.Run(t, new(FanoutSuite))
}

func (s *FanoutSuite) TestFanout_SelectSorted() {
	inputLabel := labels.FromStrings(model.MetricNameLabel, "a")
	outputLabel := labels.FromStrings(model.MetricNameLabel, "a")

	inputTotalSize := 0
	ctx := s.T().Context()

	priStorage := teststorage.New(s.T())
	defer priStorage.Close()
	app1 := priStorage.Appender(ctx)
	app1.Append(0, inputLabel, 0, 0)
	inputTotalSize++
	app1.Append(0, inputLabel, 1000, 1)
	inputTotalSize++
	app1.Append(0, inputLabel, 2000, 2)
	inputTotalSize++
	err := app1.Commit()
	s.Require().NoError(err)

	remoteStorage1 := teststorage.New(s.T())
	defer remoteStorage1.Close()
	app2 := remoteStorage1.Appender(ctx)
	app2.Append(0, inputLabel, 3000, 3)
	inputTotalSize++
	app2.Append(0, inputLabel, 4000, 4)
	inputTotalSize++
	app2.Append(0, inputLabel, 5000, 5)
	inputTotalSize++
	err = app2.Commit()
	s.Require().NoError(err)

	remoteStorage2 := teststorage.New(s.T())
	defer remoteStorage2.Close()

	app3 := remoteStorage2.Appender(ctx)
	app3.Append(0, inputLabel, 6000, 6)
	inputTotalSize++
	app3.Append(0, inputLabel, 7000, 7)
	inputTotalSize++
	app3.Append(0, inputLabel, 8000, 8)
	inputTotalSize++

	err = app3.Commit()
	s.Require().NoError(err)

	fanoutStorage := fanout.New(nil, priStorage, remoteStorage1, remoteStorage2)

	s.Run("querier", func() {
		querier, err := fanoutStorage.Querier(0, 8000)
		s.Require().NoError(err)
		defer querier.Close()

		matcher, err := labels.NewMatcher(labels.MatchEqual, model.MetricNameLabel, "a")
		s.Require().NoError(err)

		seriesSet := querier.Select(ctx, true, nil, matcher)

		result := make(map[int64]float64)
		var labelsResult labels.Labels
		var iterator chunkenc.Iterator
		for seriesSet.Next() {
			series := seriesSet.At()
			seriesLabels := series.Labels()
			labelsResult = seriesLabels
			iterator = series.Iterator(iterator)
			for iterator.Next() == chunkenc.ValFloat {
				timestamp, value := iterator.At()
				result[timestamp] = value
			}
		}

		s.Require().Equal(labelsResult, outputLabel)
		s.Require().Len(result, inputTotalSize)
	})
	s.Run("chunk querier", func() {
		querier, err := fanoutStorage.ChunkQuerier(0, 8000)
		s.Require().NoError(err)
		defer querier.Close()

		matcher, err := labels.NewMatcher(labels.MatchEqual, model.MetricNameLabel, "a")
		s.Require().NoError(err)

		seriesSet := storage.NewSeriesSetFromChunkSeriesSet(querier.Select(ctx, true, nil, matcher))

		result := make(map[int64]float64)
		var labelsResult labels.Labels
		var iterator chunkenc.Iterator
		for seriesSet.Next() {
			series := seriesSet.At()
			seriesLabels := series.Labels()
			labelsResult = seriesLabels
			iterator = series.Iterator(iterator)
			for iterator.Next() == chunkenc.ValFloat {
				timestamp, value := iterator.At()
				result[timestamp] = value
			}
		}

		s.Require().NoError(seriesSet.Err())
		s.Require().Equal(labelsResult, outputLabel)
		s.Require().Len(result, inputTotalSize)
	})
}

//revive:disable-next-line:cognitive-complexity // this is test
func (s *FanoutSuite) TestFanoutErrors() {
	workingStorage := teststorage.New(s.T())
	defer workingStorage.Close()

	cases := []struct {
		primary   storage.Storage
		secondary storage.Storage
		warning   error
		err       error
	}{
		{
			primary:   workingStorage,
			secondary: errStorage{},
			warning:   errSelect,
			err:       nil,
		},
		{
			primary:   errStorage{},
			secondary: workingStorage,
			warning:   nil,
			err:       errSelect,
		},
	}

	for _, tc := range cases {
		fanoutStorage := fanout.New(nil, tc.primary, tc.secondary)

		s.Run("samples", func() {
			querier, err := fanoutStorage.Querier(0, 8000)
			s.Require().NoError(err)
			defer querier.Close()

			matcher := labels.MustNewMatcher(labels.MatchEqual, "a", "b")
			ss := querier.Select(context.Background(), true, nil, matcher)

			// Exhaust.
			for ss.Next() {
				ss.At()
			}

			if tc.err != nil {
				s.Require().Error(ss.Err())
				s.Require().Equal(tc.err.Error(), ss.Err().Error())
			}

			if tc.warning != nil {
				s.Require().NotEmpty(ss.Warnings(), "warnings expected")
				w := ss.Warnings()
				s.Require().Error(w.AsErrors()[0])
				warn, _ := w.AsStrings("", 0, 0)
				s.Require().Equal(tc.warning.Error(), warn[0])
			}
		})
		s.Run("chunks", func() {
			s.T().Skip("enable once TestStorage and TSDB implements ChunkQuerier")
			querier, err := fanoutStorage.ChunkQuerier(0, 8000)
			s.Require().NoError(err)
			defer querier.Close()

			matcher := labels.MustNewMatcher(labels.MatchEqual, "a", "b")
			ss := querier.Select(context.Background(), true, nil, matcher)

			// Exhaust.
			for ss.Next() {
				ss.At()
			}

			if tc.err != nil {
				s.Require().Error(ss.Err())
				s.Require().Equal(tc.err.Error(), ss.Err().Error())
			}

			if tc.warning != nil {
				s.Require().NotEmpty(ss.Warnings(), "warnings expected")
				w := ss.Warnings()
				s.Require().Error(w.AsErrors()[0])
				warn, _ := w.AsStrings("", 0, 0)
				s.Require().Equal(tc.warning.Error(), warn[0])
			}
		})
	}
}

//
// errStorage
//

// errStorage implements [storage.Storage].
type errStorage struct{}

// Appender implements [storage.Storage].
func (errStorage) Appender(_ context.Context) storage.Appender { return nil }

// ChunkAppender implements [storage.Storage].
func (errStorage) ChunkQuerier(_, _ int64) (storage.ChunkQuerier, error) {
	return errChunkQuerier{}, nil
}

// Close implements [storage.Storage].
func (errStorage) Close() error { return nil }

// Querier implements [storage.Storage].
func (errStorage) Querier(_, _ int64) (storage.Querier, error) {
	return errQuerier{}, nil
}

// StartTime implements [storage.Storage].
func (errStorage) StartTime() (int64, error) { return 0, nil }

//
// errQuerier
//

// errSelect error for testing Select.
var errSelect = errors.New("select error")

// errQuerier implements [storage.Querier].
type errQuerier struct{}

// Close implements [storage.Querier].
func (errQuerier) Close() error { return nil }

// LabelNames implements [storage.Querier].
func (errQuerier) LabelNames(
	context.Context,
	*storage.LabelHints,
	...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return nil, nil, errors.New("label names error")
}

// Select implements [storage.Querier].
func (errQuerier) Select(
	context.Context,
	bool,
	*storage.SelectHints,
	...*labels.Matcher,
) storage.SeriesSet {
	return storage.ErrSeriesSet(errSelect)
}

// LabelValues implements [storage.Querier].
func (errQuerier) LabelValues(
	context.Context,
	string,
	*storage.LabelHints,
	...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return nil, nil, errors.New("label values error")
}

//
// errChunkQuerier
//

// errChunkQuerier implements [storage.ChunkQuerier].
type errChunkQuerier struct{ errQuerier }

// Select implements [storage.ChunkQuerier].
func (errChunkQuerier) Select(context.Context, bool, *storage.SelectHints, ...*labels.Matcher) storage.ChunkSeriesSet {
	return storage.ErrChunkSeriesSet(errSelect)
}
