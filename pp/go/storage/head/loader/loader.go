package loader

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
)

//
// HeadsCatalog
//

// HeadsCatalog of current head records.
type HeadsCatalog interface {
	// List returns slice of records with filter and sort.
	List(filterFn func(record *catalog.Record) bool, sortLess func(lhs, rhs *catalog.Record) bool) []*catalog.Record
}

//
// Head
//

// Head the minimum required Head implementation for a container.
type Head[T any] interface {
	// for use as a pointer
	*T
}

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
