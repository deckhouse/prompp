package writer

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync/atomic"
)

// SegmentIsWrittenNotifier notify when new segment write.
type SegmentIsWrittenNotifier interface {
	NotifySegmentIsWritten(shardID uint16)
}

// WriteSyncCloser writer implementation [os.File].
type WriteSyncCloser interface {
	io.WriteCloser
	Sync() error
	Stat() (os.FileInfo, error)
}

// SegmentWriterFN encode to slice byte and write to [io.Writer].
type SegmentWriterFN[Segment any] func(writer io.Writer, segment Segment) (n int, err error)

// Buffered writer for segments.
type Buffered[Segment any] struct {
	shardID        uint16
	segments       []Segment
	buffer         *bytes.Buffer
	notifier       SegmentIsWrittenNotifier
	swriter        SegmentWriterFN[Segment]
	writer         WriteSyncCloser
	currentSize    int64
	writeCompleted bool
}

// NewBuffered init new [Buffered].
func NewBuffered[Segment any](
	shardID uint16,
	writer WriteSyncCloser,
	swriter SegmentWriterFN[Segment],
	notifier SegmentIsWrittenNotifier,
) (*Buffered[Segment], error) {
	info, err := writer.Stat()
	if err != nil {
		return nil, err
	}

	return &Buffered[Segment]{
		shardID:        shardID,
		buffer:         bytes.NewBuffer(nil),
		notifier:       notifier,
		swriter:        swriter,
		writer:         writer,
		currentSize:    info.Size(),
		writeCompleted: true,
	}, nil
}

// Close closes the writer [WriteSyncCloser].
func (w *Buffered[Segment]) Close() error {
	return w.writer.Close()
}

// CurrentSize return current shard wal size.
func (w *Buffered[Segment]) CurrentSize() int64 {
	return atomic.LoadInt64(&w.currentSize)
}

// Flush and sync buffer and collected segments to [WriteSyncCloser].
func (w *Buffered[Segment]) Flush() error {
	if !w.writeCompleted {
		if err := w.flushAndSync(); err != nil {
			return fmt.Errorf("flush and sync: %w", err)
		}
	}

	for index, segment := range w.segments {
		if encoded, err := w.writeToBufferAndFlush(segment); err != nil {
			if encoded {
				index++
			}
			// shift encoded segments to the left
			copy(w.segments, w.segments[index:])
			w.segments = w.segments[:len(w.segments)-index]
			return fmt.Errorf("flush segment: %w", err)
		}
	}

	w.segments = nil
	return nil
}

// Write to buffer [Buffered] incoming [Segment].
func (w *Buffered[Segment]) Write(segment Segment) error {
	w.segments = append(w.segments, segment)
	return nil
}

// flushAndSync write the contents from buffer to [WriteSyncCloser] and sync.
func (w *Buffered[Segment]) flushAndSync() error {
	n, err := w.buffer.WriteTo(w.writer)
	atomic.AddInt64(&w.currentSize, n)
	if err != nil {
		return fmt.Errorf("buffer write: %w", err)
	}

	if err := w.sync(); err != nil {
		return fmt.Errorf("writer sync: %w", err)
	}

	return nil
}

// sync commits the current contents of the [WriteSyncCloser] and notify [SegmentIsWrittenNotifier].
func (w *Buffered[Segment]) sync() error {
	if err := w.writer.Sync(); err != nil {
		return fmt.Errorf("writer sync: %w", err)
	}

	w.notifier.NotifySegmentIsWritten(w.shardID)
	w.writeCompleted = true
	return nil
}

// writeToBufferAndFlush write [Segment] as slice byte to buffer and flush to [WriteSyncCloser].
func (w *Buffered[Segment]) writeToBufferAndFlush(segment Segment) (encoded bool, err error) {
	if _, err := w.swriter(w.buffer, segment); err != nil {
		w.buffer.Reset()
		return false, fmt.Errorf("encode segment: %w", err)
	}

	w.writeCompleted = false

	if err := w.flushAndSync(); err != nil {
		return true, err
	}

	return true, nil
}
