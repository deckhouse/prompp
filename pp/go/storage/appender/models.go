package appender

import (
	"sync/atomic"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

type ShardedData[DataType any] struct {
	series         []DataType
	numberOfShards uint16
}

func NewShardedData[DataType any](numberOfShards uint16) ShardedData[DataType] {
	return ShardedData[DataType]{
		series:         make([]DataType, numberOfShards*numberOfShards),
		numberOfShards: numberOfShards,
	}
}

func (sd *ShardedData[DataType]) DataByShard(shardID uint16) []DataType {
	return sd.series[shardID*sd.numberOfShards : (shardID+1)*sd.numberOfShards]
}

func (sd *ShardedData[DataType]) Transpose() {
	for i := uint16(0); i < sd.numberOfShards; i++ {
		for j := i + 1; j < sd.numberOfShards; j++ {
			sd.series[i*sd.numberOfShards+j], sd.series[j*sd.numberOfShards+i] = sd.series[j*sd.numberOfShards+i], sd.series[i*sd.numberOfShards+j]
		}
	}
}

//
// ShardedInnerSeries
//

type ShardedInnerSeries struct {
	ShardedData[*cppbridge.InnerSeries]
}

func NewShardedInnerSeries(numberOfShards uint16) *ShardedInnerSeries {
	series := &ShardedInnerSeries{
		NewShardedData[*cppbridge.InnerSeries](numberOfShards),
	}
	for i := range series.series {
		series.series[i] = cppbridge.NewInnerSeries()
	}

	return series
}

//
// ShardedRelabeledSeries
//

type ShardedRelabeledSeries struct {
	ShardedData[*cppbridge.RelabeledSeries]
}

// NewShardedRelabeledSeries init new ShardedRelabeledSeries.
func NewShardedRelabeledSeries(numberOfShards uint16) *ShardedRelabeledSeries {
	series := &ShardedRelabeledSeries{
		NewShardedData[*cppbridge.RelabeledSeries](numberOfShards),
	}
	for i := range series.series {
		series.series[i] = cppbridge.NewRelabeledSeries()
	}

	return series
}

// IsEmpty return true if all elements are empty
func (sd *ShardedRelabeledSeries) IsEmpty() bool {
	for i := range sd.series {
		if sd.series[i].Size() != 0 {
			return false
		}
	}

	return true
}

//
// ShardedStateUpdates
//

type ShardedStateUpdates struct {
	ShardedData[*cppbridge.RelabelerStateUpdate]
}

// NewShardedStateUpdates init new ShardedStateUpdates.
func NewShardedStateUpdates(numberOfShards uint16) *ShardedStateUpdates {
	series := &ShardedStateUpdates{
		NewShardedData[*cppbridge.RelabelerStateUpdate](numberOfShards),
	}
	for i := range series.series {
		series.series[i] = cppbridge.NewRelabelerStateUpdate()
	}

	return series
}

//
// MetricData
//

// MetricData is an universal interface for blob protobuf data or batch [model.TimeSeries].
type MetricData interface {
	// Destroy incoming data.
	Destroy()
}

//
// IncomingData
//

// IncomingData incoming [cppbridge.ShardedData] for shard distribution.
type IncomingData struct {
	Hashdex cppbridge.ShardedData
	Data    MetricData
}

// Destroy IncomingData.
func (i *IncomingData) Destroy() {
	i.Hashdex = nil
	if i.Data != nil {
		i.Data.Destroy()
	}
}

// ShardedData return hashdex.
func (i *IncomingData) ShardedData() cppbridge.ShardedData {
	return i.Hashdex
}

//
// DestructibleIncomingData
//

// DestructibleIncomingData wrapeer over [IncomingData] with detroy-counter.
type DestructibleIncomingData struct {
	data          *IncomingData
	destructCount atomic.Int64
}

// NewDestructibleIncomingData init new [DestructibleIncomingData].
func NewDestructibleIncomingData(data *IncomingData, destructCount int) *DestructibleIncomingData {
	d := &DestructibleIncomingData{
		data: data,
	}
	d.destructCount.Store(int64(destructCount))

	return d
}

// ShardedData return hashdex.
func (d *DestructibleIncomingData) ShardedData() cppbridge.ShardedData {
	return d.data.ShardedData()
}

// Destroy decrement count or destroy IncomingData.
func (d *DestructibleIncomingData) Destroy() {
	if d.destructCount.Add(-1) != 0 {
		return
	}

	d.data.Destroy()
}
