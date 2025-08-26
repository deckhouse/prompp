package builder

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
	*T
}

//
// Builder
//

type Builder[TShard any, TGoroutineShard any, T any, THead Head[T]] struct {
	catalog     HeadsCatalog
	dir         string
	generation  uint64
	headFactory func(
		id string,
		releaseHeadFn func(),
		generation uint64,
		numberOfShards uint16,
		registerer prometheus.Registerer,
	) (THead, error)
	registerer prometheus.Registerer
}

func (b *Builder[TShard, TGoroutineShard, T, THead]) Build(numberOfShards uint16) (THead, error) {
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

	h, err := b.headFactory(
		headRecord.ID(),
		headRecord.Acquire(),
		b.generation,
		numberOfShards,
		b.registerer,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create head: %w", err)
	}

	b.generation++

	return h, nil
}
