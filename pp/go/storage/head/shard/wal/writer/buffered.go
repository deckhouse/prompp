package writer

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync/atomic"
)

//go:generate -command moq go run github.com/matryer/moq --rm --skip-ensure --pkg writer_test --out
//go:generate moq buffered_moq_test.go . FileInfo SegmentIsWrittenNotifier FileWriter

// FileInfo alias for [os.FileInfo].
type FileInfo = os.FileInfo

// SegmentIsWrittenNotifier notify when new segment write.
type SegmentIsWrittenNotifier interface {
	NotifySegmentIsWritten(shardID uint16)
	NotifySegmentWrite(shardID uint16)
}

// FileWriter writer implementation [os.File].
type FileWriter interface {
	io.WriteCloser
	Sync() error
	Stat() (FileInfo, error)
}

// SegmentWriterFN encode to slice byte and write to [io.Writer].
type SegmentWriterFN[TSegment any] func(writer io.Writer, segment TSegment) (n int, err error)

// Buffered writer for segments.
type Buffered[TSegment any] struct {
	shardID        uint16
	segments       []TSegment
	buffer         *bytes.Buffer
	notifier       SegmentIsWrittenNotifier
	swriter        SegmentWriterFN[TSegment]
	writer         FileWriter
	currentSize    int64
	writeCompleted bool
}

// NewBuffered init new [Buffered].
func NewBuffered[TSegment any](
	shardID uint16,
	writer FileWriter,
	swriter SegmentWriterFN[TSegment],
	notifier SegmentIsWrittenNotifier,
) (*Buffered[TSegment], error) {
	info, err := writer.Stat()
	if err != nil {
		return nil, err
	}

	return &Buffered[TSegment]{
		shardID:        shardID,
		buffer:         bytes.NewBuffer(nil),
		notifier:       notifier,
		swriter:        swriter,
		writer:         writer,
		currentSize:    info.Size(),
		writeCompleted: true,
	}, nil
}

// Close closes the writer [FileWriter].
func (w *Buffered[TSegment]) Close() error {
	return w.writer.Close()
}

// CurrentSize return current shard wal size.
func (w *Buffered[TSegment]) CurrentSize() int64 {
	return atomic.LoadInt64(&w.currentSize)
}

// Flush buffer and collected segments to [WriteSyncFileWriterCloser].
func (w *Buffered[TSegment]) Flush() error {
	if !w.writeCompleted {
		if err := w.flushBuffer(); err != nil {
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

	if len(w.segments) != 0 && cap(w.segments) >= len(w.segments)*2 { //revive:disable-line:add-constant // x2
		w.segments = make([]TSegment, 0, len(w.segments))
	} else {
		clear(w.segments)
		w.segments = w.segments[:0]
	}

	return nil
}

// Sync commits the current contents of the [FileWriter] and notify [SegmentIsWrittenNotifier].
func (w *Buffered[TSegment]) Sync() error {
	if err := w.writer.Sync(); err != nil {
		return fmt.Errorf("writer sync: %w", err)
	}

	w.notifier.NotifySegmentIsWritten(w.shardID)
	w.writeCompleted = true
	return nil
}

// Write to buffer [Buffered] incoming [Segment].
func (w *Buffered[TSegment]) Write(segment TSegment) error {
	w.segments = append(w.segments, segment)
	return nil
}

// flushBuffer write the contents from buffer to [FileWriter].
func (w *Buffered[TSegment]) flushBuffer() error {
	n, err := w.buffer.WriteTo(w.writer)
	atomic.AddInt64(&w.currentSize, n)
	if err != nil {
		return fmt.Errorf("buffer write: %w", err)
	}

	return nil
}

// writeToBufferAndFlush write [Segment] as slice byte to buffer and flush to [FileWriter].
func (w *Buffered[TSegment]) writeToBufferAndFlush(segment TSegment) (encoded bool, err error) {
	if _, err := w.swriter(w.buffer, segment); err != nil {
		w.buffer.Reset()
		return false, fmt.Errorf("encode segment: %w", err)
	}

	w.writeCompleted = false
	w.notifier.NotifySegmentWrite(w.shardID)

	if err := w.flushBuffer(); err != nil {
		return true, err
	}

	return true, nil
}
