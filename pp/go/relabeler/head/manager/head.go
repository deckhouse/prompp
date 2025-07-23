package manager

import (
	"context"
	"errors"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/config"
)

type DiscardableRotatableHead struct {
	head       relabeler.Head
	onRotate   func(id string, err error) error
	onDiscard  func(id string) error
	afterClose func(id string) error
}

func NewDiscardableRotatableHead(head relabeler.Head, onRotate func(id string, err error) error, onDiscard func(id string) error, afterClose func(id string) error) *DiscardableRotatableHead {
	return &DiscardableRotatableHead{
		head:       head,
		onRotate:   onRotate,
		onDiscard:  onDiscard,
		afterClose: afterClose,
	}
}

func (h *DiscardableRotatableHead) ID() string {
	return h.head.ID()
}

func (h *DiscardableRotatableHead) Generation() uint64 {
	return h.head.Generation()
}

// String serialize as string.
func (h *DiscardableRotatableHead) String() string {
	return h.head.String()
}

func (h *DiscardableRotatableHead) Append(
	ctx context.Context,
	incomingData *relabeler.IncomingData,
	state *cppbridge.State,
	relabelerID string,
	commitToWal bool,
) ([][]*cppbridge.InnerSeries, cppbridge.RelabelerStats, error) {
	return h.head.Append(ctx, incomingData, state, relabelerID, commitToWal)
}

func (h *DiscardableRotatableHead) CommitToWal() error {
	return h.head.CommitToWal()
}

// MergeOutOfOrderChunks merge chunks with out of order data chunks.
func (h *DiscardableRotatableHead) MergeOutOfOrderChunks() {
	h.head.MergeOutOfOrderChunks()
}

func (h *DiscardableRotatableHead) NumberOfShards() uint16 {
	return h.head.NumberOfShards()
}

func (h *DiscardableRotatableHead) Stop() {
	h.head.Stop()
}

func (h *DiscardableRotatableHead) Flush() error {
	return h.head.Flush()
}

func (h *DiscardableRotatableHead) Reconfigure(ctx context.Context, inputRelabelerConfigs []*config.InputRelabelerConfig, numberOfShards uint16) error {
	return h.head.Reconfigure(ctx, inputRelabelerConfigs, numberOfShards)
}

func (h *DiscardableRotatableHead) WriteMetrics(ctx context.Context) {
	h.head.WriteMetrics(ctx)
}

func (h *DiscardableRotatableHead) Status(limit int) relabeler.HeadStatus {
	return h.head.Status(limit)
}

func (h *DiscardableRotatableHead) Rotate() error {
	err := h.head.Rotate()
	if h.onRotate != nil {
		err = errors.Join(err, h.onRotate(h.ID(), err))
		h.onRotate = nil
	}
	return err
}

func (h *DiscardableRotatableHead) Close() error {
	err := h.head.Close()
	if h.afterClose != nil {
		err = errors.Join(err, h.afterClose(h.ID()))
	}
	return err
}

func (h *DiscardableRotatableHead) Discard() (err error) {
	err = h.head.Discard()
	if h.onDiscard != nil {
		err = errors.Join(err, h.onDiscard(h.ID()))
		h.onDiscard = nil
	}
	return err
}

// CopySeriesFrom copy series from other head.
func (h *DiscardableRotatableHead) CopySeriesFrom(other relabeler.Head) {
	h.head.CopySeriesFrom(other)
}

// CreateTask create a task for operations on the head shards.
func (h *DiscardableRotatableHead) CreateTask(
	taskName string,
	fn relabeler.ShardFn,
	onLss bool,
) *relabeler.GenericTask {
	return h.head.CreateTask(taskName, fn, onLss)
}

// Enqueue the task to be executed on head.
func (h *DiscardableRotatableHead) Enqueue(t *relabeler.GenericTask) {
	h.head.Enqueue(t)
}

// EnqueueOnShard the task to be executed on head on specific shard.
func (h *DiscardableRotatableHead) EnqueueOnShard(t *relabeler.GenericTask, shardID uint16) {
	h.head.EnqueueOnShard(t, shardID)
}

// Concurrency return current head workers concurrency.
func (h *DiscardableRotatableHead) Concurrency() int64 {
	return h.head.Concurrency()
}

// RLockQuery locks for query to [Head].
func (h *DiscardableRotatableHead) RLockQuery(ctx context.Context) (runlock func(), err error) {
	return h.head.RLockQuery(ctx)
}

// FindFromBuilder label set from builder in lss, if not found return EmptyLabels.
func (h *DiscardableRotatableHead) FindFromBuilder(
	builderSortedAdd []cppbridge.Label,
	builderSortedDel []string,
	builderSnapshot *cppbridge.LabelSetSnapshot,
	hash uint64,
	builderLSID uint32,
	skipCache bool,
) (labels.Labels, bool) {
	return h.head.FindFromBuilder(builderSortedAdd, builderSortedDel, builderSnapshot, hash, builderLSID, skipCache)
}

// FindByHash label set by hash in cache.
func (h *DiscardableRotatableHead) FindByHash(
	hash uint64,
	builderSortedAdd []cppbridge.Label,
	builderSortedDel []string,
	builderSnapshot *cppbridge.LabelSetSnapshot,
	builderLSID uint32,
) (labels.Labels, bool) {
	return h.head.FindByHash(
		hash,
		builderSortedAdd,
		builderSortedDel,
		builderSnapshot,
		builderLSID,
	)
}
