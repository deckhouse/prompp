package appender

import (
	"sync/atomic"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
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
