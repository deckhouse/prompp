package storagetest

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/appender"
	"github.com/prometheus/prometheus/pp/go/storage/head/services"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	promstorage "github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// TimeSeries test data.
type TimeSeries struct {
	Labels  labels.Labels
	Samples []cppbridge.Sample
}

// String serialize time series to string.
func (s *TimeSeries) String() string {
	builder := strings.Builder{}

	_, _ = builder.WriteString("timeSeries:{labels:")
	_, _ = builder.WriteString(s.Labels.String())
	_, _ = builder.WriteString(",samples:[")

	for i := range s.Samples {
		if i > 0 {
			_, _ = builder.WriteString(",")
		}
		_, _ = builder.WriteString(fmt.Sprintf("{ts:%d, value:%f}", s.Samples[i].Timestamp, s.Samples[i].Value))
	}
	_, _ = builder.WriteString("]}")

	return builder.String()
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
		tsd := timeSeriesDataSlice{timeSeries: timeSeries[i].toModelTimeSeries()}
		hx, err := (cppbridge.HashdexFactory{}).GoModel(tsd.TimeSeries(), cppbridge.DefaultWALHashdexLimits())
		s.Require().NoError(err)

		_, _, err = headAppender.Append(
			context.Background(),
			&appender.IncomingData{Hashdex: hx, Data: &tsd},
			state,
			true)
		s.NoError(err)
	}
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
		nextResult := cppbridge.SerializedDataIteratorNextResult{}
		for {
			iterator.Next(&nextResult)
			if !nextResult.HasValue {
				break
			}
			result[seriesID] = append(result[seriesID], cppbridge.Sample{Timestamp: nextResult.Timestamp, Value: nextResult.Value})
		}
	}

	return result
}

// TimeSeriesFromSeriesSet converting seriesset to slice timeseries.
func TimeSeriesFromSeriesSet(seriesSet promstorage.SeriesSet, groupSamples bool) []TimeSeries {
	timeSeries := make([]TimeSeries, 0)
	for seriesSet.Next() {
		series := seriesSet.At()
		chunkIterator := series.Iterator(nil)
		var samples []cppbridge.Sample
		for chunkIterator.Next() != chunkenc.ValNone {
			ts, v := chunkIterator.At()
			samples = append(samples, cppbridge.Sample{Timestamp: ts, Value: v})
		}

		if groupSamples {
			timeSeries = append(timeSeries, TimeSeries{Labels: series.Labels(), Samples: samples})
			continue
		}

		for i := 0; i < len(samples); i++ {
			timeSeries = append(timeSeries, TimeSeries{Labels: series.Labels(), Samples: []cppbridge.Sample{samples[i]}})
		}
	}

	return timeSeries
}

// TimeSeriesToString serialize time series to string.
func TimeSeriesToString(timeSeries []TimeSeries) string {
	res := make([]string, 0, len(timeSeries))
	for i := range timeSeries {
		res = append(res, timeSeries[i].String())
	}
	return strings.Join(res, ",")
}
