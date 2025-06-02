package relabeler

//
// TypeTask
//

// TypeTask type of task.
type TypeTask uint8

// const (
// 	// Unknown type of task.
// 	Unknown TypeTask = iota

// 	// HeadInputRelabeling type of task.
// 	HeadInputRelabeling
// 	// HeadAppendRelabelerSeries type of task.
// 	HeadAppendRelabelerSeries

// 	// DataStorageAppendInnerSeries type of task.
// 	DataStorageAppendInnerSeries
// 	// DataStorageMergeOutOfOrderChunks type of task.
// 	DataStorageMergeOutOfOrderChunks

// 	// WalCommit type of task.
// 	WalCommit
// 	// WalFlush type of task.
// 	WalFlush
// 	// WalWrite type of task.
// 	WalWrite

// 	// HeadCopyAddedSeries type of task.
// 	HeadCopyAddedSeries

// 	// BlockWrite type of task.
// 	BlockWrite

// 	// DistributorOutputRelabeling type of task.
// 	DistributorOutputRelabeling
// 	// DistributorUpdateRelabelerState type of task.
// 	DistributorUpdateRelabelerState

// 	// exclusiveMarker dividing marker, not used
// 	exclusiveMarker

// 	// HeadUpdateRelabelerState type of task.
// 	HeadUpdateRelabelerState

// 	// HeadLSSAllocatedMemory type of task.
// 	HeadLSSAllocatedMemory
// 	// HeadDataStorageAllocatedMemory type of task.
// 	HeadDataStorageAllocatedMemory
// 	// DataStorageHeadStatus type of task.
// 	DataStorageHeadStatus
// 	// LSSHeadStatus type of task.
// 	LSSHeadStatus

// 	// ChunkQuerierSelectLSSQuery type of task.
// 	ChunkQuerierSelectLSSQuery
// 	// ChunkQuerierSelectDataStorageQuery type of task.
// 	ChunkQuerierSelectDataStorageQuery
// 	// ChunkQuerierLabelValues type of task.
// 	ChunkQuerierLabelValues
// 	// ChunkQuerierLabelNames type of task.
// 	ChunkQuerierLabelNames

// 	// QuerierSelectInstantLSSQuery type of task.
// 	QuerierSelectInstantLSSQuery
// 	// QuerierSelectInstantDataStorageQuery type of task.
// 	QuerierSelectInstantDataStorageQuery
// 	// QuerierSelectRangeLSSQuery type of task.
// 	QuerierSelectRangeLSSQuery
// 	// QuerierSelectRangeDataStorageQuery type of task.
// 	QuerierSelectRangeDataStorageQuery
// 	// QuerierLabelValues type of task.
// 	QuerierLabelValues
// 	// QuerierLabelNames type of task.
// 	QuerierLabelNames
// )

// // IsExclusive indicates whether the type is exclusive.
// func (i TypeTask) IsExclusive() bool {
// 	return i < exclusiveMarker
// }

const (
	// UnknownTask type of task.
	UnknownTask TypeTask = iota

	// LSSHeadInputRelabeling type of task.
	LSSHeadInputRelabeling
	// LSSHeadAppendRelabelerSeries type of task.
	LSSHeadAppendRelabelerSeries

	// WalCommit type of task.
	WalCommit
	// WalFlush type of task.
	WalFlush
	// WalWrite type of task.
	WalWrite

	// LSSHeadCopyAddedSeries type of task.
	LSSHeadCopyAddedSeries

	// DistributorOutputRelabeling type of task.
	DistributorOutputRelabeling
	// DistributorUpdateRelabelerState type of task.
	DistributorUpdateRelabelerState

	// LSSHeadAllocatedMemory type of task.
	LSSHeadAllocatedMemory

	// LSSHeadStatus type of task.
	LSSHeadStatus

	// LSSQueryChunkQuerierSelect type of task.
	LSSQueryChunkQuerierSelect
	// LSSLabelValuesChunkQuerier type of task.
	LSSLabelValuesChunkQuerier
	// LSSLabelNamesChunkQuerier type of task.
	LSSLabelNamesChunkQuerier

	// LSSQueryQuerierSelectInstant type of task.
	LSSQueryQuerierSelectInstant
	// LSSQueryQuerierSelectRange type of task.
	LSSQueryQuerierSelectRange
	// LSSLabelValuesQuerier type of task.
	LSSLabelValuesQuerier
	// LSSLabelNamesQuerier type of task.
	LSSLabelNamesQuerier

	// DataStorage
	// dataStorageMarker dividing marker, not used
	dataStorageMarker
	//

	// DataStorageAppendInnerSeries type of task.
	DataStorageAppendInnerSeries
	// DataStorageMergeOutOfOrderChunks type of task.
	DataStorageMergeOutOfOrderChunks

	// DataStorageHeadAllocatedMemory type of task.
	DataStorageHeadAllocatedMemory

	// DataStorageHeadStatus type of task.
	DataStorageHeadStatus

	// DataStorageQueryChunkQuerierSelect type of task.
	DataStorageQueryChunkQuerierSelect

	// DataStorageQueryQuerierSelectInstant type of task.
	DataStorageQueryQuerierSelectInstant
	// DataStorageQueryQuerierSelectRange type of task.
	DataStorageQueryQuerierSelectRange

	// Read Only

	// BlockWrite type of task.
	BlockWrite

	// HeadUpdateRelabelerState type of task.
	HeadUpdateRelabelerState
)

// ForLSS indicates whether the type for LSS.
func (i TypeTask) ForLSS() bool {
	return i < dataStorageMarker
}
