package wal

import (
	"bufio"
	"errors"
	"fmt"
	"io"
)

// ReadSegment the minimum required [Segment] implementation for a [Wal].
type ReadSegment[T any] interface {
	// ReadFrom reads [ReadSegment] data from r [io.Reader]. The return value n is the number of bytes read.
	// Any error encountered during the read is also returned.
	ReadFrom(r io.Reader) (int64, error)

	// Reset [ReadSegment] data.
	Reset()

	// for use as a pointer
	*T
}

// SegmentWalReader buffered reader [ReadSegment]s from wal.
type SegmentWalReader[T any, TReadSegment ReadSegment[T]] struct {
	reader      *bufio.Reader
	segmentCtor func() TReadSegment
}

// NewSegmentWalReader init new [SegmentWalReader].
func NewSegmentWalReader[T any, TReadSegment ReadSegment[T]](
	r io.Reader,
	segmentCtor func() TReadSegment,
) *SegmentWalReader[T, TReadSegment] {
	return &SegmentWalReader[T, TReadSegment]{
		reader:      bufio.NewReaderSize(r, 1024*1024*4),
		segmentCtor: segmentCtor,
	}
}

// ForEachSegment reads [ReadSegment]s from the reader and for each [ReadSegment] a [do] is called for each,
// if an error occurs during reading it will return and reading will stop.
func (r *SegmentWalReader[T, TReadSegment]) ForEachSegment(do func(TReadSegment) error) error {
	segment := r.segmentCtor()
	for {
		segment.Reset()

		if _, err := segment.ReadFrom(r.reader); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("failed to read segment: %w", err)
		}

		if err := do(segment); err != nil {
			return err
		}
	}

	return nil
}
