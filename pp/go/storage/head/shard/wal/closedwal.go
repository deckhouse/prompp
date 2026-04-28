package wal

import (
	"errors"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

// ErrWalClosed is returned by data-handling methods of [ClosedWal]
// (Write/Commit/Flush/Sync) to signal that the WAL has already been closed.
// Unlike [NoopWal], which is legitimately used for shards that are not backed
// by a real WAL, [ClosedWal] is a sentinel for shards whose WAL was
// deliberately closed (e.g. after rotation); silently dropping data there
// would hide bugs, so we surface it as an error instead.
//
// Callers that may legitimately hit a closed WAL (e.g. Persistener re-flushing
// an already-flushed rotated head) must tolerate this error via
// [errors.Is](err, ErrWalClosed).
var ErrWalClosed = errors.New("wal is closed")

// ClosedWal marks a WAL that has been deliberately closed. Close is an
// idempotent no-op (io.Closer convention) and CurrentSize returns 0, but
// Write/Commit/Flush/Sync return [ErrWalClosed] so accidental use-after-close
// is noisy rather than silent.
type ClosedWal struct{}

// Close implementation of [ClosedWal], idempotent no-op.
func (ClosedWal) Close() error {
	return nil
}

// Commit implementation of [ClosedWal], returns [ErrWalClosed].
func (ClosedWal) Commit() error {
	return ErrWalClosed
}

// CurrentSize implementation of [ClosedWal], always returns 0.
func (ClosedWal) CurrentSize() int64 {
	return 0
}

// Flush implementation of [ClosedWal], returns [ErrWalClosed].
func (ClosedWal) Flush() error {
	return ErrWalClosed
}

// Sync implementation of [ClosedWal], returns [ErrWalClosed].
func (ClosedWal) Sync() error {
	return ErrWalClosed
}

// Write implementation of [ClosedWal], returns [ErrWalClosed].
func (ClosedWal) Write([]cppbridge.InnerSeries) (bool, error) {
	return false, ErrWalClosed
}
