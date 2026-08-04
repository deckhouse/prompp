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

	lsQueryResultCreate = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "prompp_cppbridge_ls_query_result_create_count",
			Help: "Current number of created LSSQueryResult.",
		},
	)

	lsQueryResultFinalize = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "prompp_cppbridge_ls_query_result_finalize_count",
			Help: "Current number of finalized LSSQueryResult.",
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
	runtime.AddCleanup(lsst, func(pointer uintptr) {
		primitivesSnapshotDtor(pointer)

		snapshotFinalize.Inc()
	}, snapshotPtr)

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
func (lss *LabelSetSnapshot) Query(selector uintptr) *LSSQueryResult {
	result := newLSSQueryResult(primitivesSnapshotQuery(lss.pointer, selector))
	runtime.KeepAlive(lss)
	return result
}

type SeriesGroups struct {
	Groups [][]uint32
}

// GroupSeriesByLabelNames group series by label names.
func (lss *LabelSetSnapshot) GroupSeriesByLabelNames(seriesIDs, labelNameIDs []uint32) *SeriesGroups {
	result := &SeriesGroups{
		Groups: primitivesGroupSeriesByLabelNames(lss.pointer, seriesIDs, labelNameIDs),
	}
	runtime.AddCleanup(result, primitivesGroupSeriesByLabelNamesFree, result.Groups)

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
	runtime.AddCleanup(idsMapping, primitivesFreeLsIdsMapping, idsMapping.pointer)

	runtime.KeepAlive(lss)
	runtime.KeepAlive(bitsetSeries)
	runtime.KeepAlive(destination)

	return idsMapping
}

//
// LSSQueryResult
//

// LSSQueryResult query execution result in lss with copy.
type LSSQueryResult struct {
	matches         []uint32 // c allocated
	labelSetLengths []uint16 // c allocated
	status          uint32
}

// newLSSQueryResult init new LSSQueryResult.
func newLSSQueryResult(
	matches []uint32,
	labelSetLengths []uint16,
	status uint32,
) *LSSQueryResult {
	lqr := &LSSQueryResult{
		matches:         matches,
		labelSetLengths: labelSetLengths,
		status:          status,
	}

	if status != LSSQueryStatusMatch {
		lqr.Close()

		return lqr
	}

	runtime.SetFinalizer(lqr, func(result *LSSQueryResult) {
		result.Close()
	})

	return lqr
}

// Close frees the C-allocated result buffers and cancels the finalizer.
// It is idempotent: subsequent calls (and the finalizer) are no-ops.
// After Close the result must not be read anymore.
func (r *LSSQueryResult) Close() {
	if r.matches == nil && r.labelSetLengths == nil {
		return
	}

	runtime.SetFinalizer(r, nil)
	primitivesLabelSetMatchesFree(r)
	r.matches = nil
	r.labelSetLengths = nil
}

func (r *LSSQueryResult) IndexOf(seriesID uint32) int {
	for i, match := range r.matches {
		if match == seriesID {
			return i
		}
	}
	return -1
}

func (r *LSSQueryResult) LengthBySeriesID(seriesID uint32, searchFrom int) (length uint16, index int) {
	for {
		if searchFrom > len(r.matches)-1 {
			return 0, -1
		}

		if r.matches[searchFrom] == seriesID {
			return r.labelSetLengths[searchFrom], searchFrom
		}

		searchFrom++
	}
}

// GetByIndex return ls id and length for ls id by index.
func (r *LSSQueryResult) GetByIndex(i int) (uint32, uint16) {
	return r.matches[i], r.labelSetLengths[i]
}

// IDs return labels sets ids.
func (r *LSSQueryResult) IDs() []uint32 {
	return r.matches
}

// LabelSetLengths return labels sets lengths.
func (r *LSSQueryResult) LabelSetLengths() []uint16 {
	return r.labelSetLengths
}

// Len of result.
func (r *LSSQueryResult) Len() int {
	return len(r.matches)
}

// Status query execution.
func (r *LSSQueryResult) Status() uint32 {
	return r.status
}
