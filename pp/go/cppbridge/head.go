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

func (t *TimeInterval) IsInvalid() bool {
	return t.MinT == math.MaxInt64 && t.MaxT == math.MinInt64
}

type Sample struct {
	Timestamp int64
	Value     float64
}

// HeadDataStorage is Go wrapper around series_data::Data_storage.
type HeadDataStorage struct {
	dataStorage       uintptr
	gcDestroyDetector *uint64
	timeInterval      TimeInterval
}

// NewHeadDataStorage - constructor.
func NewHeadDataStorage() *HeadDataStorage {
	ds := &HeadDataStorage{
		dataStorage:       seriesDataDataStorageCtor(),
		gcDestroyDetector: &gcDestroyDetector,
		timeInterval:      NewInvalidTimeInterval(),
	}

	runtime.SetFinalizer(ds, func(ds *HeadDataStorage) {
		seriesDataDataStorageDtor(ds.dataStorage)
	})

	return ds
}

// Reset - resets data storage.
func (ds *HeadDataStorage) Reset() {
	seriesDataDataStorageReset(ds.dataStorage)
	ds.timeInterval = NewInvalidTimeInterval()
}

func (ds *HeadDataStorage) TimeInterval(invalidateCache bool) TimeInterval {
	if invalidateCache || ds.timeInterval.IsInvalid() {
		ds.timeInterval = seriesDataDataStorageTimeInterval(ds.dataStorage)
		runtime.KeepAlive(ds)
	}

	return ds.timeInterval
}

func (ds *HeadDataStorage) GetQueriedSeriesBitset() []byte {
	size := seriesDataDataStorageQueriedSeriesBitsetSize(ds.dataStorage)
	bitset := seriesDataDataStorageQueriedSeriesBitset(ds.dataStorage, make([]byte, 0, size))
	runtime.KeepAlive(ds)
	return bitset
}

func (ds *HeadDataStorage) SetQueriedSeriesBitset(bitset []byte) bool {
	result := seriesDataDataStorageQueriedSeriesSetBitset(ds.dataStorage, bitset)
	runtime.KeepAlive(ds)
	return result
}

func (ds *HeadDataStorage) Pointer() uintptr {
	return ds.dataStorage
}

func (ds *HeadDataStorage) AllocatedMemory() uint64 {
	res := seriesDataDataStorageAllocatedMemory(ds.dataStorage)
	runtime.KeepAlive(ds)
	return res
}

type UnusedSeriesDataUnloader struct {
	unloader uintptr
	ds       *HeadDataStorage
}

func (u *UnusedSeriesDataUnloader) CreateSnapshot() []byte {
	snapshot := seriesDataUnusedSeriesDataUnloaderCreateSnapshot(u.unloader)
	runtime.KeepAlive(u)
	return snapshot
}

func (u *UnusedSeriesDataUnloader) Unload() {
	seriesDataUnusedSeriesDataUnloaderUnload(u.unloader)
	runtime.KeepAlive(u)
}

func (ds *HeadDataStorage) CreateUnusedSeriesDataUnloader() *UnusedSeriesDataUnloader {
	unloader := &UnusedSeriesDataUnloader{
		unloader: seriesDataUnusedSeriesDataUnloaderCtor(ds.dataStorage),
		ds:       ds,
	}

	runtime.SetFinalizer(unloader, func(u *UnusedSeriesDataUnloader) {
		seriesDataUnusedSeriesDataUnloaderDtor(u.unloader)
	})

	return unloader
}

// HeadEncoder is Go wrapper around series_data::Encoder.
type HeadEncoder struct {
	encoder     uintptr
	dataStorage *HeadDataStorage
}

// NewHeadEncoderWithDataStorage - constructor.
func NewHeadEncoderWithDataStorage(dataStorage *HeadDataStorage) *HeadEncoder {
	encoder := &HeadEncoder{
		encoder:     seriesDataEncoderCtor(dataStorage.dataStorage),
		dataStorage: dataStorage,
	}

	runtime.SetFinalizer(encoder, func(e *HeadEncoder) {
		seriesDataEncoderDtor(e.encoder)
	})

	return encoder
}

// NewHeadEncoder - constructor.
func NewHeadEncoder() *HeadEncoder {
	return NewHeadEncoderWithDataStorage(NewHeadDataStorage())
}

// Encode - encodes single triplet.
func (e *HeadEncoder) Encode(seriesID uint32, timestamp int64, value float64) {
	seriesDataEncoderEncode(e.encoder, seriesID, timestamp, value)
	runtime.KeepAlive(e)
}

// EncodeInnerSeriesSlice - encodes InnerSeries slice produced by relabeler.
func (e *HeadEncoder) EncodeInnerSeriesSlice(innerSeriesSlice []InnerSeries) {
	seriesDataEncoderEncodeInnerSeriesSlice(e.encoder, innerSeriesSlice)
}

func (e *HeadEncoder) MergeOutOfOrderChunks() {
	seriesDataEncoderMergeOutOfOrderChunks(e.encoder)
}

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

// ChunkRecoder is Go wrapper around C++ ChunkRecoder.
type ChunkRecoder struct {
	recoder      uintptr
	recodedChunk RecodedChunk

	lss            *LabelSetStorage
	dataStorage    *HeadDataStorage
	serializedData *DataStorageSerializedData
}

func NewChunkRecoder(lss *LabelSetStorage, lsIdBatchSize uint32, dataStorage *HeadDataStorage, timeInterval TimeInterval) *ChunkRecoder {
	return initializeChunkRecoder(lss, dataStorage, nil, seriesDataChunkRecoderCtor(lss.Pointer(), lsIdBatchSize, dataStorage.dataStorage, timeInterval))
}

func NewSerializedChunkRecoder(serializedData *DataStorageSerializedData, timeInterval TimeInterval) *ChunkRecoder {
	return initializeChunkRecoder(nil, nil, serializedData, seriesDataSerializedChunkRecoderCtor(serializedData, timeInterval))
}

func initializeChunkRecoder(
	lss *LabelSetStorage,
	dataStorage *HeadDataStorage,
	serializedData *DataStorageSerializedData,
	recoder uintptr,
) *ChunkRecoder {
	chunkRecoder := &ChunkRecoder{
		recoder:        recoder,
		lss:            lss,
		dataStorage:    dataStorage,
		serializedData: serializedData,
	}

	runtime.SetFinalizer(chunkRecoder, func(chunkRecoder *ChunkRecoder) {
		seriesDataChunkRecoderDtor(chunkRecoder.recoder)
	})

	return chunkRecoder
}

func (recoder *ChunkRecoder) RecodeNextChunk() RecodedChunk {
	seriesDataChunkRecoderRecodeNextChunk(recoder.recoder, &recoder.recodedChunk)
	return recoder.recodedChunk
}

func (recoder *ChunkRecoder) NextBatch() bool {
	result := seriesDataChunkRecoderNextBatch(recoder.recoder)
	runtime.KeepAlive(recoder)
	return result
}

type HeadDataStorageQuery struct {
	StartTimestampMs int64
	EndTimestampMs   int64
	LabelSetIDs      []uint32
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

func (ds *HeadDataStorage) Query(query HeadDataStorageQuery, downsamplingMs int64) DataStorageQueryResult {
	sd := NewDataStorageSerializedData()
	querier, status := seriesDataDataStorageQueryV2(ds.dataStorage, query, sd, downsamplingMs)
	return DataStorageQueryResult{
		Querier:        querier,
		Status:         status,
		SerializedData: sd,
	}
}

func (ds *HeadDataStorage) InstantQuery(targetTimestamp, defaultTimestamp int64, labelSetIDs []uint32) ([]Sample, DataStorageQueryResult) {
	samples := make([]Sample, len(labelSetIDs))
	if defaultTimestamp != 0 {
		for index := range samples {
			samples[index].Timestamp = defaultTimestamp
		}
	}
	return samples, seriesDataDataStorageInstantQuery(ds.dataStorage, labelSetIDs, targetTimestamp, samples)
}

func (ds *HeadDataStorage) QueryFinal(queriers []uintptr) {
	seriesDataDataStorageQueryFinal(queriers)
	runtime.KeepAlive(queriers)
}

type DataStorageSerializedData struct {
	serializedData uintptr
}

func NewDataStorageSerializedData() *DataStorageSerializedData {
	sd := &DataStorageSerializedData{}
	runtime.SetFinalizer(sd, func(sd *DataStorageSerializedData) {
		seriesDataSerializedDataDtor(sd.serializedData)
	})
	return sd
}

func (sd *DataStorageSerializedData) Next() (uint32, uint32) {
	return seriesDataSerializedDataNext(sd.serializedData)
}

type DataStorageSerializedDataIteratorControlBlock struct {
	Timestamp        int64
	Value            float64
	remainingSamples uint8
}

type DataStorageSerializedDataIterator struct {
	DataStorageSerializedDataIteratorControlBlock
	cppInternalData [unsafe.Sizeof(CppSerializedDataIterator{}) - unsafe.Sizeof(DataStorageSerializedDataIteratorControlBlock{})]byte
}

func NewDataStorageSerializedDataIterator(serializedData *DataStorageSerializedData, chunkRef uint32) DataStorageSerializedDataIterator {
	it := DataStorageSerializedDataIterator{}
	seriesDataSerializedDataIteratorCtor(&it, serializedData.serializedData, chunkRef)
	return it
}

func (it *DataStorageSerializedDataIterator) Next() {
	seriesDataSerializedDataIteratorNext(it)
}

func (it *DataStorageSerializedDataIterator) Seek(timestamp int64) {
	seriesDataSerializedDataIteratorSeek(it, timestamp)
}

func (it *DataStorageSerializedDataIterator) Reset(serializedData *DataStorageSerializedData, chunkRef uint32) {
	seriesDataSerializedDataIteratorReset(it, serializedData.serializedData, chunkRef)
}

func (it *DataStorageSerializedDataIterator) HasData() bool {
	return it.remainingSamples != 0
}

// UnloadedDataLoader is Go wrapper around series_data::Loader.
type UnloadedDataLoader struct {
	loader uintptr
	ds     *HeadDataStorage
}

func (loader *UnloadedDataLoader) Load(snapshot []byte, isLast bool) {
	seriesDataUnloadedDataLoaderLoad(loader.loader, snapshot, isLast)
	runtime.KeepAlive(loader)
}

func (ds *HeadDataStorage) CreateLoader(queriers []uintptr) *UnloadedDataLoader {
	result := &UnloadedDataLoader{
		loader: seriesDataUnloadedDataLoaderCtor(ds.dataStorage, queriers),
		ds:     ds,
	}
	runtime.KeepAlive(queriers)

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

func (ds *HeadDataStorage) CreateRevertableLoader(lss *LabelSetStorage, lsIdBatchSize uint32) *UnloadedDataRevertableLoader {
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
