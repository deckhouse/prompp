package head

import (
	"context"
	"fmt"
	"sync"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
)

// TaskInputRelabeling - task for stage input relabeling.
type TaskInputRelabeling struct {
	ctx           context.Context
	promise       *InputRelabelingPromise
	incomingData  *relabeler.DestructibleIncomingData
	relabelerData *RelabelerData
	state         *cppbridge.State
}

// NewTaskInputRelabeling - init task for stage input relabeling.
func NewTaskInputRelabeling(
	ctx context.Context,
	promise *InputRelabelingPromise,
	incomingData *relabeler.DestructibleIncomingData,
	relabelerData *RelabelerData,
	state *cppbridge.State,
) *TaskInputRelabeling {
	return &TaskInputRelabeling{
		ctx:           ctx,
		promise:       promise,
		incomingData:  incomingData,
		relabelerData: relabelerData,
		state:         state,
	}
}

// AddError - add to promise error.
func (t *TaskInputRelabeling) AddError(shardID uint16, err error) {
	t.promise.AddError(shardID, err)
}

// AddResult - add to promise result.
func (t *TaskInputRelabeling) AddResult(shardID uint16, innerSeries *cppbridge.InnerSeries) {
	t.promise.AddResult(shardID, innerSeries)
}

// Ctx - return task context.
func (t *TaskInputRelabeling) Ctx() context.Context {
	return t.ctx
}

// InputRelabeler return *cppbridge.InputPerShardRelabeler by shard.
func (t *TaskInputRelabeling) InputRelabelerByShard(shardID uint16) *cppbridge.InputPerShardRelabeler {
	return t.relabelerData.InputRelabelerByShard(shardID)
}

// CacheByShard return *Cache by shard.
func (t *TaskInputRelabeling) CacheByShard(shardID uint16) *cppbridge.Cache {
	return t.state.CacheByShard(shardID)
}

// Options return options for relabeler.
func (t *TaskInputRelabeling) Options() cppbridge.RelabelerOptions {
	return t.state.RelabelerOptions()
}

// RelabelerData return RelabelerData for relabeler.
func (t *TaskInputRelabeling) RelabelerData() *RelabelerData {
	return t.relabelerData
}

// AddStats add returned relabler stats.
func (t *TaskInputRelabeling) AddStats(stats cppbridge.RelabelerStats) {
	t.promise.AddStats(stats)
}

// State return state for relabeler.
func (t *TaskInputRelabeling) State() *cppbridge.State {
	return t.state
}

// ShardedData - return ShardedData.
func (t *TaskInputRelabeling) ShardedData() cppbridge.ShardedData {
	return t.incomingData.Data().ShardedData()
}

// StaleNansStateByShard return state for stalenans for shard.
func (t *TaskInputRelabeling) StaleNansStateByShard(shardID uint16) *cppbridge.StaleNansState {
	return t.state.StaleNansStateByShard(shardID)
}

// WithStaleNans check task for stalenans states.
func (t *TaskInputRelabeling) WithStaleNans() bool {
	return t.state.TrackStaleness()
}

// DefTimestamp return timestamp for scrape time and stalenan.
func (t *TaskInputRelabeling) DefTimestamp() int64 {
	return t.state.DefTimestamp()
}

// IncomingDataDestroy increment or destroy IncomingData.
func (t *TaskInputRelabeling) IncomingDataDestroy() {
	t.incomingData.Destroy()
}

// Promise - return *IncomingRelabelingPromise.
func (t *TaskInputRelabeling) Promise() *InputRelabelingPromise {
	return t.promise
}

// Run task.
func (t *TaskInputRelabeling) Run(
	lss *LSS,
	stageAppendRelabelerSeries []chan *TaskAppendRelabelerSeries,
	shardID, numberOfShards uint16,
) {
	shardsInnerSeries := cppbridge.NewShardsInnerSeries(numberOfShards)
	shardsRelabeledSeries := cppbridge.NewShardsRelabeledSeries(numberOfShards)

	var (
		err   error
		stats cppbridge.RelabelerStats
	)

	if t.state.TrackStaleness() {
		stats, err = t.relabelerData.InputRelabelerByShard(shardID).InputRelabelingWithStalenans(
			t.ctx,
			lss.input,
			lss.target,
			t.state.CacheByShard(shardID),
			t.state.RelabelerOptions(),
			t.state.StaleNansStateByShard(shardID),
			t.state.DefTimestamp(),
			t.incomingData.Data().ShardedData(),
			shardsInnerSeries,
			shardsRelabeledSeries,
		)
	} else {
		stats, err = t.relabelerData.InputRelabelerByShard(shardID).InputRelabeling(
			t.ctx,
			lss.input,
			lss.target,
			t.state.CacheByShard(shardID),
			t.state.RelabelerOptions(),
			t.incomingData.Data().ShardedData(),
			shardsInnerSeries,
			shardsRelabeledSeries,
		)
	}

	t.incomingData.Destroy()
	if err != nil {
		t.promise.AddError(shardID, fmt.Errorf("failed input relabeling shard %d: %w", shardID, err))
		return
	}

	t.promise.AddStats(stats)
	for sid, relabeledSeries := range shardsRelabeledSeries {
		if relabeledSeries.Size() == 0 {
			t.promise.AddResult(uint16(sid), nil)
			continue
		}

		stageAppendRelabelerSeries[sid] <- NewTaskAppendRelabelerSeries(
			t.ctx,
			relabeledSeries,
			t.promise,
			t.relabelerData,
			t.state,
			shardID,
		)
	}

	for sid, innerSeries := range shardsInnerSeries {
		t.promise.AddResult(uint16(sid), innerSeries)
	}
}

// TaskAppendRelabelerSeries - task for stage add to the lss required shard.
type TaskAppendRelabelerSeries struct {
	ctx             context.Context
	relabeledSeries *cppbridge.RelabeledSeries
	promise         *InputRelabelingPromise
	relabelerData   *RelabelerData
	state           *cppbridge.State
	sourceShardID   uint16
}

// NewTaskAppendRelabelerSeries - init task stage for append relabeler series.
func NewTaskAppendRelabelerSeries(
	ctx context.Context,
	relabeledSeries *cppbridge.RelabeledSeries,
	promise *InputRelabelingPromise,
	relabelerData *RelabelerData,
	state *cppbridge.State,
	sourceShardID uint16,
) *TaskAppendRelabelerSeries {
	return &TaskAppendRelabelerSeries{
		ctx:             ctx,
		relabeledSeries: relabeledSeries,
		promise:         promise,
		relabelerData:   relabelerData,
		state:           state,
		sourceShardID:   sourceShardID,
	}
}

// AddError - add to promise error.
func (t *TaskAppendRelabelerSeries) AddError(shardID uint16, err error) {
	t.promise.AddError(shardID, err)
}

// AddResult - add to promise result.
func (t *TaskAppendRelabelerSeries) AddResult(shardID uint16, innerSeries *cppbridge.InnerSeries) {
	t.promise.AddResult(shardID, innerSeries)
}

// Ctx - return task context.
func (t *TaskAppendRelabelerSeries) Ctx() context.Context {
	return t.ctx
}

// CacheByShard return *Cache by shard.
func (t *TaskAppendRelabelerSeries) CacheByShard(shardID uint16) *cppbridge.Cache {
	return t.state.CacheByShard(shardID)
}

// Promise - return IncomingRelabelingPromise.
func (t *TaskAppendRelabelerSeries) Promise() *InputRelabelingPromise {
	return t.promise
}

// InputRelabeler return *cppbridge.InputPerShardRelabeler by shard.
func (t *TaskAppendRelabelerSeries) InputRelabelerByShard(shardID uint16) *cppbridge.InputPerShardRelabeler {
	return t.relabelerData.InputRelabelerByShard(shardID)
}

// RelabeledSeries - return *RelabeledSeries.
func (t *TaskAppendRelabelerSeries) RelabeledSeries() *cppbridge.RelabeledSeries {
	return t.relabeledSeries
}

// RelabelerData return RelabelerData for relabeler.
func (t *TaskAppendRelabelerSeries) RelabelerData() *RelabelerData {
	return t.relabelerData
}

// State return state for relabeler.
func (t *TaskAppendRelabelerSeries) State() *cppbridge.State {
	return t.state
}

// SourceShardID - return source shardID.
func (t *TaskAppendRelabelerSeries) SourceShardID() uint16 {
	return t.sourceShardID
}

// Run task.
func (t *TaskAppendRelabelerSeries) Run(
	target *cppbridge.LabelSetStorage,
	stageUpdateRelabelers []chan *TaskUpdateRelabelerState,
	shardID uint16,
) {
	relabelerStateUpdate := cppbridge.NewRelabelerStateUpdate()
	innerSeries := cppbridge.NewInnerSeries()

	if err := t.InputRelabelerByShard(shardID).AppendRelabelerSeries(
		t.ctx,
		target,
		relabelerStateUpdate,
		innerSeries,
		t.relabeledSeries,
	); err != nil {
		t.promise.AddError(shardID, fmt.Errorf("failed input append relabeler series shard %d: %w", shardID, err))
		return
	}

	stageUpdateRelabelers[t.sourceShardID] <- NewTaskUpdateRelabelerState(
		t.ctx,
		t.promise,
		relabelerStateUpdate,
		t.relabelerData.InputRelabelerByShard(t.sourceShardID),
		t.state.CacheByShard(t.sourceShardID),
		shardID,
	)

	t.promise.AddResult(shardID, innerSeries)
}

// TaskUpdateRelabelerState - task for stage updates the cache in the source shard with the relabeled shard.
type TaskUpdateRelabelerState struct {
	ctx                  context.Context
	promise              *InputRelabelingPromise
	relabelerStateUpdate *cppbridge.RelabelerStateUpdate
	inputRelabeler       *cppbridge.InputPerShardRelabeler
	cache                *cppbridge.Cache
	relabeledShardID     uint16
}

// NewTaskUpdateRelabelerState - init task for stage updates the cache in the source shard with the relabeled shard.
func NewTaskUpdateRelabelerState(
	ctx context.Context,
	promise *InputRelabelingPromise,
	relabelerStateUpdate *cppbridge.RelabelerStateUpdate,
	inputRelabeler *cppbridge.InputPerShardRelabeler,
	cache *cppbridge.Cache,
	relabeledShardID uint16,
) *TaskUpdateRelabelerState {
	return &TaskUpdateRelabelerState{
		ctx:                  ctx,
		promise:              promise,
		relabelerStateUpdate: relabelerStateUpdate,
		inputRelabeler:       inputRelabeler,
		cache:                cache,
		relabeledShardID:     relabeledShardID,
	}
}

// AddError - add to promise error.
func (t *TaskUpdateRelabelerState) AddError(shardID uint16, err error) {
	t.promise.AddError(shardID, err)
}

// Update run update relabeler state.
func (t *TaskUpdateRelabelerState) Update() error {
	return t.inputRelabeler.UpdateRelabelerState(
		t.ctx,
		t.cache,
		t.relabelerStateUpdate,
		t.relabeledShardID,
	)
}

// GenericTask - generic task, will be executed on each shard.
type GenericTask struct {
	errs    []error
	shardFn relabeler.ShardFn
	wg      *sync.WaitGroup
}

func (t *GenericTask) Wait() {
	t.wg.Wait()
}

func (t *GenericTask) Errors() []error {
	return t.errs
}

func (t *GenericTask) ExecuteOnShard(shard relabeler.Shard) {
	t.errs[shard.ShardID()] = t.shardFn(shard)
	t.wg.Done()
}

// NewGenericTask - constructor.
func NewGenericTask(shardFn relabeler.ShardFn, numberOfShards uint16) *GenericTask {
	errs := make([]error, numberOfShards)
	wg := &sync.WaitGroup{}
	wg.Add(int(numberOfShards))
	return &GenericTask{errs: errs, shardFn: shardFn, wg: wg}
}

// NewSingleGenericTask - constructor.
func NewSingleGenericTask(shardFn relabeler.ShardFn, numberOfShards uint16) *GenericTask {
	errs := make([]error, numberOfShards)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	return &GenericTask{errs: errs, shardFn: shardFn, wg: wg}
}

//
// GenericReadTask
//

type GenericReadTask struct {
	errs    []error
	shardFn relabeler.ShardFn
	wg      *sync.WaitGroup
}

// NewGenericReadTask init new GenericReadTask.
func NewGenericReadTask(shardFn relabeler.ShardFn, numberOfShards uint16) *GenericReadTask {
	errs := make([]error, numberOfShards)
	wg := &sync.WaitGroup{}
	wg.Add(int(numberOfShards))
	return &GenericReadTask{errs: errs, shardFn: shardFn, wg: wg}
}

func (t *GenericReadTask) Wait() {
	t.wg.Wait()
}

func (t *GenericReadTask) Errors() []error {
	return t.errs
}

func (t *GenericReadTask) ExecuteOnShard(shard relabeler.Shard) {
	t.errs[shard.ShardID()] = t.shardFn(shard)
	t.wg.Done()
}
