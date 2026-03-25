package relabeler

import (
	"context"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
)

type LSS interface {
	Raw() *cppbridge.LabelSetStorage
	AllocatedMemory() uint64
	QueryLabelValues(labelName string, matchers []model.LabelMatcher) *cppbridge.LSSQueryLabelValuesResult
	QueryLabelNames(matchers []model.LabelMatcher) *cppbridge.LSSQueryLabelNamesResult
	QuerySelector(matchers []model.LabelMatcher) (selector uintptr, status uint32)
	GetLabelSets(labelSetIDs []uint32) *cppbridge.LabelSetStorageGetLabelSetsResult
	GetSnapshot() *cppbridge.LabelSetSnapshot
	ResetSnapshot()
	Input() *cppbridge.LabelSetStorage
	Target() *cppbridge.LabelSetStorage
}

// Shard interface.
type Shard interface {
	ShardID() uint16
	LSS() LSS
	// lock for LSS
	LSSLock()
	LSSUnlock()
}

// ShardFn - shard function.
type ShardFn func(shard Shard) error

type Head interface {
	NumberOfShards() uint16
	Enqueue(t *GenericTask)
	CreateTask(taskName string, fn ShardFn, isLss bool) *GenericTask
}

type Distributor interface {
	Send(ctx context.Context, head Head, shardedData [][]*cppbridge.InnerSeries) error
	// DestinationGroups - workaround.
	DestinationGroups() DestinationGroups
	// SetDestinationGroups - workaround.
	SetDestinationGroups(destinationGroups DestinationGroups)
	WriteMetrics(head Head)
	Rotate() error
}
