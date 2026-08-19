package fanout

import (
	"context"
	"errors"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/pp-pkg/blocks/upsampler"
	"github.com/prometheus/prometheus/storage"
)

//
// resolutionQuerier
//

// resolutionQuerier returns the nominal resolution of the underlying data source.
type resolutionQuerier interface {
	// Resolution returns the nominal resolution of the underlying data source.
	Resolution() int64
}

//
// fstorage
//

// fstorage is a [storage.Storage] with multiple underlying storages.
// Fork of storage/fanout.go: the merged querier is additionally wrapped in
// [upsampler.Querier] when any source declares a nominal resolution.
type fstorage struct {
	logger log.Logger

	primary     storage.Storage
	secondaries []storage.Storage
}

// New returns a new fanout [storage.Storage], which proxies reads and writes
// through to multiple underlying storages.
//
// The difference between primary and secondary Storage is only for read (Querier) path and it goes as follows:
// * If the primary querier returns an error, then any of the Querier operations will fail.
// * If any secondary querier returns an error the result from that queries is discarded.
// The overall operation will succeed, and the error from the secondary querier will be returned as a warning.
//
// NOTE: In the case of Prometheus, it treats all remote storages as secondary / best effort.
func New(logger log.Logger, primary storage.Storage, secondaries ...storage.Storage) storage.Storage {
	return &fstorage{
		logger:      logger,
		primary:     primary,
		secondaries: secondaries,
	}
}

// Appender returns a new appender for the storage. That proxies writes through to multiple underlying storages.
// The implementation can choose whether or not to use the context, for deadlines or to check for errors.
func (f *fstorage) Appender(ctx context.Context) storage.Appender {
	primary := f.primary.Appender(ctx)
	secondaries := make([]storage.Appender, 0, len(f.secondaries))
	for _, st := range f.secondaries {
		secondaries = append(secondaries, st.Appender(ctx))
	}

	return &fanoutAppender{
		logger:      f.logger,
		primary:     primary,
		secondaries: secondaries,
	}
}

// ChunkQuerier returns a new [storage.ChunkQuerier] that proxies
// reads and writes through to multiple underlying storages.
func (f *fstorage) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	primary, err := f.primary.ChunkQuerier(mint, maxt)
	if err != nil {
		return nil, err
	}

	secondaries := make([]storage.ChunkQuerier, 0, len(f.secondaries))
	for _, st := range f.secondaries {
		querier, err := st.ChunkQuerier(mint, maxt)
		if err != nil {
			// Close already open Queriers, append potential errors to returned error.
			errs := make([]error, 0, len(f.secondaries)+1)
			errs = append(errs, err, primary.Close())

			for _, q := range secondaries {
				errs = append(errs, q.Close())
			}

			return nil, errors.Join(errs...)
		}

		secondaries = append(secondaries, querier)
	}

	return storage.NewMergeChunkQuerier(
		[]storage.ChunkQuerier{primary},
		secondaries,
		storage.NewCompactingChunkSeriesMerger(storage.ChainedSeriesMerge),
	), nil
}

// Close closes the storage and all its underlying resources.
func (f *fstorage) Close() error {
	errs := make([]error, 0, len(f.secondaries)+1)
	errs = append(errs, f.primary.Close())

	for _, s := range f.secondaries {
		errs = append(errs, s.Close())
	}

	return errors.Join(errs...)
}

// Querier returns a new [storage.Querier] that proxies reads and writes through to multiple underlying storages.
func (f *fstorage) Querier(mint, maxt int64) (storage.Querier, error) {
	primary, err := f.primary.Querier(mint, maxt)
	if err != nil {
		return nil, err
	}

	var resolutionMS int64
	if rQuerier, ok := primary.(resolutionQuerier); ok {
		resolutionMS = rQuerier.Resolution()
	}

	secondaries := make([]storage.Querier, 0, len(f.secondaries))
	for _, st := range f.secondaries {
		querier, err := st.Querier(mint, maxt)
		if err != nil {
			// Close already open Queriers, append potential errors to returned error.
			errs := make([]error, 0, len(f.secondaries)+1)
			errs = append(errs, err, primary.Close())

			for _, q := range secondaries {
				errs = append(errs, q.Close())
			}

			return nil, errors.Join(errs...)
		}

		if _, ok := querier.(noopQuerier); ok {
			continue
		}

		if rQuerier, ok := querier.(resolutionQuerier); ok {
			resolutionMS = max(resolutionMS, rQuerier.Resolution())
		}

		secondaries = append(secondaries, querier)
	}

	q := storage.NewMergeQuerier([]storage.Querier{primary}, secondaries, storage.ChainedSeriesMerge)
	if resolutionMS != 0 {
		return upsampler.NewQuerier(q, resolutionMS), nil
	}

	return q, nil
}

// StartTime returns the oldest timestamp stored in the storage.
func (f *fstorage) StartTime() (int64, error) {
	// StartTime of a fanout should be the earliest StartTime of all its storages,
	// both primary and secondaries.
	firstTime, err := f.primary.StartTime()
	if err != nil {
		return int64(model.Latest), err
	}

	for _, s := range f.secondaries {
		t, err := s.StartTime()
		if err != nil {
			return int64(model.Latest), err
		}
		if t < firstTime {
			firstTime = t
		}
	}

	return firstTime, nil
}

//
// fanoutAppender
//

// fanoutAppender implements [storage.Appender]. That proxies writes through to multiple underlying storages.
type fanoutAppender struct {
	logger log.Logger

	primary     storage.Appender
	secondaries []storage.Appender
}

// Append adds a sample pair for the given series.
// If the series does not exist, it is created.
func (f *fanoutAppender) Append(ref storage.SeriesRef, l labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	ref, err := f.primary.Append(ref, l, t, v)
	if err != nil {
		return ref, err
	}

	for _, appender := range f.secondaries {
		if _, err := appender.Append(ref, l, t, v); err != nil {
			return 0, err
		}
	}

	return ref, nil
}

// AppendCTZeroSample adds synthetic zero sample for the given ct timestamp,
// which will be associated with given series, labels and the incoming sample's t (timestamp).
func (f *fanoutAppender) AppendCTZeroSample(
	ref storage.SeriesRef,
	l labels.Labels,
	t, ct int64,
) (storage.SeriesRef, error) {
	ref, err := f.primary.AppendCTZeroSample(ref, l, t, ct)
	if err != nil {
		return ref, err
	}

	for _, appender := range f.secondaries {
		if _, err := appender.AppendCTZeroSample(ref, l, t, ct); err != nil {
			return 0, err
		}
	}

	return ref, nil
}

// AppendExemplar adds an exemplar for the given series labels.
func (f *fanoutAppender) AppendExemplar(
	ref storage.SeriesRef,
	l labels.Labels,
	e exemplar.Exemplar,
) (storage.SeriesRef, error) {
	ref, err := f.primary.AppendExemplar(ref, l, e)
	if err != nil {
		return ref, err
	}

	for _, appender := range f.secondaries {
		if _, err := appender.AppendExemplar(ref, l, e); err != nil {
			return 0, err
		}
	}

	return ref, nil
}

// AppendHistogram adds a histogram for the given series labels.
func (f *fanoutAppender) AppendHistogram(
	ref storage.SeriesRef,
	l labels.Labels,
	t int64,
	h *histogram.Histogram,
	fh *histogram.FloatHistogram,
) (storage.SeriesRef, error) {
	ref, err := f.primary.AppendHistogram(ref, l, t, h, fh)
	if err != nil {
		return ref, err
	}

	for _, appender := range f.secondaries {
		if _, err := appender.AppendHistogram(ref, l, t, h, fh); err != nil {
			return 0, err
		}
	}

	return ref, nil
}

// Commit submits the collected samples and purges the batch. If Commit
// returns a non-nil error, it also rolls back all modifications made in
// the appender so far, as Rollback would do. In any case, an Appender
// must not be used anymore after Commit has been called.
func (f *fanoutAppender) Commit() (err error) {
	err = f.primary.Commit()

	for _, appender := range f.secondaries {
		if err == nil {
			err = appender.Commit()
		} else {
			if rollbackErr := appender.Rollback(); rollbackErr != nil {
				_ = level.Error(f.logger).Log("msg", "Squashed rollback error on commit", "err", rollbackErr)
			}
		}
	}

	return err
}

// Rollback rolls back all modifications made in the appender so far. Appender has to be discarded after rollback.
func (f *fanoutAppender) Rollback() (err error) {
	err = f.primary.Rollback()

	for _, appender := range f.secondaries {
		rollbackErr := appender.Rollback()
		switch {
		case err == nil:
			err = rollbackErr
		case rollbackErr != nil:
			_ = level.Error(f.logger).Log("msg", "Squashed rollback error on rollback", "err", rollbackErr)
		}
	}
	return err
}

// UpdateMetadata updates a metadata entry for the given series and labels.
func (f *fanoutAppender) UpdateMetadata(
	ref storage.SeriesRef,
	l labels.Labels,
	m metadata.Metadata,
) (storage.SeriesRef, error) {
	ref, err := f.primary.UpdateMetadata(ref, l, m)
	if err != nil {
		return ref, err
	}

	for _, appender := range f.secondaries {
		if _, err := appender.UpdateMetadata(ref, l, m); err != nil {
			return 0, err
		}
	}

	return ref, nil
}
