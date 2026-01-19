package cppbridge

import (
	"runtime"
	"unsafe"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/model"
)

const (
	lssEncodingBimap uint32 = iota
	lssQueryableEncodingBimap
)

// lssTypeToString serialize lss type to string.
func lssTypeToString(lssType uint32) string {
	switch lssType {
	case lssEncodingBimap:
		return "encoding_bimap"

	case lssQueryableEncodingBimap:
		return "queryable_encoding_bimap"

	default:
		return "unknown_lss_type"
	}
}

//
// LSS Query Status
//

const (
	// LSSQueryStatusNoPositiveMatchers the status when there is no positive matchers.
	LSSQueryStatusNoPositiveMatchers uint32 = iota
	// LSSQueryStatusRegexpError the status when there is a regexp error.
	LSSQueryStatusRegexpError
	// LSSQueryStatusNoMatch the status when there is no match.
	LSSQueryStatusNoMatch
	// LSSQueryStatusMatch the status when there is a match.
	LSSQueryStatusMatch
)

//
// LabelSetStorage
//

// LabelSetStorage go wrapper for C-LabelSetStorage.
type LabelSetStorage struct {
	pointer           uintptr
	gcDestroyDetector *uint64
}

// NewLssStorage init new LabelSetStorage based on EncodingBimap.
func NewLssStorage() *LabelSetStorage {
	return newLabelSetStorage(lssEncodingBimap)
}

// NewQueryableLssStorage init new LabelSetStorage based on QueryableEncodingBimap.
func NewQueryableLssStorage() *LabelSetStorage {
	return newLabelSetStorage(lssQueryableEncodingBimap)
}

// newLabelSetStorage init new LabelSetStorage with lss type.
func newLabelSetStorage(lssType uint32) *LabelSetStorage {
	return newLabelSetStorageFromPointer(primitivesLSSCtor(lssType), lssType)
}

// newLabelSetStorageFromPointer init new LabelSetStorage with pointer to constructed lss
func newLabelSetStorageFromPointer(lssPointer uintptr, lssType uint32) *LabelSetStorage {
	lss := &LabelSetStorage{pointer: lssPointer, gcDestroyDetector: &gcDestroyDetector}
	runtime.SetFinalizer(lss, func(lss *LabelSetStorage) {
		primitivesLSSDtor(lss.pointer)

		lssFinalize.With(prometheus.Labels{"type": lssTypeToString(lssType)}).Inc()
	})

	ls := prometheus.Labels{"type": lssTypeToString(lssType)}
	lssFinalize.With(ls).Add(0)
	lssCreate.With(ls).Inc()

	return lss
}

// AllocatedMemory return size of allocated memory for label sets in C++.
func (lss *LabelSetStorage) AllocatedMemory() uint64 {
	res := primitivesLSSAllocatedMemory(lss.pointer)
	runtime.KeepAlive(lss)
	return res
}

// BitsetSeries returns a copy of the bitset of added series from the lss. Read operation.
func (lss *LabelSetStorage) BitsetSeries() *BitsetSeries {
	bsPointer := primitivesLSSBitsetSeries(lss.pointer)
	runtime.KeepAlive(lss)

	return newBitsetSeriesFromPointer(bsPointer)
}

// FindOrEmplace find in lss LabelSet or emplace and return ls id.
func (lss *LabelSetStorage) FindOrEmplace(labelSet model.LabelSet) FindOrEmplaceResult {
	res := primitivesLSSFindOrEmplace(lss.pointer, labelSet)
	runtime.KeepAlive(lss)
	return res
}

// FindOrEmplaceBuilder find in lss LabelSet or emplace and return ls id.
func (lss *LabelSetStorage) FindOrEmplaceBuilder(labelSet CppLabelSetBuilder) FindOrEmplaceResult {
	res := primitivesLSSFindOrEmplaceBuilder(lss.pointer, labelSet)
	runtime.KeepAlive(lss)
	return res
}

// FindFromBuilder find labelset from builder in lss, return length ls, lsid and bool ok.
//
//nolint:gocritic // unnamedResult // lsID, length, ok
func (lss *LabelSetStorage) FindFromBuilder(
	sortedAdd []Label,
	sortedDel []string,
	snapshot *LabelSetSnapshot,
	hash uint64,
	lsID uint32,
) (uint32, uint16, bool) {
	var snapshotPointer uintptr
	if snapshot != nil {
		snapshotPointer = snapshot.pointer
	}

	length, newlsID, ok := primitivesLSSFindFromBuilder(
		lss.pointer,
		snapshotPointer,
		sortedAdd,
		sortedDel,
		hash,
		lsID,
	)

	runtime.KeepAlive(sortedAdd)
	runtime.KeepAlive(sortedDel)
	runtime.KeepAlive(snapshot)
	runtime.KeepAlive(lss)

	if !ok {
		return 0, 0, false
	}

	return newlsID, uint16(length), true // #nosec G115 // no overflow
}

// QuerySelector returns a created selector that matches the given label matchers.
func (lss *LabelSetStorage) QuerySelector(matchers []model.LabelMatcher) (
	selector uintptr,
	status uint32,
) {
	selector, status = primitivesLSSQuerySelector(lss.pointer, matchers)
	runtime.KeepAlive(lss)
	return selector, status
}

// QueryLabelNames returns a LSSQueryLabelNamesResult that matches the given label matchers.
func (lss *LabelSetStorage) QueryLabelNames(matchers []model.LabelMatcher) *LSSQueryLabelNamesResult {
	result := &LSSQueryLabelNamesResult{}

	result.status, result.names = primitivesLSSQueryLabelNames(lss.pointer, matchers)

	runtime.SetFinalizer(result, func(result *LSSQueryLabelNamesResult) {
		freeBytes(*(*[]byte)(unsafe.Pointer(&result.names))) // #nosec G103 // it's meant to be that way
	})
	return result
}

// QueryLabelValues returns a LSSQueryLabelValuesResult that matches the given label matchers.
func (lss *LabelSetStorage) QueryLabelValues(
	labelName string,
	matchers []model.LabelMatcher,
) *LSSQueryLabelValuesResult {
	result := &LSSQueryLabelValuesResult{}

	result.status, result.values = primitivesLSSQueryLabelValues(lss.pointer, labelName, matchers)

	runtime.SetFinalizer(result, func(result *LSSQueryLabelValuesResult) {
		freeBytes(*(*[]byte)(unsafe.Pointer(&result.values))) // #nosec G103 // it's meant to be that way
	})
	return result
}

// GetLabelSets - returns copy of lss data.
func (lss *LabelSetStorage) GetLabelSets(labelSetIDs []uint32) *LabelSetStorageGetLabelSetsResult {
	result := &LabelSetStorageGetLabelSetsResult{labelSets: primitivesLSSGetLabelSets(lss.pointer, labelSetIDs)}
	runtime.KeepAlive(lss)

	runtime.SetFinalizer(result, func(result *LabelSetStorageGetLabelSetsResult) {
		primitivesLSSFreeLabelSets(result.labelSets)
	})
	return result
}

// Pointer return c-pointer.
func (lss *LabelSetStorage) Pointer() uintptr {
	return lss.pointer
}

// CreateLabelSetSnapshot create LabelSetSnapshot from lss.
func (lss *LabelSetStorage) CreateLabelSetSnapshot(source SnapshotSource) *LabelSetSnapshot {
	res := newLabelSetSnapshot(primitivesLSSCreateReadonlyLss(lss.pointer), source)
	runtime.KeepAlive(lss)
	return res
}

// RangeLabelSet serialize to slice labels from lss and calls f on each label.
func (lss *LabelSetStorage) RangeLabelSet(lsID uint32, dropMetricName bool, do func(l Label) error) error {
	labelSet := labelSetSerialize(lss.pointer, lsID, dropMetricName)
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

//
// LSSQueryLabelNamesResult
//

// LSSQueryLabelNamesResult query names execution result.
type LSSQueryLabelNamesResult struct {
	status uint32
	names  []string // c allocated
}

// Status query execution.
func (r *LSSQueryLabelNamesResult) Status() uint32 {
	return r.status
}

// Names return queried names.
func (r *LSSQueryLabelNamesResult) Names() []string {
	return r.names
}

//
// LSSQueryLabelValuesResult
//

// LSSQueryLabelValuesResult query values execution result.
type LSSQueryLabelValuesResult struct {
	status uint32
	values []string // c allocated
}

// Status query execution.
func (r *LSSQueryLabelValuesResult) Status() uint32 {
	return r.status
}

// Values return queried values.
func (r *LSSQueryLabelValuesResult) Values() []string {
	return r.values
}

//
// LabelSetStorageGetLabelSetsResult
//

// LabelSetStorageGetLabelSetsResult query labelsets execution result.
type LabelSetStorageGetLabelSetsResult struct {
	labelSets []Labels // c allocated
}

// LabelsSets return queried slice labelsets.
func (r *LabelSetStorageGetLabelSetsResult) LabelsSets() []Labels {
	return r.labelSets
}

//
// BitsetSeries
//

// BitsetSeries copies of the bitset of added series from the lss.
type BitsetSeries struct {
	pointer           uintptr
	gcDestroyDetector *uint64
}

// newBitsetSeriesFromPointer init new [BitsetSeries].
func newBitsetSeriesFromPointer(bitsetSeriesPointer uintptr) *BitsetSeries {
	bitsetSeries := &BitsetSeries{pointer: bitsetSeriesPointer, gcDestroyDetector: &gcDestroyDetector}
	runtime.SetFinalizer(bitsetSeries, func(bs *BitsetSeries) {
		primitivesLSSBitsetDtor(bs.pointer)
	})

	return bitsetSeries
}
