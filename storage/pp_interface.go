package storage

import (
	"context"
)

// BatchAppendable provides [BatchAppender] for transaction append operations.
type BatchAppendable interface {
	// BatchAppender creates a new [BatchAppender] for transaction append operations.
	BatchAppender(ctx context.Context) BatchAppender
}

// BatchAppender accumulates data from the appendices and adds it to the repository on the commit.
type BatchAppender interface {
	// Appender creates a new [Appender] for appending time series data to [TransactionHead].
	Appender(ctx context.Context) Appender

	// Commit adds aggregated series from [TransactionHead] to the [Head].
	Commit() error
}
