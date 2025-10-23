package wal

import (
	"bufio"
	"errors"
	"fmt"
	"io"
)

//go:generate -command moq go run github.com/matryer/moq --rm --skip-ensure --pkg wal_test --out
//go:generate moq wal_reader_moq_test.go . ReadSegment

// ReadSegment the minimum required [Segment] implementation for a [Wal].
type ReadSegment interface {
	// ReadFrom reads [ReadSegment] data from r [io.Reader]. The return value n is the number of bytes read.
	// Any error encountered during the read is also returned.
	ReadFrom(r io.Reader) (int64, error)

	// Reset [ReadSegment] data.
	Reset()
}

// SegmentWalReader buffered reader [ReadSegment]s from wal.
type SegmentWalReader[TReadSegment ReadSegment] struct {
	reader      *bufio.Reader
	segmentCtor func() TReadSegment
}

// NewSegmentWalReader init new [SegmentWalReader].
func NewSegmentWalReader[TReadSegment ReadSegment](
	r io.Reader,
	segmentCtor func() TReadSegment,
) *SegmentWalReader[TReadSegment] {
	return &SegmentWalReader[TReadSegment]{
		reader:      bufio.NewReaderSize(r, 1024*1024*4),
		segmentCtor: segmentCtor,
	}
}

// ForEachSegment reads [ReadSegment]s from the reader and for each [ReadSegment] a [do] is called for each,
// if an error occurs during reading it will return and reading will stop.
func (r *SegmentWalReader[TReadSegment]) ForEachSegment(do func(TReadSegment) error) error {
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
