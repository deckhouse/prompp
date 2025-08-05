package wal

import (
	"fmt"
	"io"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

const (
	// FileFormatVersion wal file version.
	FileFormatVersion = 1
)

// SegmentWriter writer for wal segments.
type SegmentWriter interface {
	// CurrentSize return current shard wal size.
	CurrentSize() int64
	// Write encoded segment to writer.
	Write(segment EncodedSegment) error
	// Flush write all buffered segments.
	Flush() error
	// Close closes the storage.
	Close() error
}

// EncodedSegment the minimum required Segment implementation for a [Wal].
type EncodedSegment interface {
	Size() int64
	CRC32() uint32
	Samples() uint32
	io.WriterTo
}

// Wal write-ahead log for [Shard].
type Wal[Writer SegmentWriter] struct {
	encoder        *cppbridge.HeadWalEncoder
	segmentWriter  Writer
	maxSegmentSize uint32
	corrupted      bool
	limitExhausted bool
}

// NewWal init new [Wal].
func NewWal[Writer SegmentWriter](
	encoder *cppbridge.HeadWalEncoder,
	maxSegmentSize uint32,
	segmentWriter Writer,
) *Wal[Writer] {
	return &Wal[Writer]{
		encoder:        encoder,
		segmentWriter:  segmentWriter,
		maxSegmentSize: maxSegmentSize,
	}
}

// NewCorruptedWal init new corrupted [Wal].
func NewCorruptedWal[Writer SegmentWriter]() *Wal[Writer] {
	return &Wal[Writer]{
		corrupted: true,
	}
}

// CurrentSize returns current wal size.
func (w *Wal[Writer]) CurrentSize() int64 {
	return w.segmentWriter.CurrentSize()
}

// Write the incoming inner series to wal encoder.
func (w *Wal[Writer]) Write(innerSeriesSlice []*cppbridge.InnerSeries) (bool, error) {
	if w.corrupted {
		return false, fmt.Errorf("writing in corrupted wal")
	}

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

// Commit finalize segment from encoder and write to [SegmentWriter].
func (w *Wal[Writer]) Commit() error {
	if w.corrupted {
		return fmt.Errorf("committing corrupted wal")
	}

	segment, err := w.encoder.Finalize()
	if err != nil {
		return fmt.Errorf("failed to finalize segment: %w", err)
	}
	w.limitExhausted = false

	if err = w.segmentWriter.Write(segment); err != nil {
		return fmt.Errorf("failed to write segment: %w", err)
	}

	if err = w.segmentWriter.Flush(); err != nil {
		return fmt.Errorf("failed to flush segment writer: %w", err)
	}

	return nil
}

// Flush wal [SegmentWriter].
func (w *Wal[Writer]) Flush() error {
	return w.segmentWriter.Flush()
}

// Close closes the wal segmentWriter.
func (w *Wal[Writer]) Close() error {
	return w.segmentWriter.Close()
}
