package shard

import (
	"sync"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
)

// LSS labelset storage for [shard].
type LSS struct {
	input    *cppbridge.LabelSetStorage
	target   *cppbridge.LabelSetStorage
	snapshot *cppbridge.LabelSetSnapshot
	once     sync.Once
}

func (w *LSS) Raw() *cppbridge.LabelSetStorage {
	return w.target
}

func (w *LSS) AllocatedMemory() uint64 {
	return w.input.AllocatedMemory() + w.target.AllocatedMemory()
}

func (w *LSS) QueryLabelValues(
	label_name string,
	matchers []model.LabelMatcher,
) *cppbridge.LSSQueryLabelValuesResult {
	return w.target.QueryLabelValues(label_name, matchers)
}

func (w *LSS) QueryLabelNames(matchers []model.LabelMatcher) *cppbridge.LSSQueryLabelNamesResult {
	return w.target.QueryLabelNames(matchers)
}

// QuerySelector returns a created selector that matches the given label matchers.
func (w *LSS) QuerySelector(matchers []model.LabelMatcher) (selector uintptr, status uint32) {
	return w.target.QuerySelector(matchers)
}

func (w *LSS) GetLabelSets(labelSetIDs []uint32) *cppbridge.LabelSetStorageGetLabelSetsResult {
	return w.target.GetLabelSets(labelSetIDs)
}

// GetSnapshot return the actual snapshot.
func (w *LSS) GetSnapshot() *cppbridge.LabelSetSnapshot {
	w.once.Do(func() {
		w.snapshot = w.target.CreateLabelSetSnapshot()
	})

	return w.snapshot
}

// ResetSnapshot resets the current snapshot.
func (w *LSS) ResetSnapshot() {
	w.snapshot = nil
	w.once = sync.Once{}
}

func (w *LSS) Input() *cppbridge.LabelSetStorage {
	return w.input
}

func (w *LSS) Target() *cppbridge.LabelSetStorage {
	return w.target
}
