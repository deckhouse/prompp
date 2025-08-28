package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
)

//
// HeadsCatalog
//

// HeadsCatalog of current head records.
type HeadsCatalog interface {
	// Create creates new [Record] and write to [Log].
	Create(numberOfShards uint16) (*catalog.Record, error)

	// Delete record by ID.
	Delete(id string) error

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

//
// Builder
//

// Builder building new [Head] from factory with parameters.
type Builder[T any, THead Head[T]] struct {
	catalog    HeadsCatalog
	dir        string
	generation uint64
	headCtor   func(
		id, headDir string,
		releaseHeadFn func(),
		setLastAppendedSegmentID func(segmentID uint32),
		generation uint64,
		maxSegmentSize uint32,
		numberOfShards uint16,
		registerer prometheus.Registerer,
	) (THead, error)
	maxSegmentSize uint32
	registerer     prometheus.Registerer
}

// NewBuilder init new [Builder].
func NewBuilder[T any, THead Head[T]](
	hcatalog HeadsCatalog,
	dir string,
	generation uint64,
	headCtor func(
		id, headDir string,
		releaseHeadFn func(),
		setLastAppendedSegmentID func(segmentID uint32),
		generation uint64,
		maxSegmentSize uint32,
		numberOfShards uint16,
		registerer prometheus.Registerer,
	) (THead, error),
	maxSegmentSize uint32,
	registerer prometheus.Registerer,
) *Builder[T, THead] {
	return &Builder[T, THead]{
		catalog:        hcatalog,
		dir:            dir,
		generation:     generation,
		headCtor:       headCtor,
		maxSegmentSize: maxSegmentSize,
		registerer:     registerer,
	}
}

// Build new [Head].
func (b *Builder[T, THead]) Build(numberOfShards uint16) (THead, error) {
	headRecord, err := b.catalog.Create(numberOfShards)
	if err != nil {
		return nil, err
	}

	headDir := filepath.Join(b.dir, headRecord.ID())
	//revive:disable-next-line:add-constant // this is already a constant
	if err = os.Mkdir(headDir, 0o777); err != nil { //nolint:gosec // need this permissions
		return nil, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, os.RemoveAll(headDir))
		}
	}()

	h, err := b.headCtor(
		headRecord.ID(),
		headDir,
		headRecord.Acquire(),
		headRecord.SetLastAppendedSegmentID,
		b.generation,
		b.maxSegmentSize,
		numberOfShards,
		b.registerer,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create head: %w", err)
	}

	b.generation++

	return h, nil
}
