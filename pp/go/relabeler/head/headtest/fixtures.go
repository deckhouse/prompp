package headtest

import (
	"context"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/stretchr/testify/suite"
)

type TimeSeries struct {
	Labels  labels.Labels
	Samples []cppbridge.Sample
}

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
			Timestamp: uint64(s.Samples[i].Timestamp),
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

func MustAppendTimeSeries(s *suite.Suite, head relabeler.Head, relabelerId string, timeSeries []TimeSeries) {
	for i := range timeSeries {
		tsd := timeSeriesDataSlice{timeSeries: timeSeries[i].toModelTimeSeries()}
		hx, err := (cppbridge.HashdexFactory{}).GoModel(tsd.TimeSeries(), cppbridge.DefaultWALHashdexLimits())
		s.NoError(err)

		_, _, err = head.Append(
			context.Background(),
			&relabeler.IncomingData{Hashdex: hx, Data: &tsd},
			cppbridge.NewState(head.NumberOfShards()),
			relabelerId,
			true)
		s.NoError(err)
	}
}

func TimeSeriesFromSeriesSet(seriesSet storage.SeriesSet) []TimeSeries {
	var timeSeries []TimeSeries
	for seriesSet.Next() {
		series := seriesSet.At()

		timeSeries = append(timeSeries, TimeSeries{Labels: series.Labels()})
		currentSeries := &timeSeries[len(timeSeries)-1]

		chunkIterator := series.Iterator(nil)
		for chunkIterator.Next() != chunkenc.ValNone {
			ts, v := chunkIterator.At()
			currentSeries.Samples = append(currentSeries.Samples, cppbridge.Sample{Timestamp: ts, Value: v})
		}
	}

	return timeSeries
}
