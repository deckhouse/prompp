package cppbridge

import (
	"context"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
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
	return newLabelSetStorageFromPointer(primitivesLSSCtor(lssType))
}

// newLabelSetStorageFromPointer init new LabelSetStorage with pointer to constructed lss
func newLabelSetStorageFromPointer(lssPointer uintptr) *LabelSetStorage {
	lss := &LabelSetStorage{pointer: lssPointer}
	runtime.SetFinalizer(lss, func(lss *LabelSetStorage) {
		primitivesLSSDtor(lss.pointer)
	})

	return lss
}

// newReadOnlyLssStorage init new LabelSetStorage based on lssReadOnly.
func newReadOnlyLssStorage(lssROPtr uintptr) *LabelSetStorage {
	lss := &LabelSetStorage{pointer: lssROPtr}
	runtime.SetFinalizer(lss, func(lss *LabelSetStorage) {
		primitivesLSSDtor(lss.pointer)

		lssFinalize.With(prometheus.Labels{"type": "read_only"}).Inc()
	})

	lssCreate.With(prometheus.Labels{"type": "read_only"}).Inc()

	return lss
}

// AllocatedMemory return size of allocated memory for label sets in C++.
func (lss *LabelSetStorage) AllocatedMemory() uint64 {
	return primitivesLSSAllocatedMemory(lss.pointer)
}

// FindOrEmplace find in lss LabelSet or emplace and return ls id.
func (lss *LabelSetStorage) FindOrEmplace(labelSet model.LabelSet) FindOrEmplaceResult {
	return primitivesLSSFindOrEmplace(lss.pointer, labelSet)
}

// FindOrEmplaceBuilder find in lss LabelSet or emplace and return ls id.
func (lss *LabelSetStorage) FindOrEmplaceBuilder(labelSet model.CppLabelSetBuilder) FindOrEmplaceResult {
	return primitivesLSSFindOrEmplaceBuilder(lss.pointer, labelSet)
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

// CopyAddedSeries - copy label sets which were added via FindOrEmplace to destination
func (lss *LabelSetStorage) CopyAddedSeries(destination *LabelSetStorage) {
	primitivesLSSCopyAddedSeries(lss.pointer, destination.pointer)
}

// Pointer return c-pointer.
func (lss *LabelSetStorage) Pointer() uintptr {
	return lss.pointer
}

// CreateReadonlyLss - create readonly copy of lss
func (lss *LabelSetStorage) CreateReadonlyLss() *LabelSetStorage {
	return newReadOnlyLssStorage(primitivesLSSCreateReadonlyLss(lss.pointer))
}

// RangeLabelSet serialize to slice labels from lss and calls f on each label.
func (lss *LabelSetStorage) RangeLabelSet(lsID uint32, do func(l Label) error) error {
	labelSet := labelSetSerialize(lss.pointer, lsID)

	for i := range labelSet {
		if err := do(labelSet[i]); err != nil {
			labelSetFree(labelSet)
			return err
		}
	}

	labelSetFree(labelSet)

	return nil
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
// cacheReadOnlyLSS
//

// cacheReadOnlyLSS global cache lssReadOnly.
var cacheReadOnlyLSS = newReadOnlyLSSBuffer()

type valueROLSSCache struct {
	lssRO      *LabelSetStorage
	lastUpdate int64
	maxlsid    uint32
}

// readOnlyLSSCache cache lssReadOnly for deduplicate.
type readOnlyLSSCache struct {
	sync.Map
}

func newReadOnlyLSSBuffer() *readOnlyLSSCache {
	c := &readOnlyLSSCache{}

	go c.clearing()

	return c
}

// clearing the cache of obsolete data.
func (c *readOnlyLSSCache) clearing() {
	ticker := time.NewTicker(60 * time.Second)

	for {
		now := (<-ticker.C).Unix()
		c.Range(func(key, value any) bool {
			if now-atomic.LoadInt64(&value.(*valueROLSSCache).lastUpdate) > 60 {
				c.Delete(key)
			}

			return true
		})
	}
}

// getROLSS return read only LSS from cache or add ro cache.
func (c *readOnlyLSSCache) getROLSS(lssMainPtr, lssROPtr uintptr, maxlsid uint32) *LabelSetStorage {
	var lssRO *LabelSetStorage

	v, ok := c.Load(lssMainPtr)
	if !ok {
		lssRO = newReadOnlyLssStorage(lssROPtr)
		c.Store(lssMainPtr, &valueROLSSCache{
			lssRO:      lssRO,
			maxlsid:    maxlsid,
			lastUpdate: time.Now().Unix(),
		})

		return lssRO
	}

	bv := v.(*valueROLSSCache)
	if bv.maxlsid < maxlsid {
		lssRO = newReadOnlyLssStorage(lssROPtr)
		c.Store(lssMainPtr, &valueROLSSCache{
			lssRO:      lssRO,
			maxlsid:    maxlsid,
			lastUpdate: time.Now().Unix(),
		})

		return lssRO
	}

	atomic.StoreInt64(&bv.lastUpdate, time.Now().Unix())

	primitivesLSSDtor(lssROPtr)

	return bv.lssRO
}

//
// LSSQueryResult
//

// LSSQueryResult query execution result in lss with copy.
type LSSQueryResult struct {
	queryResult *lssQueryResult
	lssRO       *LabelSetStorage
}

// newLSSQueryResult init new LSSQueryResult.
func newLSSQueryResult(
	matches []uint32,
	labelSetLengths []uint16,
	lssMainPtr uintptr,
	lssROPtr uintptr,
	status uint32,
) *LSSQueryResult {
	queryResult := &lssQueryResult{
		matches:         matches,
		labelSetLengths: labelSetLengths,
		status:          status,
	}

	if status != LSSQueryStatusMatch {
		primitivesLabelSetMatchesFree(queryResult)
		primitivesLSSDtor(lssROPtr)

		return &LSSQueryResult{queryResult: queryResult}
	}

	runtime.SetFinalizer(queryResult, func(result *lssQueryResult) {
		primitivesLabelSetMatchesFree(result)
	})

	lqr := &LSSQueryResult{
		queryResult: queryResult,
		lssRO:       cacheReadOnlyLSS.getROLSS(lssMainPtr, lssROPtr, slices.Max(matches)),
	}

	return lqr
}

// GetByIndex return ls id and length for ls id by index.
func (r *LSSQueryResult) GetByIndex(i int) (uint32, uint16) {
	return r.queryResult.matches[i], r.queryResult.labelSetLengths[i]
}

// IDs return labels sets ids.
func (r *LSSQueryResult) IDs() []uint32 {
	return r.queryResult.matches
}

// LSS return current read only lss from result.
func (r *LSSQueryResult) LSS() *LabelSetStorage {
	return r.lssRO
}

// LabelSetLengths return labels sets lengths.
func (r *LSSQueryResult) LabelSetLengths() []uint16 {
	return r.queryResult.labelSetLengths
}

// ReadonlyLss return readonly lss
func (r *LSSQueryResult) ReadonlyLss() *LabelSetStorage {
	return r.lssRO
}

// Len of result.
func (r *LSSQueryResult) Len() int {
	return len(r.queryResult.matches)
}

// MatchesRange calls callback sequentially for each result.
func (r *LSSQueryResult) MatchesRange(callback func(lss *LabelSetStorage, lsid uint32, length uint16)) {
	for i, lsId := range r.queryResult.matches {
		callback(r.lssRO, lsId, r.queryResult.labelSetLengths[i])
	}
}

// MatchesIndexRange calls callback sequentially for each result.
func (r *LSSQueryResult) MatchesIndexRange(callback func(lss *LabelSetStorage, index int, lsid uint32, length uint16)) {
	for i, lsId := range r.queryResult.matches {
		callback(r.lssRO, i, lsId, r.queryResult.labelSetLengths[i])
	}
}

// Status query execution.
func (r *LSSQueryResult) Status() uint32 {
	return r.queryResult.status
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
