package storagetest

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"time"
	"unsafe"

	"github.com/jonboulle/clockwork"
	"golang.org/x/sync/semaphore"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/appender"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/head/poolprovider"
	"github.com/prometheus/prometheus/pp/go/storage/head/services"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
	"github.com/prometheus/prometheus/pp/go/storage/head/task"
	"github.com/prometheus/prometheus/pp/go/storage/head/transactionhead"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	promstorage "github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/stretchr/testify/suite"
)

// TimeSeries test data.
type TimeSeries struct {
	Labels  labels.Labels
	Samples []cppbridge.Sample
}

// AppendSamples add samples to time series.
func (s *TimeSeries) AppendSamples(samples ...cppbridge.Sample) {
	s.Samples = append(s.Samples, samples...)
}

func (s *TimeSeries) toModelTimeSeries() []model.TimeSeries {
	lsBuilder := model.NewLabelSetBuilder()
	s.Labels.Range(func(l labels.Label) {
		lsBuilder.Add(l.Name, l.Value)
	})

	ls := lsBuilder.Build()

	timeSeries := make([]model.TimeSeries, 0, len(s.Samples))
	for i := range s.Samples {
		timeSeries = append(timeSeries, model.TimeSeries{
			LabelSet:  ls,
			Timestamp: uint64(s.Samples[i].Timestamp), // #nosec G115 // no overflow
			Value:     s.Samples[i].Value,
		})
	}

	return timeSeries
}

type timeSeriesDataSlice struct {
	timeSeries []model.TimeSeries
}

func (tsd *timeSeriesDataSlice) TimeSeries() []model.TimeSeries {
	return tsd.timeSeries
}

func (tsd *timeSeriesDataSlice) Destroy() {
	tsd.timeSeries = nil
}

// MustAppendTimeSeries add time series to head.
func MustAppendTimeSeries(s *suite.Suite, head *storage.Head, timeSeries []TimeSeries) {
	headAppender := appender.New(head, services.CFViaRange)

	statelessRelabeler, err := cppbridge.NewStatelessRelabeler([]*cppbridge.RelabelConfig{})
	s.Require().NoError(err)

	state := cppbridge.NewStateV2WithoutLock()
	state.SetStatelessRelabeler(statelessRelabeler)

	for i := range timeSeries {
		_, err = headAppender.Append(
			context.Background(),
			NewIncomingData(s, timeSeries[i].toModelTimeSeries()),
			state,
			true)
		s.NoError(err)
	}
}

func NewIncomingData(s *suite.Suite, timeSeries []model.TimeSeries) *appender.IncomingData {
	tsd := timeSeriesDataSlice{timeSeries: timeSeries}
	hx, err := (cppbridge.HashdexFactory{}).GoModel(tsd.TimeSeries(), cppbridge.DefaultWALHashdexLimits())
	s.Require().NoError(err)

	return &appender.IncomingData{Hashdex: hx, Data: &tsd}
}

// MustAppendTimeSeriesToLSSAndDataStorage adds timeSeries to the given lss and ds in a single
// batch via a one-shard [transactionhead.Head] driven by the regular [appender]. This collapses
// what used to be 2*N cgo calls per sample (FindOrEmplace + Encode) into a single
// hashdex-based Append, which materially speeds up tests on slow toolchains (ASAN, ARM CI).
func MustAppendTimeSeriesToLSSAndDataStorage(lss *shard.LSS, ds *shard.DataStorage, timeSeries ...TimeSeries) {
	if len(timeSeries) == 0 {
		return
	}

	totalSamples := 0
	for i := range timeSeries {
		totalSamples += len(timeSeries[i].Samples)
	}
	flat := make([]model.TimeSeries, 0, totalSamples)
	for i := range timeSeries {
		flat = append(flat, timeSeries[i].toModelTimeSeries()...)
	}

	hx, err := (cppbridge.HashdexFactory{}).GoModel(flat, cppbridge.DefaultWALHashdexLimits())
	if err != nil {
		panic(fmt.Errorf("storagetest: build hashdex: %w", err))
	}

	statelessRelabeler, err := cppbridge.NewStatelessRelabeler(nil)
	if err != nil {
		panic(fmt.Errorf("storagetest: stateless relabeler: %w", err))
	}
	state := cppbridge.NewStateV2WithoutLock()
	state.SetStatelessRelabeler(statelessRelabeler)

	sd := shard.NewShard(lss, ds, nil, nil, wal.NewNoopWal(), 0)
	pools := poolprovider.NewHeadPool[*shard.PerGoroutineShard](1)
	th := transactionhead.NewHead[*shard.Shard, *shard.PerGoroutineShard](
		"storagetest",
		sd,
		shard.NewPerGoroutineShard[wal.NoopWal](sd, 1),
		pools,
	)

	app := appender.New[
		*task.Generic[*shard.PerGoroutineShard],
		*shard.Shard,
		*shard.PerGoroutineShard,
		*storage.TransactionHead,
	](th, func(*storage.TransactionHead) error { return nil })

	if _, err = app.Append(
		context.Background(),
		&appender.IncomingData{Hashdex: hx, Data: &timeSeriesDataSlice{timeSeries: flat}},
		state,
		false,
	); err != nil {
		panic(fmt.Errorf("storagetest: append: %w", err))
	}
}

// SamplesMap samples map with series ID as key.
type SamplesMap map[uint32][]cppbridge.Sample

func GetSamplesFromSerializedData(serializedData *cppbridge.DataStorageSerializedData) SamplesMap {
	result := make(SamplesMap)

	for {
		seriesID, chunkRef := serializedData.Next()
		if seriesID == math.MaxUint32 {
			break
		}

		iterator := cppbridge.NewDataStorageSerializedDataIterator(serializedData, chunkRef)
		for {
			if !iterator.HasData() {
				break
			}

			result[seriesID] = append(result[seriesID], cppbridge.Sample{Timestamp: iterator.Timestamp, Value: iterator.Value})
			iterator.Next()
		}
	}

	return result
}

// TimeSeriesFromSeriesSet converts a [promstorage.SeriesSet] to a slice of [TimeSeries] preserving
// the iteration order. Sample reading is parallelised with a GOMAXPROCS-sized
// [semaphore.Weighted]: the SeriesSet itself is drained sequentially (cheap; one cgo Next per
// series), then each Series is iterated in its own goroutine via a fresh chunk iterator. Under
// ASAN/ARM the per-sample cgo reads dominate and parallelisation gives a noticeable speedup.
//
// The same semaphore replaces a WaitGroup: after all goroutines are launched the caller waits by
// acquiring all `workers` slots, which only succeeds once every Release has fired.
func TimeSeriesFromSeriesSet(seriesSet promstorage.SeriesSet, groupSamples bool) []TimeSeries {
	var allSeries []promstorage.Series
	for seriesSet.Next() {
		allSeries = append(allSeries, seriesSet.At())
	}
	if len(allSeries) == 0 {
		return []TimeSeries{}
	}

	perSeries := make([][]TimeSeries, len(allSeries))
	workers := runtime.GOMAXPROCS(0)
	if workers > len(allSeries) {
		workers = len(allSeries)
	}

	ctx := context.Background()
	sem := semaphore.NewWeighted(int64(workers))
	for i := range allSeries {
		// Acquire blocks until a worker slot is free; with context.Background it never errors.
		_ = sem.Acquire(ctx, 1)
		go func(i int) {
			defer sem.Release(1)
			perSeries[i] = TimeSeriesFromSeries(allSeries[i], nil, groupSamples)
		}(i)
	}
	// Wait for every in-flight goroutine by reserving all worker slots.
	_ = sem.Acquire(ctx, int64(workers))

	total := 0
	for _, p := range perSeries {
		total += len(p)
	}
	timeSeries := make([]TimeSeries, 0, total)
	for _, p := range perSeries {
		timeSeries = append(timeSeries, p...)
	}
	return timeSeries
}

func TimeSeriesFromSeries(series promstorage.Series, chunkIterator chunkenc.Iterator, groupSamples bool) (timeSeries []TimeSeries) {
	chunkIterator = series.Iterator(chunkIterator)
	var samples []cppbridge.Sample
	for chunkIterator.Next() != chunkenc.ValNone {
		ts, v := chunkIterator.At()
		samples = append(samples, cppbridge.Sample{Timestamp: ts, Value: v})
	}

	if groupSamples {
		timeSeries = append(timeSeries, TimeSeries{Labels: series.Labels(), Samples: samples})
		return timeSeries
	}

	for i := 0; i < len(samples); i++ {
		timeSeries = append(timeSeries, TimeSeries{Labels: series.Labels(), Samples: []cppbridge.Sample{samples[i]}})
	}

	return timeSeries
}

func IterateSeriesSet(seriesSet promstorage.SeriesSet, iterator chunkenc.Iterator) chunkenc.Iterator {
	var series promstorage.Series
	for seriesSet.Next() {
		series = seriesSet.At()
		iterator = series.Iterator(iterator)
		for iterator.Next() != chunkenc.ValNone {
			ts, v := iterator.At()
			_, _ = ts, v
		}
	}
	return iterator
}

func InstantQuery(lss *shard.LSS, ds *shard.DataStorage, targetTimestamp, valueNotFoundTimestampValue int64, matchers ...model.LabelMatcher) (*querier.InstantSeriesSet, error) {
	selector, snapshot, err := lss.QuerySelector(0, matchers)
	if err != nil {
		return nil, err
	}
	if selector == 0 || snapshot == nil {
		return &querier.InstantSeriesSet{}, nil
	}

	lssQueryResult := snapshot.Query(selector)
	if lssQueryResult.Status() == cppbridge.LSSQueryStatusNoMatch {
		return &querier.InstantSeriesSet{}, nil
	}

	instantSeries := querier.NewInstantSeriesSlice(lssQueryResult.Len(), valueNotFoundTimestampValue)

	dsQueryResult := ds.InstantQuery(targetTimestamp, lssQueryResult.IDs(), uintptr(unsafe.Pointer(unsafe.SliceData(instantSeries))))
	if dsQueryResult.Status != cppbridge.DataStorageQueryStatusSuccess {
		return nil, fmt.Errorf("invalid data storage query result status")
	}

	return querier.NewInstantSeriesSet(lssQueryResult, snapshot, valueNotFoundTimestampValue, instantSeries), nil
}

const (
	NumberOfShards            uint16        = 2
	MaxSegmentSize            uint32        = 1024
	UnloadDataStorageInterval time.Duration = 100
)

func CreateCatalog(clock clockwork.Clock, logFilePath string, idGenerator catalog.IDGenerator) (*catalog.Catalog, error) {
	l, err := catalog.NewFileLogV2(logFilePath)
	if err != nil {
		return nil, err
	}

	ctlg, err := catalog.New(
		clock,
		l,
		idGenerator,
		catalog.DefaultMaxLogFileSize,
		nil,
	)
	if err != nil {
		return nil, err
	}

	return ctlg, nil
}

// CreateDataDirectory creates a data directory.
func CreateDataDirectory(dir string) (string, error) {
	dataDir := filepath.Join(dir, "data")
	return dataDir, os.MkdirAll(dataDir, os.ModeDir)
}

// StaleNaNQuery tests a staleNaN query on the LSS and DataStorage.
func StaleNaNQuery(
	lss *shard.LSS,
	ds *shard.DataStorage,
	valueNotFoundTimestampValue int64,
	matchers ...model.LabelMatcher,
) (*querier.StaleNaNSeriesSet, error) {
	selector, snapshot, err := lss.QuerySelector(0, matchers)
	if err != nil {
		return nil, err
	}

	if selector == 0 || snapshot == nil {
		return &querier.StaleNaNSeriesSet{}, nil
	}

	lssQueryResult := snapshot.Query(selector)
	if lssQueryResult.Status() == cppbridge.LSSQueryStatusNoMatch {
		return &querier.StaleNaNSeriesSet{}, nil
	}

	timestamps := querier.MakeTimestampsSliceWithDefault(lssQueryResult.Len(), valueNotFoundTimestampValue)
	ds.QueryFirstTimestamps(lssQueryResult.IDs(), timestamps)

	return querier.NewStaleNaNSeriesSet(
		querier.NewStaleNaNSeriesSliceFromTimestamps(timestamps),
		lssQueryResult,
		snapshot,
		valueNotFoundTimestampValue,
	), nil
}
