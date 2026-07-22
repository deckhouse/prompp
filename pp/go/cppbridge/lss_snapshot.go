package cppbridge

import (
	"runtime"
	"unsafe"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/util"
)

var (
	// Working snapshot.
	snapshotCreateWorking = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_snapshot_create_count",
			Help:        "Current number of created snapshots.",
			ConstLabels: prometheus.Labels{"snapshot_type": "working"},
		},
	)
	snapshotFinalizeWorking = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_snapshot_finalize_count",
			Help:        "Current number of finalized snapshots.",
			ConstLabels: prometheus.Labels{"snapshot_type": "working"},
		},
	)

	// Transition snapshot.
	snapshotCreateTransition = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_snapshot_create_count",
			Help:        "Current number of created snapshots.",
			ConstLabels: prometheus.Labels{"snapshot_type": "transition"},
		},
	)
	snapshotFinalizeTransition = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_snapshot_finalize_count",
			Help:        "Current number of finalized snapshots.",
			ConstLabels: prometheus.Labels{"snapshot_type": "transition"},
		},
	)
	// Remote write snapshot.
	snapshotCreateRemoteWrite = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_snapshot_create_count",
			Help:        "Current number of created snapshots.",
			ConstLabels: prometheus.Labels{"snapshot_type": "remote_write"},
		},
	)
	snapshotFinalizeRemoteWrite = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_snapshot_finalize_count",
			Help:        "Current number of finalized snapshots.",
			ConstLabels: prometheus.Labels{"snapshot_type": "remote_write"},
		},
	)
	// Rotation snapshot.
	snapshotCreateRotation = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_snapshot_create_count",
			Help:        "Current number of created snapshots.",
			ConstLabels: prometheus.Labels{"snapshot_type": "rotation"},
		},
	)
	snapshotFinalizeRotation = util.NewUnconflictRegisterer(prometheus.DefaultRegisterer).NewCounter(
		prometheus.CounterOpts{
			Name:        "prompp_cppbridge_snapshot_finalize_count",
			Help:        "Current number of finalized snapshots.",
			ConstLabels: prometheus.Labels{"snapshot_type": "rotation"},
		},
	)
)

// gcDestroyDetector for field for the GC to destroy the structure.
var gcDestroyDetector uint64

//
// SnapshotType
//

// SnapshotType is the type of snapshot.
type SnapshotType uint64

// IncCreate increment the create counter for the snapshot type.
func (t SnapshotType) IncCreate() {
	switch t {
	case SnapshotTypeWorking:
		snapshotCreateWorking.Inc()
	case SnapshotTypeTransition:
		snapshotCreateTransition.Inc()
	case SnapshotTypeRemoteWrite:
		snapshotCreateRemoteWrite.Inc()
	case SnapshotTypeRotation:
		snapshotCreateRotation.Inc()
	}
}

// IncFinalize increment the finalize counter for the snapshot type.
func (t SnapshotType) IncFinalize() {
	switch t {
	case SnapshotTypeWorking:
		snapshotFinalizeWorking.Inc()
	case SnapshotTypeTransition:
		snapshotFinalizeTransition.Inc()
	case SnapshotTypeRemoteWrite:
		snapshotFinalizeRemoteWrite.Inc()
	case SnapshotTypeRotation:
		snapshotFinalizeRotation.Inc()
	}
}

const (
	// SnapshotTypeWorking is the snapshot type for working state.
	SnapshotTypeWorking SnapshotType = iota
	// SnapshotTypeTransition is the snapshot type for transition state.
	SnapshotTypeTransition
	// SnapshotTypeRemoteWrite is the snapshot type for remote write state.
	SnapshotTypeRemoteWrite
	// SnapshotTypeRotation is the snapshot type for rotation state.
	SnapshotTypeRotation
)

//
// LabelSetSnapshot
//

// LabelSetSnapshot go container for snapshot from LabelSetStorage.
type LabelSetSnapshot struct {
	pointer      uintptr
	snapshotType SnapshotType
}

// newLabelSetSnapshot init new LabelSetSnapshot.
func newLabelSetSnapshot(snapshotPtr uintptr, snapshotType SnapshotType) *LabelSetSnapshot {
	lsst := &LabelSetSnapshot{pointer: snapshotPtr, snapshotType: snapshotType}
	runtime.SetFinalizer(lsst, func(l *LabelSetSnapshot) {
		primitivesSnapshotDtor(l.pointer)

		l.snapshotType.IncFinalize()
	})

	snapshotType.IncCreate()

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

// GroupSeriesByLabelNames group series by label names
func (lss *LabelSetSnapshot) GroupSeriesByLabelNames(seriesIDs []uint32, labelNameIDs []uint32) *SeriesGroups {
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
		primitivesLabelSetMatchesFree(lqr)

		return lqr
	}

	runtime.SetFinalizer(lqr, func(result *LSSQueryResult) {
		primitivesLabelSetMatchesFree(result)
	})

	return lqr
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
