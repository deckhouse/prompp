package remotewriter

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/reader"
)

type walReader struct {
	nextSegmentID uint32
	file          *os.File
	reader        io.Reader
}

func newWalReader(fileName string) (*walReader, uint8, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read wal file: %w", err)
	}

	_, encoderVersion, _, err := reader.ReadHeader(file)
	if err != nil {
		return nil, 0, errors.Join(fmt.Errorf("failed to read header: %w", err), file.Close())
	}

	return &walReader{
		file:   file,
		reader: bufio.NewReaderSize(file, 4096),
	}, encoderVersion, nil
}

// Close wal file.
func (r *walReader) Close() error {
	return r.file.Close()
}

// Read [Segment] from wal and return.
func (r *walReader) Read() (segment Segment, err error) {
	if _, err = segment.ReadFrom(r.reader); err != nil {
		return segment, fmt.Errorf("failed to read segment: %w", err)
	}

	segment.ID = r.nextSegmentID
	r.nextSegmentID++

	return segment, nil
}

// Segment encoded segment from wal.
type Segment struct {
	ID uint32
	reader.Segment
}
