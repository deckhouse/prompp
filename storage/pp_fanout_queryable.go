package storage

import (
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
)

//
// fanoutQueryable
//

// fanoutQueryable handles queries against a storage.
type fanoutQueryable struct {
	primaries   []Queryable
	secondaries []Queryable
}

// NewFanoutQueryable init new [fanoutQueryable] as [Queryable].
func NewFanoutQueryable(primary Queryable, secondaries ...Queryable) Queryable {
	primaries := make([]Queryable, 0, 2)
	primaries = append(primaries, primary)

	sq := make([]Queryable, 0, len(secondaries))
	for _, q := range secondaries {
		if f, ok := q.(*fanout); ok {
			primaries = append(primaries, f.primary)
			for _, s := range f.secondaries {
				sq = append(sq, s)
			}

			continue
		}

		sq = append(sq, q)
	}

	return &fanoutQueryable{
		primaries:   primaries,
		secondaries: sq,
	}
}

// Querier calls f() with the given parameters. Returns a merged [Querier].
func (fq *fanoutQueryable) Querier(mint, maxt int64) (Querier, error) {
	primaries := make([]Querier, 0, len(fq.primaries))
	for _, q := range fq.primaries {
		querier, err := q.Querier(mint, maxt)
		if err != nil {
			// Close already open Queriers, append potential errors to returned error.
			errs := tsdb_errors.NewMulti(err)
			for _, q := range primaries {
				errs.Add(q.Close())
			}
			return nil, errs.Err()
		}
		if _, ok := querier.(noopQuerier); ok {
			continue
		}

		primaries = append(primaries, querier)
	}

	secondaries := make([]Querier, 0, len(fq.secondaries))
	for _, q := range fq.secondaries {
		querier, err := q.Querier(mint, maxt)
		if err != nil {
			// Close already open Queriers, append potential errors to returned error.
			errs := tsdb_errors.NewMulti(err)
			for _, q := range primaries {
				errs.Add(q.Close())
			}
			for _, q := range secondaries {
				errs.Add(q.Close())
			}
			return nil, errs.Err()
		}
		if _, ok := querier.(noopQuerier); ok {
			continue
		}

		secondaries = append(secondaries, querier)
	}

	return NewMergeQuerierConcurrent(primaries, secondaries, ChainedSeriesMerge), nil
}
