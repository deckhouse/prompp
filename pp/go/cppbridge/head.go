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

// HeadEncoder is Go wrapper around series_data::Encoder.
type HeadEncoder struct {
	encoder     uintptr
	dataStorage *DataStorage
}

// NewHeadEncoderWithDataStorage - constructor.
func NewHeadEncoderWithDataStorage(dataStorage *DataStorage) *HeadEncoder {
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
	return NewHeadEncoderWithDataStorage(NewDataStorage())
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

	lss            *LabelSetStorage
	dataStorage    *DataStorage
	serializedData *DataStorageSerializedData
}

func NewChunkRecoder(lss *LabelSetStorage, lsIdBatchSize uint32, dataStorage *DataStorage, timeInterval TimeInterval) *ChunkRecoder {
	return initializeChunkRecoder(lss, dataStorage, nil, seriesDataChunkRecoderCtor(lss.Pointer(), lsIdBatchSize, dataStorage.dataStorage, timeInterval))
}

func NewSerializedChunkRecoder(serializedData *DataStorageSerializedData, timeInterval TimeInterval) *ChunkRecoder {
	return initializeChunkRecoder(nil, nil, serializedData, seriesDataSerializedChunkRecoderCtor(serializedData, timeInterval))
}

func initializeChunkRecoder(
	lss *LabelSetStorage,
	dataStorage *DataStorage,
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

type DataStorageSerializedDataIterator struct {
	iterator uintptr
}

func NewDataStorageSerializedDataIterator(serializedData *DataStorageSerializedData, chunkRef uint32) DataStorageSerializedDataIterator {
	return DataStorageSerializedDataIterator{iterator: seriesDataSerializedDataIteratorCtor(serializedData.serializedData, chunkRef)}
}

func (it DataStorageSerializedDataIterator) Next(result *SerializedDataIteratorIterationResult) {
	seriesDataSerializedDataIteratorNext(it.iterator, result)
}

func (it DataStorageSerializedDataIterator) Seek(timestamp int64, result *SerializedDataIteratorIterationResult) {
	seriesDataSerializedDataIteratorSeek(it.iterator, timestamp, result)
}

func (it DataStorageSerializedDataIterator) Reset(serializedData *DataStorageSerializedData, chunkRef uint32) {
	seriesDataSerializedDataIteratorReset(serializedData.serializedData, it.iterator, chunkRef)
}

func (it DataStorageSerializedDataIterator) Destroy() {
	seriesDataSerializedDataIteratorDtor(it.iterator)
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
