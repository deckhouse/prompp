package cppbridge

import (
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/prometheus/pp/go/model"
)

var (
	snapshotCreate = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "prompp_cppbridge_snapshot_create_count",
			Help: "Current number of created snapshots.",
		},
	)

	snapshotFinalize = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "prompp_cppbridge_snapshot_finalize_count",
			Help: "Current number of finalized snapshots.",
		},
	)
)

// gcDestroyDetector for field for the GC to destroy the structure.
var gcDestroyDetector uint64

// SnapshotSource a source that contains and updates the snapshot itself.
type SnapshotSource interface {
	// FastSnapshot return the actual snapshot or nil if not exist.
	FastSnapshot() *LabelSetSnapshot
}

//
// LabelSetSnapshot
//

// LabelSetSnapshot go container for snapshot from LabelSetStorage.
type LabelSetSnapshot struct {
	pointer uintptr
	source  SnapshotSource
}

// newLabelSetSnapshot init new LabelSetSnapshot.
func newLabelSetSnapshot(lsstPtr uintptr, source SnapshotSource) *LabelSetSnapshot {
	lsst := &LabelSetSnapshot{
		pointer: lsstPtr,
		source:  source,
	}
	runtime.SetFinalizer(lsst, func(l *LabelSetSnapshot) {
		primitivesLSSDtor(l.pointer)

		snapshotFinalize.Inc()
	})

	snapshotCreate.Inc()

	return lsst
}

// Snapshot return the actual snapshot.
func (lsst *LabelSetSnapshot) Snapshot() *LabelSetSnapshot {
	if snapshot := lsst.source.FastSnapshot(); snapshot != nil {
		return snapshot
	}

	return lsst
}

// LabelSetBytes returns ls as a byte slice.
// It uses an byte invalid character as a separator and so should not be used for printing.
func (lsst *LabelSetSnapshot) LabelSetBytes(lsID uint32, bytes []byte, dropMetricName bool) []byte {
	bytes = labelSetBytes(lsst.pointer, lsID, bytes, dropMetricName)
	runtime.KeepAlive(lsst)
	return bytes
}

// LabelSetBytesWithLabels is just as Bytes(), but only for labels matching names.
// 'names' have to be sorted in ascending order.
func (lsst *LabelSetSnapshot) LabelSetBytesWithLabels(
	lsID uint32,
	bytes []byte,
	dropMetricName bool,
	names []string,
) []byte {
	bytes = labelSetBytesWithLabels(lsst.pointer, lsID, bytes, dropMetricName, names)
	runtime.KeepAlive(lsst)
	return bytes
}

// LabelSetBytesWithoutLabels is just as Bytes(), but only for labels not matching names.
// 'names' have to be sorted in ascending order.
func (lsst *LabelSetSnapshot) LabelSetBytesWithoutLabels(
	lsID uint32,
	bytes []byte,
	dropMetricName bool,
	names []string,
) []byte {
	bytes = labelSetBytesWithoutLabels(lsst.pointer, lsID, bytes, dropMetricName, names)
	runtime.KeepAlive(lsst)
	return bytes
}

// LabelSetGetValue returns the value for the label with the given name.
// Returns an empty string if the label doesn't exist.
func (lsst *LabelSetSnapshot) LabelSetGetValue(lsID uint32, labelName string) string {
	name := labelSetGetValue(lsst.pointer, labelName, lsID)
	runtime.KeepAlive(lsst)
	return name
}

// LabelSetHasDuplicateLabelNames returns whether ls has duplicate label names.
func (lsst *LabelSetSnapshot) LabelSetHasDuplicateLabelNames(lsID uint32, dropMetricName bool) (string, bool) {
	name, ok := labelSetHasDuplicateLabelNames(lsst.pointer, lsID, dropMetricName)
	runtime.KeepAlive(lsst)
	return name, ok
}

// LabelSetHasLabelName returns true if the label with the given name is present.
func (lsst *LabelSetSnapshot) LabelSetHasLabelName(lsID uint32, labelName string) bool {
	ok := labelSetHasLabelName(lsst.pointer, labelName, lsID)
	runtime.KeepAlive(lsst)
	return ok
}

// LabelSetHash returns a hash value for the label set.
func (lsst *LabelSetSnapshot) LabelSetHash(lsID uint32, dropMetricName bool) uint64 {
	hash := labelSetHash(lsst.pointer, lsID, dropMetricName)
	runtime.KeepAlive(lsst)
	return hash
}

// LabelSetHashForLabels returns a hash value for the labels matching the provided names.
// 'names' have to be sorted in ascending order.
func (lsst *LabelSetSnapshot) LabelSetHashForLabels(lsID uint32, labelNames []string, dropMetricName bool) uint64 {
	hash := labelSetHashForLabels(lsst.pointer, labelNames, lsID, dropMetricName)
	runtime.KeepAlive(lsst)
	return hash
}

// LabelSetHashWithoutLabels returns a hash value for all labels except those matching
// the provided names. 'names' have to be sorted in ascending order.
func (lsst *LabelSetSnapshot) LabelSetHashWithoutLabels(lsID uint32, labelNames []string) uint64 {
	hash := labelSetHashWithoutLabels(lsst.pointer, labelNames, lsID)
	runtime.KeepAlive(lsst)
	return hash
}

// LabelSetLength returns the number of labels for ls id.
func (lsst *LabelSetSnapshot) LabelSetLength(lsID uint32, dropMetricName bool) int {
	length := int(labelSetLength(lsst.pointer, lsID, dropMetricName)) // #nosec G115 // no overflow
	runtime.KeepAlive(lsst)
	return length
}

// Pointer return c-pointer.
func (lsst *LabelSetSnapshot) Pointer() uintptr {
	return lsst.pointer
}

// RangeLabelSet serialize to slice labels from snapshot and calls f on each label.
func (lsst *LabelSetSnapshot) RangeLabelSet(lsID uint32, dropMetricName bool, do func(l Label) error) error {
	labelSet := labelSetSerialize(lsst.pointer, lsID, dropMetricName)

	for i := range labelSet {
		if err := do(labelSet[i]); err != nil {
			labelSetFree(labelSet)
			return err
		}
	}

	labelSetFree(labelSet)
	runtime.KeepAlive(lsst)

	return nil
}

//
// LSSWithSnapshot
//

// LSSWithSnapshot container for LabelSetStorage with snapshot.
type LSSWithSnapshot struct {
	lss      *LabelSetStorage
	snapshot unsafe.Pointer
	once     sync.Once
}

// NewLSSWithSnapshot init new *LSSWithSnapshot.
func NewLSSWithSnapshot(lss *LabelSetStorage) *LSSWithSnapshot {
	return &LSSWithSnapshot{
		lss:  lss,
		once: sync.Once{},
	}
}

// AllocatedMemory return size of allocated memory LabelSetStorage.
func (lws *LSSWithSnapshot) AllocatedMemory() uint64 {
	return lws.lss.AllocatedMemory()
}

// FastSnapshot return the actual snapshot or nil if not exist.
func (lws *LSSWithSnapshot) FastSnapshot() *LabelSetSnapshot {
	return (*LabelSetSnapshot)(atomic.LoadPointer(&lws.snapshot))
}

// FindOrEmplace find in lss LabelSet or emplace and return ls id.
func (lws *LSSWithSnapshot) FindOrEmplace(labelSet model.LabelSet) uint32 {
	res := lws.lss.FindOrEmplace(labelSet)
	if res.LssHasReallocations {
		atomic.StorePointer(
			&lws.snapshot,
			unsafe.Pointer(lws.lss.CreateLabelSetSnapshot(lws)), // #nosec G103 // it's meant to be that way
		)
	}

	return res.LabelSetID
}

// FindOrEmplaceFromBuilder find in lss LabelSet from builder or emplace and
// return LabelSetSnapshot if there was a reallocation and ls id.
func (lws *LSSWithSnapshot) FindOrEmplaceFromBuilder(
	sortedAdd []Label,
	sortedDel []string,
	otherSnapshot *LabelSetSnapshot,
	lsID uint32,
) (length uint64, newlsID uint32) {
	var snapshotPointer uintptr
	if otherSnapshot != nil {
		snapshotPointer = otherSnapshot.pointer
	}

	lssROPtr, length, newlsID, hasReallocations := primitivesLSSFindOrEmplaceFromBuilder(
		lws.lss.pointer,
		snapshotPointer,
		sortedAdd,
		sortedDel,
		lsID,
	)
	runtime.KeepAlive(lws)
	runtime.KeepAlive(otherSnapshot)

	if hasReallocations {
		atomic.StorePointer(
			&lws.snapshot,
			unsafe.Pointer(newLabelSetSnapshot(lssROPtr, lws)), // #nosec G103 // it's meant to be that way
		)
	}

	return length, newlsID
}

// LSS return raw *LabelSetStorage.
func (lws *LSSWithSnapshot) LSS() *LabelSetStorage {
	return lws.lss
}

// ResetSnapshot resets the current snapshot.
func (lws *LSSWithSnapshot) ResetSnapshot() {
	lws.snapshot = nil
	lws.once = sync.Once{}
}

// Snapshot return the actual snapshot.
func (lws *LSSWithSnapshot) Snapshot() *LabelSetSnapshot {
	lws.once.Do(func() {
		atomic.StorePointer(
			&lws.snapshot,
			unsafe.Pointer(lws.lss.CreateLabelSetSnapshot(lws)), // #nosec G103 // it's meant to be that way
		)
	})

	return lws.FastSnapshot()
}

//
// CppLabelSetBuilder
//

// CppLabelSetBuilder - container used for Go-C++ interaction and shouldn't be modified.
type CppLabelSetBuilder struct {
	ReadonlyLss uintptr
	LsId        uint32
	SortedAdd   []Label
	SortedDel   []string
}
