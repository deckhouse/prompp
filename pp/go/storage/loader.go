package storage

import (
	"github.com/prometheus/client_golang/prometheus"
)

type Loader[T any, THead Head[T]] struct {
	catalog  HeadsCatalog
	dir      string
	headCtor func(
		id, headDir string,
		releaseHeadFn func(),
		setLastAppendedSegmentID func(segmentID uint32),
		generation uint64,
		maxSegmentSize uint32,
		numberOfShards uint16,
		registerer prometheus.Registerer,
	) (THead, error)
	uploadHead func(
		id, headDir string,
		releaseHeadFn func(),
		setLastAppendedSegmentID func(segmentID uint32),
		generation uint64,
		maxSegmentSize uint32,
		numberOfShards uint16,
		registerer prometheus.Registerer,
	) (THead, uint32, bool)
	maxSegmentSize uint32
	registerer     prometheus.Registerer

	// TODO ?
	// generation uint64
}
