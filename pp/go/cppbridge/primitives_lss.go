package cppbridge

import (
	"context"
	"runtime"
	"slices"
	"sync"
	"time"
	"unsafe"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/prometheus/pp/go/model"
)

var (
	lssCreate = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "prompp_cppbridge_lss_create",
			Help: "Current create lsses.",
		},
		[]string{"type"},
	)

	lssFinalize = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "prompp_cppbridge_lss_finalize",
			Help: "Current finalize lsses.",
		},
		[]string{"type"},
	)
)

const (
	lssEncodingBimap uint32 = iota
	lssOrderedEncodingBimap
	lssQueryableEncodingBimap
	lssShared
)

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
	LSSQuerySourceRule uint32 = iota
	LSSQuerySourceFederate
	LSSQuerySourceOther
)

//
// LabelSetStorage
//

//
// LabelSetStorage
//

// LabelSetStorage go wrapper for C-LabelSetStorage.
type LabelSetStorage struct {
	pointer uintptr
	maxID   uint32
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
	lss := &LabelSetStorage{pointer: primitivesLSSCtor(lssType)}
	runtime.SetFinalizer(lss, func(lss *LabelSetStorage) {
		primitivesLSSDtor(lss.pointer)
	})

	return lss
}

// newCopiedLssStorrage init new LabelSetStorage based on CopiedLss.
func newCopiedLssStorrage(lssCopyPtr uintptr) *LabelSetStorage {
	lss := &LabelSetStorage{pointer: lssCopyPtr}
	runtime.SetFinalizer(lss, func(lss *LabelSetStorage) {
		primitivesLSSDtor(lss.pointer)

		lssFinalize.With(prometheus.Labels{"type": "copied"}).Inc()
	})

	lssCreate.With(prometheus.Labels{"type": "copied"}).Inc()

	return lss
}

// AllocatedMemory return size of allocated memory for label sets in C++.
func (lss *LabelSetStorage) AllocatedMemory() uint64 {
	return primitivesLSSAllocatedMemory(lss.pointer)
}

// FindOrEmplace find in lss LabelSet or emplace and return ls id.
func (lss *LabelSetStorage) FindOrEmplace(labelSet model.LabelSet) uint32 {
	id := primitivesLSSFindOrEmplace(lss.pointer, labelSet)
	lss.maxID = max(id, lss.maxID)
	return id
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

	runtime.SetFinalizer(result, func(result *LabelSetStorageGetLabelSetsResult) {
		primitivesLSSFreeLabelSets(result.labelSets)
	})
	return result
}

// Pointer return c-pointer.
func (lss *LabelSetStorage) Pointer() uintptr {
	return lss.pointer
}

//
// LSSQueryResult
//

// lssQueryResult query execution result in lss, filled in c.
type lssQueryResult struct {
	matches         []uint32 // c allocated
	labelSetLengths []uint16 // c allocated
	status          uint32
}

//
// bufCopiedLSS
//

// bufCopiedLSS buffer CopiedLSS for deduplicate.
var bufCopiedLSS = sync.Map{}

type bufLSSValue struct {
	lssMain uintptr
	lssCopy *LabelSetStorage
	maxlsid uint32
	timer   *time.Timer
}

func getLSSCopy(lssMainPtr, lssCopyPtr uintptr, maxlsid uint32) *LabelSetStorage {
	var lssCopy *LabelSetStorage

	v, ok := bufCopiedLSS.Load(lssMainPtr)
	if !ok {
		lssCopy = newCopiedLssStorrage(lssCopyPtr)
		bufCopiedLSS.Store(lssMainPtr, &bufLSSValue{
			lssMain: lssMainPtr,
			lssCopy: lssCopy,
			maxlsid: maxlsid,
			timer: time.AfterFunc(1*time.Minute, func() {
				bufCopiedLSS.Delete(lssMainPtr)
			}),
		})

		return lssCopy
	}

	bv := v.(*bufLSSValue)
	if bv.maxlsid < maxlsid {
		lssCopy = newCopiedLssStorrage(lssCopyPtr)
		bufCopiedLSS.Store(lssMainPtr, &bufLSSValue{
			lssMain: lssMainPtr,
			lssCopy: lssCopy,
			maxlsid: maxlsid,
			timer: time.AfterFunc(1*time.Minute, func() {
				bufCopiedLSS.Delete(lssMainPtr)
			}),
		})

		return lssCopy
	}

	bv.timer.Reset(1 * time.Minute)

	primitivesLSSDtor(lssCopyPtr)

	return bv.lssCopy
}

// LSSQueryResult query execution result in lss with copy.
type LSSQueryResult struct {
	queryResult *lssQueryResult
	lssCopy     *LabelSetStorage
}

// newLSSQueryResult init new LSSQueryResult.
func newLSSQueryResult(
	matches []uint32,
	labelSetLengths []uint16,
	lssMainPtr uintptr,
	lssCopyPtr uintptr,
	status uint32,
) *LSSQueryResult {
	queryResult := &lssQueryResult{
		matches:         matches,
		labelSetLengths: labelSetLengths,
		status:          status,
	}

	if status != LSSQueryStatusMatch {
		primitivesLabelSetMatchesFree(queryResult)
		primitivesLSSDtor(lssCopyPtr)

		return &LSSQueryResult{queryResult: queryResult}
	}

	runtime.SetFinalizer(queryResult, func(result *lssQueryResult) {
		primitivesLabelSetMatchesFree(result)
	})

	lqr := &LSSQueryResult{
		queryResult: queryResult,
		lssCopy:     getLSSCopy(lssMainPtr, lssCopyPtr, slices.Max(matches)),
	}

	return lqr
}

// Status query execution.
func (r *LSSQueryResult) Status() uint32 {
	return r.queryResult.status
}

// IDs return labels sets ids.
func (r *LSSQueryResult) IDs() []uint32 {
	return r.queryResult.matches
}

// LabelSetLengths return labels sets lengths.
func (r *LSSQueryResult) LabelSetLengths() []uint16 {
	return r.queryResult.labelSetLengths
}

// MatchesRange calls callback sequentially for each result.
func (r *LSSQueryResult) MatchesRange(callback func(lss *LabelSetStorage, lsid uint32, length uint16)) {
	for i, lsId := range r.queryResult.matches {
		callback(r.lssCopy, lsId, r.queryResult.labelSetLengths[i])
	}
}

// Lss return copy LabelSetStorage.
func (r *LSSQueryResult) Lss() *LabelSetStorage {
	return r.lssCopy
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
