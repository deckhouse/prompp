package storage

import (
	"context"
)

//
// Batcher
//

// Batcher provides [BatchStorage] for transaction append and read operations.
type Batcher interface {
	// BatchStorage creates a new [BatchStorage] for transaction append and read operations.
	BatchStorage() BatchStorage
}

//
// BatchStorage
//

// BatchStorage accumulates data from the appendices and adds it to the repository on the commit.
// It can read as [Querier] the added data.
type BatchStorage interface {
	// Commit adds aggregated series from [TransactionHead] to the [Head].
	Commit(ctx context.Context) error

	Appendable
	Queryable
}
