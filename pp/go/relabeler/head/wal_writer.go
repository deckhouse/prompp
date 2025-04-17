package head

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/util"
)

type SegmentIsWrittenNotifier interface {
	NotifySegmentIsWritten(shardID uint16)
}

type WriteSyncCloser interface {
	io.WriteCloser
	Sync() error
	Stat() (os.FileInfo, error)
}

type segmentWriter struct {
	shardID        uint16
	segments       []EncodedSegment
	buffer         *bytes.Buffer
	notifier       SegmentIsWrittenNotifier
	writer         WriteSyncCloser
	walSize        prometheus.Gauge
	initSize       int64
	writeCompleted bool
}

func newSegmentWriter(
	shardID uint16,
	writer WriteSyncCloser,
	notifier SegmentIsWrittenNotifier,
	registerer prometheus.Registerer,
) (*segmentWriter, error) {
	info, err := writer.Stat()
	if err != nil {
		return nil, err
	}

	factory := util.NewUnconflictRegisterer(registerer)

	return &segmentWriter{
		shardID:  shardID,
		buffer:   bytes.NewBuffer(nil),
		notifier: notifier,
		writer:   writer,
		walSize: factory.NewGauge(
			prometheus.GaugeOpts{
				Name:        "prompp_head_current_wal_size",
				Help:        "The size of the wall of the current head.",
				ConstLabels: prometheus.Labels{"shard_id": strconv.FormatUint(uint64(shardID), 10)},
			},
		),
		initSize:       info.Size(),
		writeCompleted: true,
	}, nil
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
	n, err := WriteSegment(w.buffer, segment)
	if err != nil {
		w.buffer.Reset()
		return false, fmt.Errorf("encode segment: %w", err)
	}

	w.writeCompleted = false

	if err := w.flushAndSync(); err != nil {
		return true, err
	}

	if w.initSize != 0 {
		w.walSize.Set(float64(w.initSize))
		w.initSize = 0
	}

	w.walSize.Add(float64(n))

	return true, nil
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
