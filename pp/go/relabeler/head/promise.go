package head

import (
	"context"
	"errors"
	"sync"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

// InputRelabelingPromise - promise for processing incoming data.
type InputRelabelingPromise struct {
	mx        *sync.Mutex
	statsMX   *sync.Mutex
	done      chan struct{}
	data      [][]*cppbridge.InnerSeries
	errors    []error
	stats     cppbridge.RelabelerStats
	shardDone uint16
}

// NewInputRelabelingPromise - init new *InputRelabelingPromise.
func NewInputRelabelingPromise(numberOfShards uint16) *InputRelabelingPromise {
	// id slice - shard id
	data := make([][]*cppbridge.InnerSeries, numberOfShards)
	for i := range data {
		// amount of data = x2 numberOfShards
		data[i] = make([]*cppbridge.InnerSeries, 0, 2*numberOfShards)
	}
	return &InputRelabelingPromise{
		data:      data,
		errors:    make([]error, numberOfShards),
		shardDone: numberOfShards,
		done:      make(chan struct{}),
		mx:        new(sync.Mutex),
		statsMX:   new(sync.Mutex),
	}
}

// AddError - add to promise error.
func (p *InputRelabelingPromise) AddError(shardID uint16, err error) {
	// error on shard
	p.mx.Lock()
	p.errors[shardID] = err
	p.shardDone--

	if p.shardDone == 0 {
		close(p.done)
	}
	p.mx.Unlock()
}

// AddResult - add to promise result.
func (p *InputRelabelingPromise) AddResult(shardID uint16, innerSeries *cppbridge.InnerSeries) {
	p.mx.Lock()
	if innerSeries != nil && innerSeries.Size() == 0 {
		innerSeries = nil
	}
	p.data[shardID] = append(p.data[shardID], innerSeries)
	if cap(p.data[shardID]) == len(p.data[shardID]) {
		p.shardDone--
	}
	if p.shardDone == 0 {
		close(p.done)
	}
	p.mx.Unlock()
}

// AddStats add returned relabler stats.
func (p *InputRelabelingPromise) AddStats(stats cppbridge.RelabelerStats) {
	p.statsMX.Lock()
	p.stats.SamplesAdded += stats.SamplesAdded
	p.stats.SeriesAdded += stats.SeriesAdded
	p.stats.SeriesDrop += stats.SeriesDrop
	p.statsMX.Unlock()
}

// Stats return relabler stats.
func (p *InputRelabelingPromise) Stats() cppbridge.RelabelerStats {
	return p.stats
}

// ShardsInnerSeries - return slice with the results of relabeling per shards.
func (p *InputRelabelingPromise) ShardsInnerSeries(shardID uint16) []*cppbridge.InnerSeries {
	return p.data[shardID]
}

// Wait - wait until all results are received.
func (p *InputRelabelingPromise) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-p.done:
		return errors.Join(p.errors...)
	}
}

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
		if data[i].Size() != 0 {
			ok = true
		}
	}

	return data, ok
}
