package storagetest

import (
	"context"
	"math"

	"github.com/prometheus/prometheus/pp/go/storage/head/shard"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/appender"
	"github.com/prometheus/prometheus/pp/go/storage/head/services"
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

func MustAppendTimeSeriesToLSSAndDataStorage(lss *shard.LSS, ds *shard.DataStorage, timeSeries ...TimeSeries) {
	for i := range timeSeries {
		modelTimeSeries := timeSeries[i].toModelTimeSeries()
		for j := range modelTimeSeries {
			foeResult := lss.Target().FindOrEmplace(modelTimeSeries[j].LabelSet)
			ds.Encode(foeResult.LabelSetID, int64(modelTimeSeries[j].Timestamp), modelTimeSeries[j].Value)
		}
	}
}

// SamplesMap samples map with series ID as key.
type SamplesMap map[uint32][]cppbridge.Sample

// GetSamplesFromSerializedChunks returns sample from serialized chunks.
func GetSamplesFromSerializedChunks(chunks *cppbridge.HeadDataStorageSerializedChunks) SamplesMap {
	result := make(SamplesMap)

	deserializer := cppbridge.NewHeadDataStorageDeserializer(chunks)

	n := chunks.NumberOfChunks()
	for i := 0; i < n; i++ {
		metadata := chunks.Metadata(i)
		seriesId := metadata.SeriesID()
		iterator := deserializer.CreateDecodeIterator(metadata)
		for iterator.Next() {
			ts, value := iterator.Sample()
			result[seriesId] = append(result[seriesId], cppbridge.Sample{Timestamp: ts, Value: value})
		}
	}

	return result
}

func GetSamplesFromSerializedData(serializedData *cppbridge.DataStorageSerializedData) SamplesMap {
	result := make(SamplesMap)

	for {
		seriesID, chunkRef := serializedData.Next()
		if seriesID == math.MaxUint32 {
			break
		}

		iterator := cppbridge.NewDataStorageSerializedDataIterator(serializedData, chunkRef)
		iterationResult := cppbridge.NewSerializedDataIteratorIterationResult()
		for {
			iterator.Next(&iterationResult)
			if !iterationResult.HasValue {
				break
			}
			result[seriesID] = append(result[seriesID], cppbridge.Sample{Timestamp: iterationResult.Timestamp, Value: iterationResult.Value})
		}
	}

	return result
}

// TimeSeriesFromSeriesSet converting seriesset to slice timeseries.
func TimeSeriesFromSeriesSet(seriesSet promstorage.SeriesSet, groupSamples bool) []TimeSeries {
	var chunkIterator chunkenc.Iterator
	timeSeries := make([]TimeSeries, 0)
	for seriesSet.Next() {
		series := seriesSet.At()
		timeSeries = append(timeSeries, TimeSeriesFromSeries(series, chunkIterator, groupSamples)...)
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
