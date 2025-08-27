package storage

import (
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
)

//
// MetricData
//

// MetricData is an universal interface for blob protobuf data or batch [model.TimeSeries].
type MetricData interface {
	// Destroy incoming data.
	Destroy()
}

//
// ProtobufData
//

// ProtobufData is an universal interface for blob protobuf data.
type ProtobufData interface {
	Bytes() []byte
	Destroy()
}

//
// TimeSeriesData
//

// TimeSeriesBatch is an universal interface for batch [model.TimeSeries].
type TimeSeriesBatch interface {
	TimeSeries() []model.TimeSeries
	Destroy()
}

//
// IncomingData
//

// IncomingData implements.
type IncomingData struct {
	Hashdex cppbridge.ShardedData
	Data    MetricData
}

// ShardedData return hashdex.
func (i *IncomingData) ShardedData() cppbridge.ShardedData {
	return i.Hashdex
}

// Destroy IncomingData.
func (i *IncomingData) Destroy() {
	i.Hashdex = nil
	if i.Data != nil {
		i.Data.Destroy()
	}
}

//
// HeadStatus
//

// HeadStatus holds information about all shards.
type HeadStatus struct {
	HeadStats                   HeadStats  `json:"headStats"`
	SeriesCountByMetricName     []HeadStat `json:"seriesCountByMetricName"`
	LabelValueCountByLabelName  []HeadStat `json:"labelValueCountByLabelName"`
	MemoryInBytesByLabelName    []HeadStat `json:"memoryInBytesByLabelName"`
	SeriesCountByLabelValuePair []HeadStat `json:"seriesCountByLabelValuePair"`
}

// HeadStat holds the information about individual cardinality.
type HeadStat struct {
	Name  string `json:"name"`
	Value uint64 `json:"value"`
}

// HeadStats has information about the head.
type HeadStats struct {
	NumSeries     uint64 `json:"numSeries"`
	NumLabelPairs int    `json:"numLabelPairs"`
	ChunkCount    int64  `json:"chunkCount"`
	MinTime       int64  `json:"minTime"`
	MaxTime       int64  `json:"maxTime"`
}
