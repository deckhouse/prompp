package cppbridge

import (
	"runtime"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	snapshotCreate = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "prompp_cppbridge_snapshot_create",
			Help: "Current number of created snapshots.",
		},
	)

	snapshotFinalize = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "prompp_cppbridge_snapshot_finalize",
			Help: "Current number of finalized snapshots.",
		},
	)
)

// gcDestroyDetector for field for the GC to destroy the structure.
var gcDestroyDetector uint64

//
// LabelSetSnapshot
//

// LabelSetSnapshot go container for snapshot from LabelSetStorage.
type LabelSetSnapshot struct {
	pointer           uintptr
	gcDestroyDetector *uint64 // field for the GC to destroy the structure.
}

// newLabelSetSnapshot init new LabelSetSnapshot.
func newLabelSetSnapshot(lsstPtr uintptr) *LabelSetSnapshot {
	lsst := &LabelSetSnapshot{pointer: lsstPtr, gcDestroyDetector: &gcDestroyDetector}
	runtime.SetFinalizer(lsst, func(l *LabelSetSnapshot) {
		primitivesLSSDtor(l.pointer)

		snapshotFinalize.Inc()
	})

	snapshotCreate.Inc()

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
// CppLabelSetBuilder
//

// CppLabelSetBuilder - container used for Go-C++ interaction and shouldn't be modified.
type CppLabelSetBuilder struct {
	sortedAdd []Label
	sortedDel []string
	// labels
	snapshot       *LabelSetSnapshot
	lsID           uint32
	length         uint16
	dropMetricName bool
}
