package wal

import (
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

//go:generate -command moq go run github.com/matryer/moq --rm --skip-ensure --pkg wal_test --out
//go:generate moq wal_moq_test.go . SegmentWriter Encoder StatsSegment EncodedSegment

const (
	// FileFormatVersion wal file version.
	FileFormatVersion = 1
)

// ErrWalIsCorrupted errror when wal is corrupted.
var ErrWalIsCorrupted = errors.New("wal is corrupted")

// SegmentWriter writer for wal segments.
type SegmentWriter[TSegment EncodedSegment] interface {
	// CurrentSize return current shard wal size.
	CurrentSize() int64

	// Write encoded segment to writer.
	Write(segment TSegment) error

	// Flush write all buffered segments.
	Flush() error

	// Sync commits the current contents of the [SegmentWriter].
	Sync() error

	// Close closes the storage.
	Close() error
}

// Encoder the minimum required Encoder implementation for a [Wal].
type Encoder[TSegment EncodedSegment, TStats StatsSegment] interface {
	Encode(innerSeriesSlice []*cppbridge.InnerSeries) (TStats, error)
	Finalize() (TSegment, error)
}

// StatsSegment stats data for [Encoder].
type StatsSegment interface {
	Samples() uint32
}

// EncodedSegment the minimum required Segment implementation for a [Wal].
type EncodedSegment interface {
	Size() int64
	CRC32() uint32
	Samples() uint32
	io.WriterTo
}

// Wal write-ahead log for [Shard].
type Wal[TSegment EncodedSegment, TStats StatsSegment, TWriter SegmentWriter[TSegment]] struct {
	encoder        Encoder[TSegment, TStats] // *cppbridge.HeadWalEncoder
	segmentWriter  TWriter
	locker         sync.Mutex
	maxSegmentSize uint32
	corrupted      bool
	limitExhausted bool
	closed         bool
}

// NewWal init new [Wal].
func NewWal[TSegment EncodedSegment, TStats StatsSegment, TWriter SegmentWriter[TSegment]](
	encoder Encoder[TSegment, TStats],
	segmentWriter TWriter,
	maxSegmentSize uint32,
) *Wal[TSegment, TStats, TWriter] {
	return &Wal[TSegment, TStats, TWriter]{
		encoder:        encoder,
		segmentWriter:  segmentWriter,
		locker:         sync.Mutex{},
		maxSegmentSize: maxSegmentSize,
	}
}

// NewCorruptedWal init new corrupted [Wal].
func NewCorruptedWal[
	TSegment EncodedSegment,
	TStats StatsSegment,
	TWriter SegmentWriter[TSegment],
]() *Wal[TSegment, TStats, TWriter] {
	return &Wal[TSegment, TStats, TWriter]{
		locker:    sync.Mutex{},
		corrupted: true,
	}
}

// Close closes the wal segmentWriter.
func (w *Wal[TSegment, TStats, TWriter]) Close() error {
	if w.corrupted {
		return nil
	}

	w.locker.Lock()
	defer w.locker.Unlock()

	if w.closed {
		return nil
	}

	if err := w.segmentWriter.Close(); err != nil {
		return err
	}

	w.closed = true

	return nil
}

// Commit finalize segment from encoder and write to [SegmentWriter].
// It is necessary to lock the LSS for reading for the commit.
func (w *Wal[TSegment, TStats, TWriter]) Commit() error {
	if w.corrupted {
		return ErrWalIsCorrupted
	}

	w.locker.Lock()
	defer w.locker.Unlock()

	segment, err := w.encoder.Finalize()
	if err != nil {
		return fmt.Errorf("failed to finalize segment: %w", err)
	}
	w.limitExhausted = false

	if err = w.segmentWriter.Write(segment); err != nil {
		return fmt.Errorf("failed to write segment: %w", err)
	}

	return nil
}

// CurrentSize returns current wal size.
func (w *Wal[TSegment, TStats, TWriter]) CurrentSize() int64 {
	if w.corrupted {
		return 0
	}

	return w.segmentWriter.CurrentSize()
}

// Flush wal [SegmentWriter], write all buffered data to storage.
func (w *Wal[TSegment, TStats, TWriter]) Flush() error {
	if w.corrupted {
		return ErrWalIsCorrupted
	}

	w.locker.Lock()
	defer w.locker.Unlock()

	return w.segmentWriter.Flush()
}

// Sync commits the current contents of the [SegmentWriter].
func (w *Wal[TSegment, TStats, TWriter]) Sync() error {
	if w.corrupted {
		return ErrWalIsCorrupted
	}

	w.locker.Lock()
	defer w.locker.Unlock()

	return w.segmentWriter.Sync()
}

// Write the incoming inner series to wal encoder.
func (w *Wal[TSegment, TStats, TWriter]) Write(innerSeriesSlice []*cppbridge.InnerSeries) (bool, error) {
	if w.corrupted {
		return false, ErrWalIsCorrupted
	}

	w.locker.Lock()
	defer w.locker.Unlock()

	stats, err := w.encoder.Encode(innerSeriesSlice)
	if err != nil {
		return false, fmt.Errorf("failed to encode inner series: %w", err)
	}

	if w.maxSegmentSize == 0 {
		return false, nil
	}

	// memoize reaching of limits to deduplicate triggers
	if !w.limitExhausted && stats.Samples() >= w.maxSegmentSize {
		w.limitExhausted = true
		return true, nil
	}

	return false, nil
}
