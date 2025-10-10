package cppbridge

import (
	"math"
	"runtime"
	"unsafe"
)

const (
	NormalNaN uint64 = 0x7ff8000000000001

	StaleNaN uint64 = 0x7ff0000000000002
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

type Sample struct {
	Timestamp int64
	Value     float64
}

// HeadDataStorage is Go wrapper around series_data::Data_storage.
type HeadDataStorage struct {
	dataStorage       uintptr
	gcDestroyDetector *uint64
}

// NewHeadDataStorage - constructor.
func NewHeadDataStorage() *HeadDataStorage {
	ds := &HeadDataStorage{
		dataStorage:       seriesDataDataStorageCtor(),
		gcDestroyDetector: &gcDestroyDetector,
	}

	runtime.SetFinalizer(ds, func(ds *HeadDataStorage) {
		seriesDataDataStorageDtor(ds.dataStorage)
	})

	return ds
}

// Reset - resets data storage.
func (ds *HeadDataStorage) Reset() {
	seriesDataDataStorageReset(ds.dataStorage)
}

func (ds *HeadDataStorage) TimeInterval() TimeInterval {
	res := seriesDataDataStorageTimeInterval(ds.dataStorage)
	runtime.KeepAlive(ds)
	return res
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
func (e *HeadEncoder) EncodeInnerSeriesSlice(innerSeriesSlice []*InnerSeries) {
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

	lss              *LabelSetStorage
	dataStorage      *HeadDataStorage
	serializedChunks *HeadDataStorageSerializedChunks
}

func NewChunkRecoder(lss *LabelSetStorage, lsIdBatchSize uint32, dataStorage *HeadDataStorage, timeInterval TimeInterval) *ChunkRecoder {
	return initializeChunkRecoder(lss, dataStorage, nil, seriesDataChunkRecoderCtor(lss.Pointer(), lsIdBatchSize, dataStorage.dataStorage, timeInterval))
}

func NewSerializedChunkRecoder(serializedChunks *HeadDataStorageSerializedChunks, timeInterval TimeInterval) *ChunkRecoder {
	return initializeChunkRecoder(nil, nil, serializedChunks, seriesDataSerializedChunkRecoderCtor(serializedChunks.Data(), timeInterval))
}

func initializeChunkRecoder(
	lss *LabelSetStorage,
	dataStorage *HeadDataStorage,
	serializedChunks *HeadDataStorageSerializedChunks,
	recoder uintptr,
) *ChunkRecoder {
	chunkRecoder := &ChunkRecoder{
		recoder:          recoder,
		lss:              lss,
		dataStorage:      dataStorage,
		serializedChunks: serializedChunks,
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
	return *(*uint32)(unsafe.Pointer(&cm[0])) // #nosec G103 // it's meant to be that way
}

func (r *HeadDataStorageSerializedChunks) NumberOfChunks() int {
	if len(r.data) == 0 {
		return 0
	}

	return int(*(*int32)(unsafe.Pointer(&r.data[0]))) // #nosec G103 // it's meant to be that way
}

func (r *HeadDataStorageSerializedChunks) Len() int {
	return len(r.data)
}

func (r *HeadDataStorageSerializedChunks) Data() []byte {
	return r.data
}

type HeadDataStorageSerializedChunkIndex struct {
	m map[uint32][]int
}

func (r *HeadDataStorageSerializedChunks) MakeIndex() HeadDataStorageSerializedChunkIndex {
	n := r.NumberOfChunks()
	m := make(map[uint32][]int, n)
	offset := Uint32Size
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

func (ds *HeadDataStorage) Query(query HeadDataStorageQuery) (*HeadDataStorageSerializedChunks, DataStorageQueryResult) {
	serializedChunks := &HeadDataStorageSerializedChunks{}
	result := seriesDataDataStorageQuery(ds.dataStorage, query, &serializedChunks.data)
	runtime.KeepAlive(ds)
	runtime.SetFinalizer(serializedChunks, func(sc *HeadDataStorageSerializedChunks) {
		freeBytes(sc.data)
	})
	return serializedChunks, result
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

type HeadDataStorageDeserializer struct {
	deserializer     uintptr
	serializedChunks *HeadDataStorageSerializedChunks
}

func NewHeadDataStorageDeserializer(serializedChunks *HeadDataStorageSerializedChunks) *HeadDataStorageDeserializer {
	d := &HeadDataStorageDeserializer{
		deserializer:     seriesDataDeserializerCtor(serializedChunks.Data()),
		serializedChunks: serializedChunks,
	}
	runtime.SetFinalizer(d, func(d *HeadDataStorageDeserializer) {
		seriesDataDeserializerDtor(d.deserializer)
	})
	return d
}

func (d *HeadDataStorageDeserializer) CreateDecodeIterator(chunkMetadata HeadDataStorageSerializedChunkMetadata) *HeadDataStorageDecodeIterator {
	decodeIterator := &HeadDataStorageDecodeIterator{
		decodeIterator: seriesDataDeserializerCreateDecodeIterator(d.deserializer, chunkMetadata[:]),
	}

	runtime.SetFinalizer(decodeIterator, func(decodeIterator *HeadDataStorageDecodeIterator) {
		seriesDataDecodeIteratorDtor(decodeIterator.decodeIterator)
	})

	return decodeIterator
}

type HeadDataStorageDecodeIterator struct {
	decodeIterator uintptr
	started        bool
	finished       bool
}

func (i *HeadDataStorageDecodeIterator) Next() bool {
	if !i.started {
		i.started = true
		return true
	}

	if i.finished {
		return false
	}

	i.finished = !seriesDataDecodeIteratorNext(i.decodeIterator)
	return !i.finished
}

func (i *HeadDataStorageDecodeIterator) Sample() (int64, float64) {
	return seriesDataDecodeIteratorSample(i.decodeIterator)
}
