package cppbridge

import (
	"math"
	"runtime"
	"unsafe"
)

const (
	NormalNaN uint64 = 0x7ff8000000000001

	StaleNaN uint64 = 0x7ff0000000000002

	NoDownsampling = 0
)

func IsStaleNaN(v float64) bool {
	return math.Float64bits(v) == StaleNaN
}

const (
	MaxPointsInChunk            = 240
	Uint32Size                  = 4
	SerializedChunkMetadataSize = 13

	DataStorageQueryStatusSuccess      uint8 = 0
	DataStorageQueryStatusNeedDataLoad uint8 = 1
)

type TimeInterval struct {
	MinT int64
	MaxT int64
}

func NewInvalidTimeInterval() TimeInterval {
	return TimeInterval{
		MinT: math.MaxInt64,
		MaxT: math.MinInt64,
	}
}

func newInvalidTimeIntervalPtr() *TimeInterval {
	return &TimeInterval{
		MinT: math.MaxInt64,
		MaxT: math.MinInt64,
	}
}

func (t *TimeInterval) IsInvalid() bool {
	return t.MinT == math.MaxInt64 && t.MaxT == math.MinInt64
}

type Sample struct {
	Timestamp int64
	Value     float64
}

// RecodedChunk describes a chunk whose payload is borrowed from its [ChunkRecoder].
type RecodedChunk struct {
	TimeInterval
	SeriesId     uint32
	SamplesCount uint8
	HasMoreData  bool
	ChunkData    []byte
}

const (
	InvalidSeriesId = math.MaxUint32

	UnlimitedLsIdBatchSize uint32 = math.MaxUint32
)

// ChunkRecoder wraps a native recoder and keeps its input storage reachable.
// It must not be copied or used after [ChunkRecoder.Close].
type ChunkRecoder struct {
	recoder      uintptr
	recodedChunk RecodedChunk

	lss            *LabelSetStorage
	dataStorage    *DataStorage
	serializedData *DataStorageSerializedData
}

// NewChunkRecoder creates a recoder for block writing, with a finalizer as a fallback.
func NewChunkRecoder(
	lss *LabelSetStorage,
	lsIdBatchSize uint32,
	dataStorage *DataStorage,
	timeInterval TimeInterval,
	downsamplingMs int64,
) *ChunkRecoder {
	recoder := &ChunkRecoder{
		recoder: seriesDataChunkRecoderCtor(
			lss.Pointer(),
			lsIdBatchSize,
			dataStorage.dataStorage,
			timeInterval,
			downsamplingMs,
		),
		lss:         lss,
		dataStorage: dataStorage,
	}
	runtime.KeepAlive(lss)
	runtime.KeepAlive(dataStorage)
	runtime.SetFinalizer(recoder, (*ChunkRecoder).Close)

	return recoder
}

// NewSerializedChunkRecoder borrows serializedData. The caller must close the recoder
// before closing serializedData.
func NewSerializedChunkRecoder(serializedData *DataStorageSerializedData, timeInterval TimeInterval) *ChunkRecoder {
	return &ChunkRecoder{
		recoder:        seriesDataSerializedChunkRecoderCtor(serializedData, timeInterval),
		serializedData: serializedData,
	}
}

// Close destroys the recoder and invalidates borrowed payloads. It does not close the input.
// Repeated calls, including calls on a nil or zero recoder, are no-ops.
func (recoder *ChunkRecoder) Close() {
	if recoder == nil || recoder.recoder == 0 {
		return
	}

	if recoder.lss != nil {
		runtime.SetFinalizer(recoder, nil)
	}

	seriesDataChunkRecoderDtor(recoder.recoder)
	runtime.KeepAlive(recoder)
	*recoder = ChunkRecoder{}
}

// NextBatch advances the block-writing recoder to the next batch.
func (recoder *ChunkRecoder) NextBatch() bool {
	result := seriesDataChunkRecoderNextBatch(recoder.recoder)
	runtime.KeepAlive(recoder)

	return result
}

// RecodeNextChunk returns metadata and a borrowed payload, valid until the next
// recode, batch advance, or close. Keep the recoder reachable until the payload
// is consumed or copied.
func (recoder *ChunkRecoder) RecodeNextChunk() RecodedChunk {
	seriesDataChunkRecoderRecodeNextChunk(recoder.recoder, &recoder.recodedChunk)
	result := recoder.recodedChunk
	runtime.KeepAlive(recoder)

	return result
}

func getSeriesIDFromBytes(data []byte) uint32 {
	return *(*uint32)(unsafe.Pointer(&data[0])) // #nosec G103 // it's meant to be that way
}

type HeadDataStorageSerializedChunks struct {
	data []byte
}

type HeadDataStorageSerializedChunkMetadata [SerializedChunkMetadataSize]byte

func (cm HeadDataStorageSerializedChunkMetadata) SeriesID() uint32 {
	return *(*uint32)(unsafe.Pointer(&cm[0]))
}

func (r *HeadDataStorageSerializedChunks) NumberOfChunks() int {
	if len(r.data) == 0 {
		return 0
	}

	return int(*(*int32)(unsafe.Pointer(&r.data[0])))
}

func (r *HeadDataStorageSerializedChunks) Len() int {
	return len(r.data)
}

func (r *HeadDataStorageSerializedChunks) Data() []byte {
	return r.data
}

func (r *HeadDataStorageSerializedChunks) Metadata(chunkIndex int) HeadDataStorageSerializedChunkMetadata {
	offset := Uint32Size + chunkIndex*SerializedChunkMetadataSize
	return HeadDataStorageSerializedChunkMetadata(r.data[offset : offset+SerializedChunkMetadataSize])
}

type HeadDataStorageSerializedChunkIndex struct {
	m map[uint32][]int
}

func (r *HeadDataStorageSerializedChunks) MakeIndex() HeadDataStorageSerializedChunkIndex {
	m := make(map[uint32][]int)
	offset := Uint32Size
	n := r.NumberOfChunks()
	for i := 0; i < n; i, offset = i+1, offset+SerializedChunkMetadataSize {
		sID := getSeriesIDFromBytes(r.data[offset : offset+4])
		m[sID] = append(m[sID], offset)
	}
	return HeadDataStorageSerializedChunkIndex{m}
}

func (i HeadDataStorageSerializedChunkIndex) Has(seriesID uint32) bool {
	return len(i.m[seriesID]) > 0
}

func (i HeadDataStorageSerializedChunkIndex) Len() int {
	return len(i.m)
}

func (i HeadDataStorageSerializedChunkIndex) Chunks(r *HeadDataStorageSerializedChunks, seriesID uint32) []HeadDataStorageSerializedChunkMetadata {
	offsets, ok := i.m[seriesID]
	if !ok {
		return nil
	}
	res := make([]HeadDataStorageSerializedChunkMetadata, len(offsets))
	for i, offset := range offsets {
		res[i] = HeadDataStorageSerializedChunkMetadata(r.data[offset : offset+SerializedChunkMetadataSize])
	}
	return res
}

// DataStorageSerializedData owns the native buffer of a range query.
// It must not be copied, and its owner must close it after deferred loading
// and all borrowers have finished.
type DataStorageSerializedData struct {
	serializedData uintptr
	ds             *DataStorage
}

// NewDataStorageSerializedData creates a range query result before the native buffer is available.
func NewDataStorageSerializedData(ds *DataStorage) *DataStorageSerializedData {
	return &DataStorageSerializedData{ds: ds}
}

// Close frees the native buffer and releases the storage reference. The caller must first
// finish or discard any deferred native query and close any recoder borrowing the buffer.
// Close does nothing on a nil receiver and may be called repeatedly.
func (sd *DataStorageSerializedData) Close() {
	if sd == nil {
		return
	}

	if sd.serializedData != 0 {
		seriesDataSerializedDataDtor(sd.serializedData)
		runtime.KeepAlive(sd)
		sd.serializedData = 0
	}

	sd.ds = nil
}

// IsClosed reports whether there is no native buffer, including for a nil
// receiver or a pending query whose buffer has not been populated.
func (sd *DataStorageSerializedData) IsClosed() bool {
	return sd == nil || sd.serializedData == 0
}

// Next returns the next series and chunk reference. It requires an open buffer.
func (sd *DataStorageSerializedData) Next() (uint32, uint32) {
	seriesID, chunkRef := seriesDataSerializedDataNext(sd.serializedData)
	runtime.KeepAlive(sd)
	return seriesID, chunkRef
}

type DataStorageSerializedDataSamplesIteratorControlBlock struct {
	timestamp        int64
	value            float64
	remainingSamples uint8
}

// DataStorageSerializedDataSamplesIterator borrows the buffer supplied on construction or reset.
type DataStorageSerializedDataSamplesIterator struct {
	DataStorageSerializedDataSamplesIteratorControlBlock
	cppInternalData [unsafe.Sizeof(CppSerializedDataSamplesIterator{}) - unsafe.Sizeof(DataStorageSerializedDataSamplesIteratorControlBlock{})]byte
}

// NewDataStorageSerializedDataSamplesIterator borrows serializedData for iteration.
func NewDataStorageSerializedDataSamplesIterator(serializedData *DataStorageSerializedData, chunkRef uint32) DataStorageSerializedDataSamplesIterator {
	it := DataStorageSerializedDataSamplesIterator{}
	seriesDataSerializedDataSamplesIteratorCtor(&it, serializedData.serializedData, chunkRef)
	runtime.KeepAlive(serializedData)

	return it
}

func (it *DataStorageSerializedDataSamplesIterator) Next() {
	seriesDataSerializedDataSamplesIteratorNext(it)
}

func (it *DataStorageSerializedDataSamplesIterator) Seek(timestamp int64) {
	seriesDataSerializedDataSamplesIteratorSeek(it, timestamp)
}

// Reset reuses the iterator, borrowing serializedData for subsequent iteration.
func (it *DataStorageSerializedDataSamplesIterator) Reset(serializedData *DataStorageSerializedData, chunkRef uint32) {
	seriesDataSerializedDataSamplesIteratorReset(it, serializedData.serializedData, chunkRef)
	runtime.KeepAlive(serializedData)
}

func (it *DataStorageSerializedDataSamplesIterator) HasData() bool {
	return it.remainingSamples != 0
}

func (it *DataStorageSerializedDataSamplesIterator) Timestamp() int64 {
	return it.timestamp
}

func (it *DataStorageSerializedDataSamplesIterator) Value() float64 {
	return it.value
}

type sampleControlBlock struct {
	timestamp int64
	value     float64
}

func (it *sampleControlBlock) HasData() bool {
	return it.timestamp != math.MinInt64
}

func (it *sampleControlBlock) Timestamp() int64 {
	return it.timestamp
}

func (it *sampleControlBlock) Value() float64 {
	return it.value
}

// DataStorageSerializedDataAggregationIterator borrows the buffer supplied on construction or reset.
type DataStorageSerializedDataAggregationIterator struct {
	sampleControlBlock
	cppInternalData [unsafe.Sizeof(CppSerializedDataAggregationIterator{}) - unsafe.Sizeof(sampleControlBlock{})]byte
}

// NewDataStorageSerializedDataAggregationIterator borrows serializedData for iteration.
func NewDataStorageSerializedDataAggregationIterator(serializedData *DataStorageSerializedData, chunkRef uint32) DataStorageSerializedDataAggregationIterator {
	it := DataStorageSerializedDataAggregationIterator{}
	seriesDataSerializedDataAggregationIteratorCtor(&it, serializedData.serializedData, chunkRef)
	runtime.KeepAlive(serializedData)

	return it
}

func (it *DataStorageSerializedDataAggregationIterator) Next() {
	seriesDataSerializedDataAggregationIteratorNext(it)
}

// Reset reuses the iterator, borrowing serializedData for subsequent iteration.
func (it *DataStorageSerializedDataAggregationIterator) Reset(serializedData *DataStorageSerializedData, chunkRef uint32) {
	seriesDataSerializedDataAggregationIteratorReset(it, serializedData.serializedData, chunkRef)
	runtime.KeepAlive(serializedData)
}

// DataStorageSerializedDataMultiSeriesIterator borrows the buffer supplied on construction or reset.
type DataStorageSerializedDataMultiSeriesIterator struct {
	sampleControlBlock
	cppInternalData [unsafe.Sizeof(CppSerializedDataMultiSeriesIterator{}) - unsafe.Sizeof(sampleControlBlock{})]byte
}

// NewDataStorageSerializedDataMultiSeriesIterator borrows serializedData for iteration.
func NewDataStorageSerializedDataMultiSeriesIterator(serializedData *DataStorageSerializedData, seriesIDs []uint32) DataStorageSerializedDataMultiSeriesIterator {
	it := DataStorageSerializedDataMultiSeriesIterator{}
	seriesDataSerializedDataMultiSeriesIteratorCtor(&it, serializedData.serializedData, seriesIDs)
	runtime.KeepAlive(serializedData)

	return it
}

func (it *DataStorageSerializedDataMultiSeriesIterator) Next() {
	seriesDataSerializedDataMultiSeriesIteratorNext(it)
}

// Reset reuses the iterator, borrowing serializedData for subsequent iteration.
func (it *DataStorageSerializedDataMultiSeriesIterator) Reset(serializedData *DataStorageSerializedData, seriesIDs []uint32) {
	seriesDataSerializedDataMultiSeriesIteratorReset(it, serializedData.serializedData, seriesIDs)
	runtime.KeepAlive(serializedData)
}

// Close frees the iterator's own allocations without reading the borrowed buffer.
func (it *DataStorageSerializedDataMultiSeriesIterator) Close() {
	seriesDataSerializedDataMultiSeriesIteratorDtor(it)
}

// UnloadedDataLoader is Go wrapper around series_data::Loader.
type UnloadedDataLoader struct {
	loader uintptr
	ds     *DataStorage
}

func (loader *UnloadedDataLoader) Load(snapshot []byte, isLast bool) {
	seriesDataUnloadedDataLoaderLoad(loader.loader, snapshot, isLast)
	runtime.KeepAlive(loader)
}

func (ds *DataStorage) CreateLoader(queriers []uintptr) *UnloadedDataLoader {
	result := &UnloadedDataLoader{
		loader: seriesDataUnloadedDataLoaderCtor(ds.dataStorage, queriers),
		ds:     ds,
	}
	runtime.KeepAlive(queriers)
	runtime.KeepAlive(ds)

	runtime.SetFinalizer(result, func(loader *UnloadedDataLoader) {
		seriesDataUnloadedDataLoaderDtor(loader.loader)
	})

	return result
}

// UnloadedDataRevertableLoader is Go wrapper around series_data::RevertableLoader.
type UnloadedDataRevertableLoader struct {
	UnloadedDataLoader
	lss *LabelSetStorage
}

func (loader *UnloadedDataRevertableLoader) NextBatch() bool {
	result := seriesDataUnloadedDataRevertableLoaderNextBatch(loader.loader)
	runtime.KeepAlive(loader)
	return result
}

func (ds *DataStorage) CreateRevertableLoader(lss *LabelSetStorage, lsIdBatchSize uint32) *UnloadedDataRevertableLoader {
	result := &UnloadedDataRevertableLoader{
		UnloadedDataLoader: UnloadedDataLoader{
			loader: seriesDataUnloadedDataRevertableLoaderCtor(lss.pointer, lsIdBatchSize, ds.dataStorage),
			ds:     ds,
		},
		lss: lss,
	}

	runtime.SetFinalizer(result, func(loader *UnloadedDataRevertableLoader) {
		seriesDataUnloadedDataLoaderDtor(loader.loader)
	})

	return result
}
