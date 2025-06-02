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

	// HeadInputRelabeling type of task.
	HeadInputRelabeling
	// HeadAppendRelabelerSeries type of task.
	HeadAppendRelabelerSeries

	// WalCommit type of task.
	WalCommit
	// WalFlush type of task.
	WalFlush
	// WalWrite type of task.
	WalWrite

	// HeadCopyAddedSeries type of task.
	HeadCopyAddedSeries

	// DistributorOutputRelabeling type of task.
	DistributorOutputRelabeling
	// DistributorUpdateRelabelerState type of task.
	DistributorUpdateRelabelerState

	// HeadLSSAllocatedMemory type of task.
	HeadLSSAllocatedMemory

	// LSSHeadStatus type of task.
	LSSHeadStatus

	// ChunkQuerierSelectLSSQuery type of task.
	ChunkQuerierSelectLSSQuery
	// ChunkQuerierLabelValues type of task.
	ChunkQuerierLabelValues
	// ChunkQuerierLabelNames type of task.
	ChunkQuerierLabelNames

	// QuerierSelectInstantLSSQuery type of task.
	QuerierSelectInstantLSSQuery
	// QuerierSelectRangeLSSQuery type of task.
	QuerierSelectRangeLSSQuery
	// QuerierLabelValues type of task.
	QuerierLabelValues
	// QuerierLabelNames type of task.
	QuerierLabelNames

	// DataStorage
	// dataStorageMarker dividing marker, not used
	dataStorageMarker
	//

	// DataStorageAppendInnerSeries type of task.
	DataStorageAppendInnerSeries
	// DataStorageMergeOutOfOrderChunks type of task.
	DataStorageMergeOutOfOrderChunks

	// HeadDataStorageAllocatedMemory type of task.
	HeadDataStorageAllocatedMemory

	// DataStorageHeadStatus type of task.
	DataStorageHeadStatus

	// ChunkQuerierSelectDataStorageQuery type of task.
	ChunkQuerierSelectDataStorageQuery

	// QuerierSelectInstantDataStorageQuery type of task.
	QuerierSelectInstantDataStorageQuery
	// QuerierSelectRangeDataStorageQuery type of task.
	QuerierSelectRangeDataStorageQuery

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
