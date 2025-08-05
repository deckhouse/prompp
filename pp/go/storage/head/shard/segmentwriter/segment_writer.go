package segmentwriter

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

// SegmentWriter writer for segments.
type SegmentWriter[EncodedSegment any] struct {
	shardID        uint16
	segments       []EncodedSegment
	buffer         *bytes.Buffer
	notifier       SegmentIsWrittenNotifier
	writer         WriteSyncCloser
	currentSize    int64
	writeCompleted bool
}

// NewSegmentWriter init new [SegmentWriter].
func NewSegmentWriter[EncodedSegment any](
	shardID uint16,
	writer WriteSyncCloser,
	notifier SegmentIsWrittenNotifier,
) (*SegmentWriter[EncodedSegment], error) {
	info, err := writer.Stat()
	if err != nil {
		return nil, err
	}

	return &SegmentWriter[EncodedSegment]{
		shardID:        shardID,
		buffer:         bytes.NewBuffer(nil),
		notifier:       notifier,
		writer:         writer,
		currentSize:    info.Size(),
		writeCompleted: true,
	}, nil
}

// Close closes the writer [WriteSyncCloser].
func (w *SegmentWriter[EncodedSegment]) Close() error {
	return w.writer.Close()
}

// CurrentSize return current shard wal size.
func (w *SegmentWriter[EncodedSegment]) CurrentSize() int64 {
	return atomic.LoadInt64(&w.currentSize)
}

// Flush and sync buffer and collected segments to [WriteSyncCloser].
func (w *SegmentWriter[EncodedSegment]) Flush() error {
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

// Write to buffer [SegmentWriter] incoming [EncodedSegment].
func (w *SegmentWriter[EncodedSegment]) Write(segment EncodedSegment) error {
	w.segments = append(w.segments, segment)
	return nil
}

// flushAndSync write the contents from buffer to [WriteSyncCloser] and sync.
func (w *SegmentWriter[EncodedSegment]) flushAndSync() error {
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
func (w *SegmentWriter[EncodedSegment]) sync() error {
	if err := w.writer.Sync(); err != nil {
		return fmt.Errorf("writer sync: %w", err)
	}

	w.notifier.NotifySegmentIsWritten(w.shardID)
	w.writeCompleted = true
	return nil
}

// writeToBufferAndFlush write [EncodedSegment] as slice byte to buffer and flush to [WriteSyncCloser].
func (w *SegmentWriter[EncodedSegment]) writeToBufferAndFlush(segment EncodedSegment) (encoded bool, err error) {
	if _, err := WriteSegment(w.buffer, segment); err != nil {
		w.buffer.Reset()
		return false, fmt.Errorf("encode segment: %w", err)
	}

	w.writeCompleted = false

	if err := w.flushAndSync(); err != nil {
		return true, err
	}

	return true, nil
}
