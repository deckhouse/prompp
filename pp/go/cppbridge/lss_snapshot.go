package cppbridge

import (
	"runtime"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
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

// Pointer return c-pointer.
func (lsst *LabelSetSnapshot) Pointer() uintptr {
	return lsst.pointer
}

// RangeLabelSet serialize to slice labels from snapshot and calls f on each label.
func (lsst *LabelSetSnapshot) RangeLabelSet(lsID uint32, do func(l Label) error) error {
	labelSet := labelSetSerialize(lsst.pointer, lsID)
	for i := range labelSet {
		if err := do(labelSet[i]); err != nil {
			labelSetFree(labelSet)
			return err
		}
	}
	runtime.KeepAlive(lsst)
	labelSetFree(labelSet)

	return nil
}

// Query returns a LSSQueryResult that matches the given selector.
func (lss *LabelSetSnapshot) Query(selector uintptr) *LSSQueryResult {
	return newLSSQueryResult(primitivesLSSQuery(lss.pointer, selector))
}
