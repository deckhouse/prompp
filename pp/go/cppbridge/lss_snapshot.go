package cppbridge

import (
	"runtime"
	"unsafe"

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
func newLabelSetSnapshot(snapshotPtr uintptr) *LabelSetSnapshot {
	lsst := &LabelSetSnapshot{pointer: snapshotPtr, gcDestroyDetector: &gcDestroyDetector}
	runtime.SetFinalizer(lsst, func(l *LabelSetSnapshot) {
		primitivesSnapshotDtor(l.pointer)

		snapshotFinalize.Inc()
	})

	snapshotCreate.Inc()

	return lsst
}

// Pointer return c-pointer.
func (lss *LabelSetSnapshot) Pointer() uintptr {
	return lss.pointer
}

// RangeLabelSet serialize to slice labels from snapshot and calls f on each label.
func (lss *LabelSetSnapshot) RangeLabelSet(lsID uint32, do func(l Label) error) error {
	labelSet := labelSetSerializeFromSnapshot(lss.pointer, lsID)
	for i := range labelSet {
		if err := do(labelSet[i]); err != nil {
			labelSetFree(labelSet)
			return err
		}
	}
	runtime.KeepAlive(lss)
	labelSetFree(labelSet)

	return nil
}

func (lss *LabelSetSnapshot) Serialize(lsID uint32) string {
	length := labelSetSerializeFromSnapshotLength(lss.pointer, lsID)
	if length == 0 {
		return ""
	}

	buf := make([]byte, length)
	labelSetSerializeFromSnapshotToBuffer(lss.pointer, lsID, buf)
	runtime.KeepAlive(lss)
	return unsafe.String(unsafe.SliceData(buf), length)
}

// Query returns a LSSQueryResult that matches the given selector.
// With sortSeries the ids are ordered by label sets; otherwise they stay in ascending series id order.
func (lss *LabelSetSnapshot) Query(selector uintptr, sortSeries bool) *LSSQueryResult {
	result := newLSSQueryResult(primitivesSnapshotQuery(lss.pointer, selector, sortSeries))
	runtime.KeepAlive(lss)
	return result
}

// SeriesGroups group series by label names.
type SeriesGroups struct {
	Groups [][]uint32
}

// GroupSeriesByLabelNames group series by label names.
func (lss *LabelSetSnapshot) GroupSeriesByLabelNames(seriesIDs, labelNameIDs []uint32) *SeriesGroups {
	result := &SeriesGroups{
		Groups: primitivesGroupSeriesByLabelNames(lss.pointer, seriesIDs, labelNameIDs),
	}
	runtime.SetFinalizer(result, func(result *SeriesGroups) {
		primitivesGroupSeriesByLabelNamesFree(result.Groups)
	})

	runtime.KeepAlive(lss)
	return result
}

type IdsMapping struct {
	pointer           uintptr
	gcDestroyDetector *uint64
}

func (m *IdsMapping) IsEmpty() bool {
	return m.pointer == uintptr(0)
}

// CopyAddedSeries copy the label sets from the source lss to the destination lss
// that were added source lss.
func (lss *LabelSetSnapshot) CopyAddedSeries(bitsetSeries *BitsetSeries, destination *LabelSetStorage) *IdsMapping {
	idsMapping := &IdsMapping{
		pointer:           primitivesSnapshotLSSCopyAddedSeries(lss.pointer, bitsetSeries.pointer, destination.pointer),
		gcDestroyDetector: &gcDestroyDetector,
	}
	runtime.SetFinalizer(idsMapping, func(idsMapping *IdsMapping) {
		primitivesFreeLsIdsMapping(idsMapping.pointer)
	})

	runtime.KeepAlive(lss)
	runtime.KeepAlive(bitsetSeries)
	runtime.KeepAlive(destination)

	return idsMapping
}

//
// LSSQueryResult
//

// LSSQueryResult owns the native series ID buffer returned by [LabelSetSnapshot.Query].
type LSSQueryResult struct {
	matches []uint32 // C-allocated.
	status  uint32
}

// newLSSQueryResult wraps the native result, freeing the buffer immediately on non-matches.
func newLSSQueryResult(matches []uint32, status uint32) *LSSQueryResult {
	lqr := &LSSQueryResult{matches: matches, status: status}

	if status != LSSQueryStatusMatch {
		lqr.Close()

		return lqr
	}

	runtime.SetFinalizer(lqr, func(result *LSSQueryResult) {
		result.Close()
	})

	return lqr
}

// Close frees the C-allocated series IDs and cancels the finalizer.
// It is idempotent: subsequent calls (and the finalizer) are no-ops.
// After Close the result must not be read anymore.
func (r *LSSQueryResult) Close() {
	if r.matches == nil {
		return
	}

	runtime.SetFinalizer(r, nil)
	primitivesLabelSetMatchesFree(r)
	r.matches = nil
}

func (r *LSSQueryResult) IndexOf(seriesID uint32) int {
	for i, match := range r.matches {
		if match == seriesID {
			return i
		}
	}
	return -1
}

// IDs return labels sets ids.
func (r *LSSQueryResult) IDs() []uint32 {
	return r.matches
}

// Len of result.
func (r *LSSQueryResult) Len() int {
	return len(r.matches)
}

// Status query execution.
func (r *LSSQueryResult) Status() uint32 {
	return r.status
}
