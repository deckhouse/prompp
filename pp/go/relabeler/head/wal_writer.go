package head

import (
	"bytes"
	"fmt"
	"io"
	"slices"
	"sync"
)

type SegmentIsWrittenNotifier interface {
	NotifySegmentIsWritten(shardID uint16)
}

type WriteSyncCloser interface {
	io.WriteCloser
	Sync() error
}

type segmentWriter struct {
	shardID        uint16
	segments       []EncodedSegment
	buffer         *bytes.Buffer
	notifier       SegmentIsWrittenNotifier
	writer         WriteSyncCloser
	writeCompleted bool
}

func newSegmentWriter(shardID uint16, writer WriteSyncCloser, notifier SegmentIsWrittenNotifier) *segmentWriter {
	return &segmentWriter{
		shardID:        shardID,
		buffer:         bytes.NewBuffer(nil),
		notifier:       notifier,
		writer:         writer,
		writeCompleted: true,
	}
}

func (w *segmentWriter) Write(segment EncodedSegment) error {
	w.segments = append(w.segments, segment)
	return nil
}

func (w *segmentWriter) Flush() error {
	if !w.writeCompleted {
		if err := w.flushAndSync(); err != nil {
			return fmt.Errorf("flush and sync: %w", err)
		}
	}

	for index, segment := range w.segments {
		if encoded, err := w.encodeAndFlush(segment); err != nil {
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

func (w *segmentWriter) sync() error {
	if err := w.writer.Sync(); err != nil {
		return fmt.Errorf("writer sync: %w", err)
	}

	w.notifier.NotifySegmentIsWritten(w.shardID)
	w.writeCompleted = true
	return nil
}

func (w *segmentWriter) encodeAndFlush(segment EncodedSegment) (encoded bool, err error) {
	if _, err := WriteSegment(w.buffer, segment); err != nil {
		w.buffer.Reset()
		return false, fmt.Errorf("encode segment: %w", err)
	}

	w.writeCompleted = false
	return true, w.flushAndSync()
}

func (w *segmentWriter) flushAndSync() error {
	if _, err := w.buffer.WriteTo(w.writer); err != nil {
		return fmt.Errorf("buffer write: %w", err)
	}

	if err := w.sync(); err != nil {
		return fmt.Errorf("writer sync: %w", err)
	}

	return nil
}

func (w *segmentWriter) Close() error {
	return w.writer.Close()
}

type segmentWriteNotifier struct {
	mtx    sync.Mutex
	shards []uint32
	setter LastAppendedSegmentIDSetter
}

func newSegmentWriteNotifier(numberOfShards uint16, setter LastAppendedSegmentIDSetter) *segmentWriteNotifier {
	return &segmentWriteNotifier{
		shards: make([]uint32, numberOfShards),
		setter: setter,
	}
}

func (swn *segmentWriteNotifier) NotifySegmentIsWritten(shardID uint16) {
	swn.mtx.Lock()
	defer swn.mtx.Unlock()
	swn.shards[shardID]++
	minNumberOfSegments := slices.Min(swn.shards)
	if minNumberOfSegments > 0 {
		swn.setter.SetLastAppendedSegmentID(minNumberOfSegments - 1)
	}
}

func (swn *segmentWriteNotifier) Set(shardID uint16, numberOfSegments uint32) {
	swn.shards[shardID] = numberOfSegments
}
