package appender

import (
	"sync/atomic"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage"
)

//
// ShardedInnerSeries
//

// ShardedInnerSeries conteiner for InnerSeries for each shard.
type ShardedInnerSeries struct {
	// id slice - shard id, data[shard_id] - amount of data = x2 numberOfShards
	data [][]*cppbridge.InnerSeries
}

// NewShardedInnerSeries init new ShardedInnerSeries.
func NewShardedInnerSeries(numberOfShards uint16) *ShardedInnerSeries {
	// id slice - shard id
	data := make([][]*cppbridge.InnerSeries, numberOfShards)
	for i := range data {
		// amount of data = x2 numberOfShards
		data[i] = cppbridge.NewShardsInnerSeries(numberOfShards)
	}

	return &ShardedInnerSeries{
		data: data,
	}
}

// Data return slice of elemets for each shard.
func (sis *ShardedInnerSeries) Data() [][]*cppbridge.InnerSeries {
	return sis.data
}

// DataByShard return slice with the results per shard.
func (sis *ShardedInnerSeries) DataByShard(shardID uint16) []*cppbridge.InnerSeries {
	return sis.data[shardID]
}

// DataBySourceShard return slice with the results per source shard.
func (sis *ShardedInnerSeries) DataBySourceShard(sourceShardID uint16) []*cppbridge.InnerSeries {
	data := make([]*cppbridge.InnerSeries, len(sis.data))
	for i, iss := range sis.data {
		data[i] = iss[sourceShardID]
	}

	return data
}

//
// ShardedRelabeledSeries
//

// ShardedRelabeledSeries conteiner for RelabeledSeries for each shard.
type ShardedRelabeledSeries struct {
	// id slice - shard id, data[shard_id] id slice - source shard id
	// data[shard_id][source_shard_id] - amount of data = numberOfShards
	data [][]*cppbridge.RelabeledSeries
}

// NewShardedRelabeledSeries init new ShardedRelabeledSeries.
func NewShardedRelabeledSeries(numberOfShards uint16) *ShardedRelabeledSeries {
	// id slice - shard id
	data := make([][]*cppbridge.RelabeledSeries, numberOfShards)
	for i := range data {
		// data[shard_id] id slice - source shard id
		// data[shard_id][source_shard_id] - amount of data = numberOfShards
		data[i] = cppbridge.NewShardsRelabeledSeries(numberOfShards)
	}
	return &ShardedRelabeledSeries{
		data: data,
	}
}

// DataByShard return slice with the results per shard.
func (srs *ShardedRelabeledSeries) DataByShard(shardID uint16) []*cppbridge.RelabeledSeries {
	return srs.data[shardID]
}

// DataBySourceShard return slice with the results per source shard.
func (srs *ShardedRelabeledSeries) DataBySourceShard(sourceShardID uint16) ([]*cppbridge.RelabeledSeries, bool) {
	ok := false
	data := make([]*cppbridge.RelabeledSeries, len(srs.data))
	for i, rss := range srs.data {
		data[i] = rss[sourceShardID]
		if data[i].Size() != 0 {
			ok = true
		}
	}

	return data, ok
}

// IsEmpty return false if there are no elements.
func (srs *ShardedRelabeledSeries) IsEmpty() bool {
	for _, rss := range srs.data {
		for _, rs := range rss {
			if rs.Size() != 0 {
				return false
			}
		}
	}

	return true
}

//
// ShardedStateUpdates
//

// ShardedStateUpdates conteiner for RelabelerStateUpdate for each shard.
type ShardedStateUpdates struct {
	// id slice - shard id, data[shard_id] id slice - source shard id
	// data[shard_id][source_shard_id] - amount of data = numberOfShards
	data [][]*cppbridge.RelabelerStateUpdate
}

// NewShardedStateUpdates init new ShardedStateUpdates.
func NewShardedStateUpdates(numberOfShards uint16) *ShardedStateUpdates {
	// id slice - shard id
	data := make([][]*cppbridge.RelabelerStateUpdate, numberOfShards)
	for i := range data {
		// data[shard_id] id slice - source shard id
		// data[shard_id][source_shard_id] - amount of data = numberOfShards
		data[i] = cppbridge.NewShardsRelabelerStateUpdate(numberOfShards)
	}
	return &ShardedStateUpdates{
		data: data,
	}
}

// DataByShard return slice with the results per shard.
func (sru *ShardedStateUpdates) DataByShard(shardID uint16) []*cppbridge.RelabelerStateUpdate {
	return sru.data[shardID]
}

// DataBySourceShard return slice with the results per source shard.
func (sru *ShardedStateUpdates) DataBySourceShard(sourceShardID uint16) ([]*cppbridge.RelabelerStateUpdate, bool) {
	ok := false
	data := make([]*cppbridge.RelabelerStateUpdate, len(sru.data))
	for i, rsu := range sru.data {
		data[i] = rsu[sourceShardID]
		if !data[i].IsEmpty() {
			ok = true
		}
	}

	return data, ok
}

//
// DestructibleIncomingData
//

// DestructibleIncomingData wrapeer over [storage.IncomingData] with detroy-counter.
type DestructibleIncomingData struct {
	data          *storage.IncomingData
	destructCount atomic.Int64
}

// NewDestructibleIncomingData init new [DestructibleIncomingData].
func NewDestructibleIncomingData(data *storage.IncomingData, destructCount int) *DestructibleIncomingData {
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
