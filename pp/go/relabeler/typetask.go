package relabeler

//
// TypeTask
//

// TypeTask type of task.
type TypeTask uint8

const (
	// Unknown type of task.
	Unknown TypeTask = iota

	// HeadInputRelabeling type of task.
	HeadInputRelabeling
	// HeadAppendRelabelerSeries type of task.
	HeadAppendRelabelerSeries
	// HeadUpdateRelabelerState type of task.
	HeadUpdateRelabelerState

	// HeadWriteMetrics type of task.
	HeadWriteMetrics
	// HeadStatusType type of task.
	HeadStatusType
	// HeadCopyAddedSeries type of task.
	HeadCopyAddedSeries

	// DataStorageAppendInnerSeries type of task.
	DataStorageAppendInnerSeries
	// DataStorageMergeOutOfOrderChunks type of task.
	DataStorageMergeOutOfOrderChunks

	// BlockWrite type of task.
	BlockWrite

	// WalCommit type of task.
	WalCommit
	// WalFlush type of task.
	WalFlush
	// WalWrite type of task.
	WalWrite

	// ChunkQuerierSelect type of task.
	ChunkQuerierSelect
	// ChunkQuerierLabelValues type of task.
	ChunkQuerierLabelValues
	// ChunkQuerierLabelNames type of task.
	ChunkQuerierLabelNames

	// QuerierSelectInstant type of task.
	QuerierSelectInstant
	// QuerierSelectRange type of task.
	QuerierSelectRange
	// QuerierLabelValues type of task.
	QuerierLabelValues
	// QuerierLabelNames type of task.
	QuerierLabelNames

	// DistributorWriteMetrics type of task.
	DistributorWriteMetrics
	// DistributorOutputRelabeling type of task.
	DistributorOutputRelabeling
	// DistributorUpdateRelabelerState type of task.
	DistributorUpdateRelabelerState

	// Other type of task.
	Other
)
