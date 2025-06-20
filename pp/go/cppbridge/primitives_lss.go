package cppbridge

import (
	"context"
	"runtime"
	"unsafe"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/model"
)

const (
	lssEncodingBimap uint32 = iota
	lssOrderedEncodingBimap
	lssQueryableEncodingBimap
)

// lssTypeToString serialize lss type to string.
func lssTypeToString(lssType uint32) string {
	switch lssType {
	case lssEncodingBimap:
		return "encoding_bimap"

	case lssOrderedEncodingBimap:
		return "ordered_encoding_bimap"

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
	LSSQueryStatusNoPositiveMatchers uint32 = iota
	LSSQueryStatusRegexpError
	LSSQueryStatusNoMatch
	LSSQueryStatusMatch
)

//
// LSS Query Source
//

const (
	// LSSQuerySourceRule the source of query is rules.
	LSSQuerySourceRule uint32 = iota
	// LSSQuerySourceFederate the source of query is federate.
	LSSQuerySourceFederate
	// LSSQuerySourceOther the source of query is another sources.
	LSSQuerySourceOther
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

// NewOrderedLssStorage init new LabelSetStorage based on OrderedEncodingBimap.
func NewOrderedLssStorage() *LabelSetStorage {
	return newLabelSetStorage(lssOrderedEncodingBimap)
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

// FindFromBuilder label set from builder in lss, return length ls, lsid and bool ok.
//
//nolint:gocritic // unnamedResult not need
func (lss *LabelSetStorage) FindFromBuilder(
	sortedAdd []Label,
	sortedDel []string,
	snapshot *LabelSetSnapshot,
	lsID uint32,
) (uint32, uint16, bool) {
	var snapshotPointer uintptr
	if snapshot != nil {
		snapshotPointer = snapshot.pointer
	}

	length, lsID, ok := primitivesLSSFindFromBuilder(
		lss.pointer,
		snapshotPointer,
		sortedAdd,
		sortedDel,
		lsID,
	)
	runtime.KeepAlive(lss)
	runtime.KeepAlive(snapshot)
	if !ok {
		return 0, 0, false
	}

	return lsID, uint16(length), true // #nosec G115 // no overflow
}

// Query returns a LSSQueryResult that matches the given label matchers.
func (lss *LabelSetStorage) Query(matchers []model.LabelMatcher, querySource uint32) *LSSQueryResult {
	return newLSSQueryResult(primitivesLSSQuery(lss.pointer, matchers, querySource))
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

// CopyAddedSeries - copy label sets which were added via FindOrEmplace to destination
func (lss *LabelSetStorage) CopyAddedSeries(destination *LabelSetStorage) {
	primitivesLSSCopyAddedSeries(lss.pointer, destination.pointer)
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

// GetByIndex return ls id and length for ls id by index.
//
//nolint:gocritic // unnamedResult not need
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
// Caller
//

type ctxCallerKey struct{}

// GetCaller get from context callerID, if not exist return LSSQuerySourceOther.
func GetCaller(ctx context.Context) uint32 {
	v, ok := ctx.Value(ctxCallerKey{}).(uint32)
	if !ok {
		return LSSQuerySourceOther
	}

	if v >= LSSQuerySourceOther {
		return LSSQuerySourceOther
	}

	return v
}

// SetCaller set callerID to context.
func SetCaller(parent context.Context, callerID uint32) context.Context {
	return context.WithValue(parent, ctxCallerKey{}, callerID)
}
