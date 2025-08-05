package segmentwriter

import (
	"bytes"
	"io"
	"os"
	"sync/atomic"
)

type SegmentIsWrittenNotifier interface {
	NotifySegmentIsWritten(shardID uint16)
}

type WriteSyncCloser interface {
	io.WriteCloser
	Sync() error
	Stat() (os.FileInfo, error)
}

type SegmentWriter struct {
	shardID        uint16
	segments       []EncodedSegment
	buffer         *bytes.Buffer
	notifier       SegmentIsWrittenNotifier
	writer         WriteSyncCloser
	currentSize    int64
	writeCompleted bool
}

func NewSegmentWriter(
	shardID uint16,
	writer WriteSyncCloser,
	notifier SegmentIsWrittenNotifier,
) (*SegmentWriter, error) {
	info, err := writer.Stat()
	if err != nil {
		return nil, err
	}

	return &SegmentWriter{
		shardID:        shardID,
		buffer:         bytes.NewBuffer(nil),
		notifier:       notifier,
		writer:         writer,
		currentSize:    info.Size(),
		writeCompleted: true,
	}, nil
}

// CurrentSize return current shard wal size.
func (w *SegmentWriter) CurrentSize() int64 {
	return atomic.LoadInt64(&w.currentSize)
}
