package relabeler

//
// TypeTask
//

// TypeTask type of task.
type TypeTask uint8

const (
	// Unknown type of task.
	Unknown TypeTask = iota

	// LSSHeadInputRelabeling type of task.
	LSSHeadInputRelabeling
	// LSSHeadAppendRelabelerSeries type of task.
	LSSHeadAppendRelabelerSeries

	// DataStorageAppendInnerSeries type of task.
	DataStorageAppendInnerSeries
	// DataStorageMergeOutOfOrderChunks type of task.
	DataStorageMergeOutOfOrderChunks

	// WalCommit type of task.
	WalCommit
	// WalFlush type of task.
	WalFlush
	// WalWrite type of task.
	WalWrite

	// LSSHeadCopyAddedSeries type of task.
	LSSHeadCopyAddedSeries

	// BlockWrite type of task.
	BlockWrite

	// DistributorOutputRelabeling type of task.
	DistributorOutputRelabeling
	// DistributorUpdateRelabelerState type of task.
	DistributorUpdateRelabelerState

	// dataStorageMarker dividing marker, not used
	dataStorageMarker

	// LSSHeadAllocatedMemory type of task.
	LSSHeadAllocatedMemory
	// DataStorageHeadAllocatedMemory type of task.
	DataStorageHeadAllocatedMemory
	// DataStorageHeadStatus type of task.
	DataStorageHeadStatus
	// LSSHeadStatus type of task.
	LSSHeadStatus

	// LSSQueryChunkQuerierSelect type of task.
	LSSQueryChunkQuerierSelect
	// DataStorageQueryChunkQuerierSelect type of task.
	DataStorageQueryChunkQuerierSelect
	// LSSLabelValuesChunkQuerier type of task.
	LSSLabelValuesChunkQuerier
	// LSSLabelNamesChunkQuerier type of task.
	LSSLabelNamesChunkQuerier

	// LSSQueryQuerierSelectInstant type of task.
	LSSQueryQuerierSelectInstant
	// DataStorageQueryQuerierSelectInstant type of task.
	DataStorageQueryQuerierSelectInstant
	// LSSQueryQuerierSelectRange type of task.
	LSSQueryQuerierSelectRange
	// DataStorageQueryQuerierSelectRange type of task.
	DataStorageQueryQuerierSelectRange
	// LSSLabelValuesQuerier type of task.
	LSSLabelValuesQuerier
	// LSSLabelNamesQuerier type of task.
	LSSLabelNamesQuerier
)

// ForLSS indicates whether the type for LSS.
func (i TypeTask) ForLSS() bool {
	return i < dataStorageMarker
}

// const (
// 	// UnknownTask type of task.
// 	UnknownTask TypeTask = iota

// 	// LSSHeadInputRelabeling type of task.
// 	LSSHeadInputRelabeling
// 	// LSSHeadAppendRelabelerSeries type of task.
// 	LSSHeadAppendRelabelerSeries

// 	// WalCommit type of task.
// 	WalCommit
// 	// WalFlush type of task.
// 	WalFlush
// 	// WalWrite type of task.
// 	WalWrite

// 	// LSSHeadCopyAddedSeries type of task.
// 	LSSHeadCopyAddedSeries

// 	// DistributorOutputRelabeling type of task.
// 	DistributorOutputRelabeling
// 	// DistributorUpdateRelabelerState type of task.
// 	DistributorUpdateRelabelerState

// 	// LSSHeadAllocatedMemory type of task.
// 	LSSHeadAllocatedMemory

// 	// LSSHeadStatus type of task.
// 	LSSHeadStatus

// 	// LSSQueryChunkQuerierSelect type of task.
// 	LSSQueryChunkQuerierSelect
// 	// LSSLabelValuesChunkQuerier type of task.
// 	LSSLabelValuesChunkQuerier
// 	// LSSLabelNamesChunkQuerier type of task.
// 	LSSLabelNamesChunkQuerier

// 	// LSSQueryQuerierSelectInstant type of task.
// 	LSSQueryQuerierSelectInstant
// 	// LSSQueryQuerierSelectRange type of task.
// 	LSSQueryQuerierSelectRange
// 	// LSSLabelValuesQuerier type of task.
// 	LSSLabelValuesQuerier
// 	// LSSLabelNamesQuerier type of task.
// 	LSSLabelNamesQuerier

// 	// DataStorage
// 	// dataStorageMarker dividing marker, not used
// 	dataStorageMarker
// 	//

// 	// DataStorageAppendInnerSeries type of task.
// 	DataStorageAppendInnerSeries
// 	// DataStorageMergeOutOfOrderChunks type of task.
// 	DataStorageMergeOutOfOrderChunks

// 	// DataStorageHeadAllocatedMemory type of task.
// 	DataStorageHeadAllocatedMemory

// 	// DataStorageHeadStatus type of task.
// 	DataStorageHeadStatus

// 	// DataStorageQueryChunkQuerierSelect type of task.
// 	DataStorageQueryChunkQuerierSelect

// 	// DataStorageQueryQuerierSelectInstant type of task.
// 	DataStorageQueryQuerierSelectInstant
// 	// DataStorageQueryQuerierSelectRange type of task.
// 	DataStorageQueryQuerierSelectRange

// 	// Read Only

// 	// BlockWrite type of task.
// 	BlockWrite
// )

// // ForLSS indicates whether the type for LSS.
// func (i TypeTask) ForLSS() bool {
// 	return i < dataStorageMarker
// }
