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
	lssCreate = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "prompp_cppbridge_lss_create_count",
			Help: "Current number of created snapshots.",
		},
		[]string{"type"},
	)

	lssFinalize = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "prompp_cppbridge_lss_finalize_count",
			Help: "Current number of finalized snapshots.",
		},
		[]string{"type"},
	)
)

// gcDestroyDetector for field for the GC to destroy the structure.
var gcDestroyDetector uint64

// SnapshotSource a source that contains and updates the snapshot itself.
type SnapshotSource interface {
	// FastSnapshot return the actual snapshot or nil if not exist.
	FastSnapshot() *LabelSetSnapshot
	// IsOutdated return true if source is outdated.
	IsOutdated() bool
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

		lssFinalize.With(prometheus.Labels{"type": "snapshot"}).Inc()
	})

	ls := prometheus.Labels{"type": "snapshot"}
	lssFinalize.With(ls).Add(0)
	lssCreate.With(ls).Inc()

	return lsst
}

// IsOutdated return true if source of snapshot is outdated.
func (lsst *LabelSetSnapshot) IsOutdated() bool {
	return lsst.source.IsOutdated()
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

// Query returns a LSSQueryResult that matches the given selector.
func (lsst *LabelSetSnapshot) Query(selector uintptr) *LSSQueryResult {
	result := newLSSQueryResult(primitivesLSSQuery(lsst.pointer, selector))
	runtime.KeepAlive(lsst)
	return result
}

// CopyAddedSeries copy the label sets from the source lss to the destination lss
// that were added source lss.
func (lsst *LabelSetSnapshot) CopyAddedSeries(bitsetSeries *BitsetSeries, destination *LabelSetStorage) *IdsMapping {
	idsMapping := &IdsMapping{
		pointer: primitivesReadonlyLSSCopyAddedSeries(lsst.pointer, bitsetSeries.pointer, destination.pointer),
	}
	runtime.SetFinalizer(idsMapping, func(idsMapping *IdsMapping) {
		primitivesFreeLsIdsMapping(idsMapping.pointer)
	})

	runtime.KeepAlive(lsst)
	runtime.KeepAlive(bitsetSeries)
	runtime.KeepAlive(destination)

	return idsMapping
}

//
// IdsMapping
//

// IdsMapping wrapper for c-pointer to ls ids mapping.
type IdsMapping struct {
	pointer uintptr
}

// IsEmpty return true if ids mapping is empty.
func (m *IdsMapping) IsEmpty() bool {
	return m.pointer == uintptr(0)
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

// IndexOf return index of series id in matches.
func (r *LSSQueryResult) IndexOf(seriesID uint32) int {
	for i, match := range r.matches {
		if match == seriesID {
			return i
		}
	}
	return -1
}

// LengthBySeriesID return length of series id in matches.
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
func (r *LSSQueryResult) GetByIndex(i int) (lsID uint32, length uint16) {
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

// Snapshot return the actual snapshot.
func (lsst *LabelSetSnapshot) Snapshot() *LabelSetSnapshot {
	if lsst.source.IsOutdated() {
		return lsst
	}

	if snapshot := lsst.source.FastSnapshot(); snapshot != nil {
		return snapshot
	}

	return lsst
}

// LabelSetEqualWithBuilder returns whether the label set and the label set from builder are equal.
func (lsst *LabelSetSnapshot) LabelSetEqualWithBuilder(
	builderSortedAdd []Label,
	builderSortedDel []string,
	builderSnapshot *LabelSetSnapshot,
	builderLSID, lsID uint32,
) bool {
	var builderSnapshotPointer uintptr
	if builderSnapshot != nil {
		builderSnapshotPointer = builderSnapshot.pointer
	}

	eq := labelSetEqualWithBuilder(
		lsst.pointer,
		builderSnapshotPointer,
		builderSortedAdd,
		builderSortedDel,
		builderLSID,
		lsID,
	)
	runtime.KeepAlive(builderSnapshot)
	runtime.KeepAlive(lsst)

	return eq
}

//
// fastSnapshot
//

// fastSnapshot pointer for snapshot.
type fastSnapshot struct {
	snapshot unsafe.Pointer
	outdated uint32
}

// FastSnapshot return the actual snapshot or nil if not exist.
func (fs *fastSnapshot) FastSnapshot() *LabelSetSnapshot {
	return (*LabelSetSnapshot)(atomic.LoadPointer(&fs.snapshot))
}

// IsOutdated return true if *LabelSetStorage is outdated.
func (fs *fastSnapshot) IsOutdated() bool {
	return atomic.LoadUint32(&fs.outdated) > 0
}

// storeSnapshot store new snapshot to fastSnapshot.
func (fs *fastSnapshot) storeSnapshot(snapshot *LabelSetSnapshot) {
	atomic.StorePointer(
		&fs.snapshot,
		unsafe.Pointer(snapshot), // #nosec G103 // it's meant to be that way
	)
}

// outdate marked *LabelSetStorage is outdated.
func (fs *fastSnapshot) outdate() {
	atomic.AddUint32(&fs.outdated, 1)
}

//
// LSSWithSnapshot
//

// LSSWithSnapshot container for LabelSetStorage with snapshot.
type LSSWithSnapshot struct {
	lss           *LabelSetStorage
	fsnapshot     *fastSnapshot
	bitsetPointer uintptr
	once          sync.Once
}

// NewLSSWithSnapshot init new *LSSWithSnapshot.
func NewLSSWithSnapshot(lss *LabelSetStorage) *LSSWithSnapshot {
	lws := &LSSWithSnapshot{
		lss:           lss,
		bitsetPointer: primitivesBitsetCtor(),
		fsnapshot:     &fastSnapshot{},
		once:          sync.Once{},
	}

	runtime.SetFinalizer(lws, func(l *LSSWithSnapshot) {
		primitivesBitsetDtor(l.bitsetPointer)
		l.fsnapshot.storeSnapshot(nil)
	})

	return lws
}

// NewLSSWithSnapshotWithoutBitset init new *LSSWithSnapshot without bitset.
func NewLSSWithSnapshotWithoutBitset(lss *LabelSetStorage) *LSSWithSnapshot {
	lws := &LSSWithSnapshot{
		lss:       lss,
		fsnapshot: &fastSnapshot{},
		once:      sync.Once{},
	}

	runtime.SetFinalizer(lws, func(l *LSSWithSnapshot) {
		l.fsnapshot.storeSnapshot(nil)
	})

	return lws
}

// FindOrEmplace find in lss LabelSet or emplace and return ls id.
func (lws *LSSWithSnapshot) FindOrEmplace(labelSet model.LabelSet) uint32 {
	res := lws.lss.FindOrEmplace(labelSet)
	runtime.KeepAlive(lws)
	if res.LssHasReallocations {
		lws.fsnapshot.storeSnapshot(lws.lss.CreateLabelSetSnapshot(lws.fsnapshot))
	}

	return res.LabelSetID
}

// FindOrEmplaceFromBuilder find in lss LabelSet from builder or emplace and
// return LabelSetSnapshot if there was a reallocation and ls id.
//
//nolint:gocritic // unnamedResult not need
func (lws *LSSWithSnapshot) FindOrEmplaceFromBuilder(
	sortedAdd []Label,
	sortedDel []string,
	otherSnapshot *LabelSetSnapshot,
	hash uint64,
	lsID uint32,
) (uint32, uint16) {
	var snapshotPointer uintptr
	if otherSnapshot != nil {
		snapshotPointer = otherSnapshot.pointer
	}

	lssROPtr, length, newlsID, hasReallocations := primitivesLSSFindOrEmplaceFromBuilder(
		lws.lss.pointer,
		snapshotPointer,
		lws.bitsetPointer,
		sortedAdd,
		sortedDel,
		hash,
		lsID,
	)
	runtime.KeepAlive(sortedAdd)
	runtime.KeepAlive(sortedDel)
	runtime.KeepAlive(otherSnapshot)
	runtime.KeepAlive(lws)

	if hasReallocations {
		lws.fsnapshot.storeSnapshot(newLabelSetSnapshot(lssROPtr, lws.fsnapshot))
	}

	return newlsID, uint16(length) // #nosec G115 // no overflow
}

// LSS return raw *LabelSetStorage.
func (lws *LSSWithSnapshot) LSS() *LabelSetStorage {
	return lws.lss
}

// Outdate marked *LabelSetStorage is outdated.
func (lws *LSSWithSnapshot) Outdate() {
	lws.fsnapshot.outdate()
}

// ResetSnapshot resets the current snapshot.
func (lws *LSSWithSnapshot) ResetSnapshot() {
	lws.fsnapshot.storeSnapshot(nil)
	lws.once = sync.Once{}
}

// Snapshot return the actual snapshot.
func (lws *LSSWithSnapshot) Snapshot() *LabelSetSnapshot {
	lws.once.Do(func() {
		lws.fsnapshot.storeSnapshot(lws.lss.CreateLabelSetSnapshot(lws.fsnapshot))
	})

	return lws.fsnapshot.FastSnapshot()
}

// Stats return allocated memory for lss, size of lss and count of emplace to bitset.
func (lws *LSSWithSnapshot) Stats() (allocatedMemory, lssSize uint64, bitsetCount uint32) {
	allocatedMemory, lssSize, bitsetCount = primitivesLSSWithSnapshotStats(lws.lss.pointer, lws.bitsetPointer, false)
	runtime.KeepAlive(lws)
	return allocatedMemory, lssSize, bitsetCount
}

// StatsWithReset return size of lss and count of emplace to bitset, clearing bitset.
func (lws *LSSWithSnapshot) StatsWithReset() (lssSize uint64, bitsetCount uint32) {
	_, lssSize, bitsetCount = primitivesLSSWithSnapshotStats(lws.lss.pointer, lws.bitsetPointer, true)
	runtime.KeepAlive(lws)
	return lssSize, bitsetCount
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
