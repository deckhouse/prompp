package wal

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/prometheus/pp/go/util/locker"
)

//go:generate -command moq go run github.com/matryer/moq --rm --skip-ensure --pkg wal_test --out
//go:generate moq wal_moq_test.go . SegmentWriter Encoder EncodedSegment

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
type Encoder[TSegment EncodedSegment] interface {
	// Encode encodes the inner series into a segment.
	Encode(innerSeriesSlice []cppbridge.InnerSeries) (uint32, error)

	// Finalize finalizes the encoder and returns the encoded segment.
	Finalize() (TSegment, error)
}

// EncodedSegment the minimum required Segment implementation for a [Wal].
type EncodedSegment interface {
	// Size returns the size of the segment.
	Size() int64

	// CRC32 returns the CRC32 of the segment.
	CRC32() uint32

	// Samples returns the number of samples in the segment.
	Samples() uint32

	// WriteTo implements [io.WriterTo] interface.
	io.WriterTo
}

// Wal write-ahead log for [Shard].
type Wal[TSegment EncodedSegment, TWriter SegmentWriter[TSegment]] struct {
	encoder        Encoder[TSegment] // *cppbridge.HeadWalEncoder
	segmentWriter  TWriter
	lssLocker      locker.RLockable
	encLocker      sync.Mutex
	swLocker       sync.Mutex
	maxSegmentSize uint32
	corrupted      bool
	limitExhausted bool
	closed         bool
	// stat
	samplesPerSegment prometheus.Counter
	segments          prometheus.Gauge
}

// NewWal init new [Wal].
func NewWal[TSegment EncodedSegment, TWriter SegmentWriter[TSegment]](
	encoder Encoder[TSegment],
	segmentWriter TWriter,
	lssLocker locker.RLockable,
	maxSegmentSize uint32,
	shardID uint16,
	registerer prometheus.Registerer,
) *Wal[TSegment, TWriter] {
	factory := util.NewUnconflictRegisterer(registerer)
	ls := prometheus.Labels{"shard_id": strconv.FormatUint(uint64(shardID), 10)}
	w := &Wal[TSegment, TWriter]{
		encoder:        encoder,
		segmentWriter:  segmentWriter,
		lssLocker:      lssLocker,
		encLocker:      sync.Mutex{},
		swLocker:       sync.Mutex{},
		maxSegmentSize: maxSegmentSize,
		samplesPerSegment: factory.NewCounter(prometheus.CounterOpts{
			Name:        "prompp_shard_wal_samples_per_segment_sum",
			Help:        "Number of samples per segment.",
			ConstLabels: ls,
		}),
		segments: factory.NewGauge(prometheus.GaugeOpts{
			Name:        "prompp_shard_wal_segments",
			Help:        "Number of segments.",
			ConstLabels: ls,
		}),
	}

	w.segments.Set(0)

	return w
}

// NewCorruptedWal init new corrupted [Wal].
func NewCorruptedWal[
	TSegment EncodedSegment,
	TWriter SegmentWriter[TSegment],
]() *Wal[TSegment, TWriter] {
	return &Wal[TSegment, TWriter]{
		lssLocker: locker.NoopLocker{},
		encLocker: sync.Mutex{},
		swLocker:  sync.Mutex{},
		corrupted: true,
	}
}

// Close closes the wal segmentWriter.
func (w *Wal[TSegment, TWriter]) Close() error {
	if w.corrupted {
		return nil
	}

	w.swLocker.Lock()
	defer w.swLocker.Unlock()

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
func (w *Wal[TSegment, TWriter]) Commit() error {
	if w.corrupted {
		return ErrWalIsCorrupted
	}

	w.swLocker.Lock()
	defer w.swLocker.Unlock()

	w.encLocker.Lock()
	w.lssLocker.RLock()
	segment, err := w.encoder.Finalize()
	w.lssLocker.RUnlock()
	if err != nil {
		w.encLocker.Unlock()
		return fmt.Errorf("failed to finalize segment: %w", err)
	}

	w.limitExhausted = false
	w.encLocker.Unlock()

	w.samplesPerSegment.Add(float64(segment.Samples()))
	w.segments.Inc()

	if err = w.segmentWriter.Write(segment); err != nil {
		return fmt.Errorf("failed to write segment: %w", err)
	}

	return nil
}

// CurrentSize returns current wal size.
func (w *Wal[TSegment, TWriter]) CurrentSize() int64 {
	if w.corrupted {
		return 0
	}

	return w.segmentWriter.CurrentSize()
}

// Flush wal [SegmentWriter], write all buffered data to storage.
func (w *Wal[TSegment, TWriter]) Flush() error {
	if w.corrupted {
		return nil
	}

	w.swLocker.Lock()
	defer w.swLocker.Unlock()

	return w.segmentWriter.Flush()
}

// Sync commits the current contents of the [SegmentWriter].
func (w *Wal[TSegment, TWriter]) Sync() error {
	if w.corrupted {
		return ErrWalIsCorrupted
	}

	w.swLocker.Lock()
	defer w.swLocker.Unlock()

	return w.segmentWriter.Sync()
}

// Write the incoming inner series to wal encoder.
func (w *Wal[TSegment, TWriter]) Write(innerSeriesSlice []cppbridge.InnerSeries) (bool, error) {
	if w.corrupted {
		return false, ErrWalIsCorrupted
	}

	w.encLocker.Lock()
	defer w.encLocker.Unlock()

	samples, err := w.encoder.Encode(innerSeriesSlice)
	if err != nil {
		return false, fmt.Errorf("failed to encode inner series: %w", err)
	}

	if w.maxSegmentSize == 0 {
		return false, nil
	}

	// memoize reaching of limits to deduplicate triggers
	if !w.limitExhausted && samples >= w.maxSegmentSize {
		w.limitExhausted = true
		return true, nil
	}

	return false, nil
}
