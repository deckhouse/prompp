package wal

import (
	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

// NoopWal no operation write-ahead log for [Shard].
type NoopWal struct{}

// NewNoopWal init new [NoopWal].
func NewNoopWal() NoopWal {
	return NoopWal{}
}

// Close implementation of [NoopWal], do nothing.
func (NoopWal) Close() error {
	return nil
}

// Commit implementation of [NoopWal], do nothing.
func (NoopWal) Commit() error {
	return nil
}

// CurrentSize implementation of [NoopWal], do nothing.
func (NoopWal) CurrentSize() int64 {
	return 0
}

// Flush implementation of [NoopWal], do nothing.
func (NoopWal) Flush() error {
	return nil
}

// MaxItemIndex implementation of [NoopWal], do nothing.
func (NoopWal) MaxItemIndex() uint32 {
	return 0
}

// Sync implementation of [NoopWal], do nothing.
func (NoopWal) Sync() error {
	return nil
}

// Write implementation of [NoopWal], do nothing.
func (NoopWal) Write([]cppbridge.InnerSeries) (bool, error) {
	return false, nil
}
