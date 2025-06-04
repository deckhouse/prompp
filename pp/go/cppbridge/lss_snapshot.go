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
func (lsst *LabelSetSnapshot) LabelSetBytes(lsID uint32, bytes *[]byte, dropMetricName bool) []byte {
	return labelSetBytes(lsst.pointer, lsID, *bytes, dropMetricName)
}

// LabelSetBytesWithLabels is just as Bytes(), but only for labels matching names.
// 'names' have to be sorted in ascending order.
func (lsst *LabelSetSnapshot) LabelSetBytesWithLabels(
	lsID uint32,
	bytes *[]byte,
	dropMetricName bool,
	names []string,
) []byte {
	return labelSetBytesWithLabels(lsst.pointer, lsID, *bytes, dropMetricName, names)
}

// LabelSetBytesWithoutLabels is just as Bytes(), but only for labels not matching names.
// 'names' have to be sorted in ascending order.
func (lsst *LabelSetSnapshot) LabelSetBytesWithoutLabels(
	lsID uint32,
	bytes *[]byte,
	dropMetricName bool,
	names []string,
) []byte {
	return labelSetBytesWithoutLabels(lsst.pointer, lsID, *bytes, dropMetricName, names)
}

// LabelSetGetValue returns the value for the label with the given name.
// Returns an empty string if the label doesn't exist.
func (lsst *LabelSetSnapshot) LabelSetGetValue(lsID uint32, labelName string) string {
	return labelSetGetValue(lsst.pointer, labelName, lsID)
}

// LabelSetHasDuplicateLabelNames returns whether ls has duplicate label names.
func (lsst *LabelSetSnapshot) LabelSetHasDuplicateLabelNames(lsID uint32, dropMetricName bool) (string, bool) {
	return labelSetHasDuplicateLabelNames(lsst.pointer, lsID, dropMetricName)
}

// LabelSetHasLabelName returns true if the label with the given name is present.
func (lsst *LabelSetSnapshot) LabelSetHasLabelName(lsID uint32, labelName string) bool {
	return labelSetHasLabelName(lsst.pointer, labelName, lsID)
}

// LabelSetHash returns a hash value for the label set.
func (lsst *LabelSetSnapshot) LabelSetHash(lsID uint32, dropMetricName bool) uint64 {
	return labelSetHash(lsst.pointer, lsID, dropMetricName)
}

// LabelSetHashForLabels returns a hash value for the labels matching the provided names.
// 'names' have to be sorted in ascending order.
func (lsst *LabelSetSnapshot) LabelSetHashForLabels(lsID uint32, labelNames []string, dropMetricName bool) uint64 {
	return labelSetHashForLabels(lsst.pointer, labelNames, lsID, dropMetricName)
}

// LabelSetHashWithoutLabels returns a hash value for all labels except those matching
// the provided names. 'names' have to be sorted in ascending order.
func (lsst *LabelSetSnapshot) LabelSetHashWithoutLabels(lsID uint32, labelNames []string) uint64 {
	return labelSetHashWithoutLabels(lsst.pointer, labelNames, lsID)
}

// LabelSetLength returns the number of labels for ls id.
func (lsst *LabelSetSnapshot) LabelSetLength(lsID uint32, dropMetricName bool) int {
	return int(labelSetLength(lsst.pointer, lsID, dropMetricName)) // #nosec G115 // no overflow
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

	return nil
}
