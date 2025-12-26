package storage

import (
	"context"

	"github.com/prometheus/prometheus/pp-pkg/model"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	pp_model "github.com/prometheus/prometheus/pp/go/model"
	pp_storage "github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/appender"
	"github.com/prometheus/prometheus/pp/go/storage/head/services"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/storage"
)

// BatchStorage appender for rules, aggregates the [model.TimeSeries] batch and append to [pp_storage.TransactionHead],
// on commit append from [pp_storage.TransactionHead] to [Head]. It can read as [storage.Querier] the added data.
type BatchStorage struct {
	hashdexFactory  cppbridge.HashdexFactory
	hashdexLimits   cppbridge.WALHashdexLimits
	transactionHead *pp_storage.TransactionHead
	state           *cppbridge.StateV2
	// TODO tmp
	batch   *timeSeriesBatch
	adapter *Adapter
}

// NewBatchStorage init new [BatchStorage].
func NewBatchStorage(
	hashdexFactory cppbridge.HashdexFactory,
	hashdexLimits cppbridge.WALHashdexLimits,
	transactionHead *pp_storage.TransactionHead,
	state *cppbridge.StateV2,
	adapter *Adapter,
) *BatchStorage {
	return &BatchStorage{
		hashdexFactory:  hashdexFactory,
		hashdexLimits:   hashdexLimits,
		transactionHead: transactionHead,
		state:           state,
		batch:           &timeSeriesBatch{timeSeries: make([]pp_model.TimeSeries, 0, 10)},
		adapter:         adapter,
	}
}

// Appender creates a new [storage.Appender] for appending time series data to [pp_storage.TransactionHead].
func (bs *BatchStorage) Appender(ctx context.Context) storage.Appender {
	return newTimeSeriesAppender(ctx, bs, bs.state)
}

// AppendTimeSeries append TimeSeries data to [pp_storage.TransactionHead].
func (bs *BatchStorage) AppendTimeSeries(
	ctx context.Context,
	data model.TimeSeriesBatch,
	state *cppbridge.StateV2,
	commitToWal bool,
) (stats cppbridge.RelabelerStats, err error) {
	hx, err := bs.hashdexFactory.GoModel(data.TimeSeries(), bs.hashdexLimits)
	if err != nil {
		data.Destroy()
		return stats, err
	}

	tdata := data.TimeSeries()
	stats, err = appender.New(bs.transactionHead, services.CFViaRange).Append(
		ctx,
		&appender.IncomingData{Hashdex: hx, Data: data},
		state,
		commitToWal,
	)
	bs.batch.timeSeries = append(bs.batch.timeSeries, tdata...)

	return stats, err
}

// Commit adds aggregated series from [pp_storage.TransactionHead] to the [Head].
func (bs *BatchStorage) Commit(ctx context.Context) error {
	if len(bs.batch.timeSeries) == 0 {
		return nil
	}

	_, err := bs.adapter.AppendTimeSeries(ctx, bs.batch, bs.state, false)
	_ = bs.transactionHead.Close()
	return err
}

// Querier calls f() with the given parameters. Returns a [querier.Querier].
func (bs *BatchStorage) Querier(mint, maxt int64) (storage.Querier, error) {
	return querier.NewQuerier(bs.transactionHead, querier.NewNoOpShardedDeduplicator, mint, maxt, nil, nil), nil
}
