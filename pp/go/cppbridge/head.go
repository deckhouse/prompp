package cppbridge

import (
	"math"
	"runtime"
	"unsafe"
)

const (
	// NormalNaN represents a normal NaN as uint64.
	NormalNaN uint64 = 0x7ff8000000000001

	// StaleNaN represents a stale NaN as uint64.
	StaleNaN uint64 = 0x7ff0000000000002
)

// IsStaleNaN returns true if the value is a stale NaN.
func IsStaleNaN(v float64) bool {
	return math.Float64bits(v) == StaleNaN
}

const (
	// MaxPointsInChunk the maximum number of points in a chunk.
	MaxPointsInChunk = 240
	// Uint32Size the size of a uint32.
	Uint32Size = 4
	// SerializedChunkMetadataSize the size of a serialized chunk metadata.
	SerializedChunkMetadataSize = 13

	// DataStorageQueryStatusSuccess the status when the query is successful.
	DataStorageQueryStatusSuccess uint8 = 0
	// DataStorageQueryStatusNeedDataLoad the status when the query needs data load.
	DataStorageQueryStatusNeedDataLoad uint8 = 1
)

// TimeInterval represents a time interval.
type TimeInterval struct {
	MinT int64
	MaxT int64
}

// NewInvalidTimeInterval creates a new invalid [TimeInterval].
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

// IsInvalid returns true if the time interval is invalid.
func (t *TimeInterval) IsInvalid() bool {
	return t.MinT == math.MaxInt64 && t.MaxT == math.MinInt64
}

// Sample represents a points in a time series.
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
func (e *HeadEncoder) EncodeInnerSeriesSlice(innerSeriesSlice []InnerSeries) {
	seriesDataEncoderEncodeInnerSeriesSlice(e.encoder, innerSeriesSlice)
}

// MergeOutOfOrderChunks merges out of order chunks.
func (e *HeadEncoder) MergeOutOfOrderChunks() {
	seriesDataEncoderMergeOutOfOrderChunks(e.encoder)
}

// RecodedChunk represents a recoded chunk.
type RecodedChunk struct {
	TimeInterval
	SeriesId     uint32
	SamplesCount uint8
	HasMoreData  bool
	ChunkData    []byte
}

const (
	// InvalidSeriesId represents an invalid series ID.
	InvalidSeriesId = math.MaxUint32

	// UnlimitedLsIdBatchSize represents an unlimited LSS ID batch size.
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

// NewChunkRecoder creates a new [ChunkRecoder] for non-serialized chunks.
func NewChunkRecoder(
	lss *LabelSetStorage,
	lsIdBatchSize uint32,
	dataStorage *DataStorage,
	timeInterval TimeInterval,
) *ChunkRecoder {
	return initializeChunkRecoder(
		lss,
		dataStorage,
		nil,
		seriesDataChunkRecoderCtor(lss.Pointer(), lsIdBatchSize, dataStorage.dataStorage, timeInterval),
	)
}

// NewSerializedChunkRecoder creates a new [ChunkRecoder] for serialized chunks.
func NewSerializedChunkRecoder(serializedData *DataStorageSerializedData, timeInterval TimeInterval) *ChunkRecoder {
	return initializeChunkRecoder(
		nil,
		nil,
		serializedData,
		seriesDataSerializedChunkRecoderCtor(serializedData, timeInterval),
	)
}

// initializeChunkRecoder initializes a new [ChunkRecoder].
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

// RecodeNextChunk recodes the next chunk.
func (recoder *ChunkRecoder) RecodeNextChunk() RecodedChunk {
	seriesDataChunkRecoderRecodeNextChunk(recoder.recoder, &recoder.recodedChunk)
	return recoder.recodedChunk
}

// NextBatch returns true if there are more batches to recode.
func (recoder *ChunkRecoder) NextBatch() bool {
	result := seriesDataChunkRecoderNextBatch(recoder.recoder)
	runtime.KeepAlive(recoder)
	return result
}

// getSeriesIDFromBytes returns the series ID from a given byte slice.
func getSeriesIDFromBytes(data []byte) uint32 {
	return *(*uint32)(unsafe.Pointer(&data[0])) // #nosec G103 // it's meant to be that way
}

// HeadDataStorageSerializedChunks represents a serialized chunks.
type HeadDataStorageSerializedChunks struct {
	data []byte
}

// HeadDataStorageSerializedChunkMetadata represents a serialized chunk metadata.
type HeadDataStorageSerializedChunkMetadata [SerializedChunkMetadataSize]byte

// SeriesID returns the series ID for a given chunk metadata.
func (cm HeadDataStorageSerializedChunkMetadata) SeriesID() uint32 {
	return *(*uint32)(unsafe.Pointer(&cm[0])) // #nosec G103 // it's meant to be that way
}

// NumberOfChunks returns the number of chunks in the serialized chunks.
func (r *HeadDataStorageSerializedChunks) NumberOfChunks() int {
	if len(r.data) == 0 {
		return 0
	}

	return int(*(*int32)(unsafe.Pointer(&r.data[0]))) // #nosec G103 // it's meant to be that way
}

// Len returns the number of serialized chunks.
func (r *HeadDataStorageSerializedChunks) Len() int {
	return len(r.data)
}

// Data returns the data of the serialized chunks.
func (r *HeadDataStorageSerializedChunks) Data() []byte {
	return r.data
}

// Metadata returns the metadata for a given chunk index.
func (r *HeadDataStorageSerializedChunks) Metadata(chunkIndex int) HeadDataStorageSerializedChunkMetadata {
	offset := Uint32Size + chunkIndex*SerializedChunkMetadataSize
	return HeadDataStorageSerializedChunkMetadata(r.data[offset : offset+SerializedChunkMetadataSize])
}

// HeadDataStorageSerializedChunkIndex represents a serialized chunk index.
type HeadDataStorageSerializedChunkIndex struct {
	m map[uint32][]int
}

// MakeIndex creates a new [HeadDataStorageSerializedChunkIndex].
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

// Has returns true if the index has a given series ID.
func (i HeadDataStorageSerializedChunkIndex) Has(seriesID uint32) bool {
	return len(i.m[seriesID]) > 0
}

// Len returns the number of series in the index.
func (i HeadDataStorageSerializedChunkIndex) Len() int {
	return len(i.m)
}

// Chunks returns the chunks for a given series ID.
func (i HeadDataStorageSerializedChunkIndex) Chunks(
	r *HeadDataStorageSerializedChunks,
	seriesID uint32,
) []HeadDataStorageSerializedChunkMetadata {
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

type DataStorageSerializedDataIteratorControlBlock struct {
	decoderVariant   uint64
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
	ds     *DataStorage
}

// Load loads the data from the snapshot.
func (loader *UnloadedDataLoader) Load(snapshot []byte, isLast bool) {
	seriesDataUnloadedDataLoaderLoad(loader.loader, snapshot, isLast)
	runtime.KeepAlive(loader)
}

// CreateLoader creates a new [UnloadedDataLoader].
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

// NextBatch returns true if there are more batches to load.
func (loader *UnloadedDataRevertableLoader) NextBatch() bool {
	result := seriesDataUnloadedDataRevertableLoaderNextBatch(loader.loader)
	runtime.KeepAlive(loader)
	return result
}

// CreateRevertableLoader creates a new [UnloadedDataRevertableLoader].
func (ds *DataStorage) CreateRevertableLoader(
	lss *LabelSetStorage,
	lsIdBatchSize uint32,
) *UnloadedDataRevertableLoader {
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
