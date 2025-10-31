package storage

import (
	"context"
	"fmt"

	"github.com/prometheus/prometheus/pp-pkg/model"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	pp_storage "github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/appender"
	"github.com/prometheus/prometheus/pp/go/storage/head/services"
	"github.com/prometheus/prometheus/storage"
)

// BatchAppender appender for rules, aggregates the [model.TimeSeries] batch and append to [pp_storage.TransactionHead],
// on commit append from [pp_storage.TransactionHead] to [Head].
type BatchAppender struct {
	hashdexFactory  cppbridge.HashdexFactory
	hashdexLimits   cppbridge.WALHashdexLimits
	transactionHead *pp_storage.TransactionHead
	state           *cppbridge.StateV2
}

// NewBatchAppender init new [BatchAppender].
func NewBatchAppender(
	hashdexFactory cppbridge.HashdexFactory,
	hashdexLimits cppbridge.WALHashdexLimits,
	transactionHead *pp_storage.TransactionHead,
	state *cppbridge.StateV2,
) *BatchAppender {
	return &BatchAppender{
		hashdexFactory:  hashdexFactory,
		hashdexLimits:   hashdexLimits,
		transactionHead: transactionHead,
		state:           state,
	}
}

// Appender creates a new [storage.Appender] for appending time series data to [pp_storage.TransactionHead].
func (ba *BatchAppender) Appender(ctx context.Context) storage.Appender {
	return newTimeSeriesAppender(ctx, ba, ba.state)
}

// AppendTimeSeries append TimeSeries data to [pp_storage.TransactionHead].
func (ba *BatchAppender) AppendTimeSeries(
	ctx context.Context,
	data model.TimeSeriesBatch,
	state *cppbridge.StateV2,
	commitToWal bool,
) (stats cppbridge.RelabelerStats, err error) {
	hx, err := ba.hashdexFactory.GoModel(data.TimeSeries(), ba.hashdexLimits)
	if err != nil {
		data.Destroy()
		return stats, err
	}

	_, stats, err = appender.New(ba.transactionHead, services.CFViaRange).Append(
		ctx,
		&appender.IncomingData{Hashdex: hx, Data: data},
		state,
		commitToWal,
	)

	return stats, err
}

// Commit adds aggregated series from [pp_storage.TransactionHead] to the [Head].
func (ba *BatchAppender) Commit() error {
	fmt.Println(" === BatchAppender Commit")
	return nil
}
