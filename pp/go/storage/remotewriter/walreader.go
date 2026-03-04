package remotewriter

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/reader"
	"github.com/prometheus/prometheus/pp/go/util"
)

//go:generate -command moq go run github.com/matryer/moq --rm --skip-ensure --pkg mock --out
//go:generate moq mock/segment.go . Segment

type walReader struct {
	file              *util.FileReader
	reader            io.Reader
	nextSegmentID     uint32
	fileFormatVersion uint8
}

func newWalReader(fileName string) (*walReader, uint8, error) {
	file, err := util.OpenFileReader(fileName)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read wal file: %w", err)
	}

	fileFormatVersion, encoderVersion, _, err := reader.ReadHeader(file)
	if err != nil {
		return nil, 0, errors.Join(fmt.Errorf("failed to read header: %w", err), file.Close())
	}

	return &walReader{
		file:              file,
		reader:            bufio.NewReaderSize(file, 4096), //revive:disable-line:add-constant // 4kb
		fileFormatVersion: fileFormatVersion,
	}, encoderVersion, nil
}

// Close wal file.
func (r *walReader) Close() error {
	return r.file.Close()
}

// EmptySegment creates an empty segment of the required version.
func (r *walReader) EmptySegment() (Segment, error) {
	switch r.fileFormatVersion {
	case 1:
		return EmptySegmentV1(), nil
	case 2: //revive:disable-line:add-constant // it's wal fileFormatVersion v2
		return EmptySegmentV2(), nil
	default:
		return nil, fmt.Errorf("unknown wal file format: %d", r.fileFormatVersion)
	}
}

// Read reads up data into s [Segment] from wal.
// It may return a non-nil error if some error condition is known, such as EOF.
func (r *walReader) Read(s Segment) error {
	if _, err := s.ReadFrom(r.reader); err != nil {
		return fmt.Errorf("failed to read segment: %w", err)
	}

	s.SetSegmentID(r.nextSegmentID)
	r.nextSegmentID++

	return nil
}

// ReadSegmentBody reads [Segment] body from wal.
// It may return a non-nil error if some error condition is known, such as EOF.
func (r *walReader) ReadSegmentBody(s Segment) error {
	if _, err := s.ReadBody(r.reader); err != nil {
		return fmt.Errorf("failed to read segment body: %w", err)
	}

	r.nextSegmentID++

	return nil
}

// ReadSegmentID reads [Segment] ID from wal.
// It may return a non-nil error if some error condition is known, such as EOF.
func (r *walReader) ReadSegmentID(s Segment) error {
	if _, err := s.ReadID(r.reader); err != nil {
		return fmt.Errorf("failed to read segment id: %w", err)
	}

	s.SetSegmentID(r.nextSegmentID)

	return nil
}

//
// Segment
//

// Segment implementation encoded segment from wal.
type Segment interface {
	// Bytes returns the data read.
	Bytes() []byte

	// ID returns [Segment] ID.
	ID() uint32

	// Length returns the length of slice byte.
	Length() int

	// ReadBody reads [Segment] body from r [io.Reader]. The return value n is the number of bytes read.
	// Any error encountered during the read is also returned.
	ReadBody(r io.Reader) (int64, error)

	// ReadFrom reads [Segment] data from r [io.Reader]. The return value n is the number of bytes read.
	// Any error encountered during the read is also returned.
	ReadFrom(r io.Reader) (int64, error)

	// ReadID reads [Segment] ID from r [io.Reader]. The return value n is the number of bytes read.
	// Any error encountered during the read is also returned.
	ReadID(r io.Reader) (int64, error)

	// Reset [Segment] data.
	Reset()

	// Samples returns count of samples in [Segment].
	Samples() uint32

	// SetSegmentID sets the segment ID value.
	SetSegmentID(sid uint32)
}

//
// SegmentV1
//

// SegmentV1 encoded segment from wal, version 1.
type SegmentV1 struct {
	id uint32
	reader.Segment
}

// EmptySegmentV1 init new empty [SegmentV1].
func EmptySegmentV1() *SegmentV1 {
	return &SegmentV1{
		id:      math.MaxUint32,
		Segment: *reader.NewSegment(),
	}
}

// ID returns [SegmentV1] ID.
func (s *SegmentV1) ID() uint32 {
	return s.id
}

// ReadBody reads [SegmentV1] body from r [io.Reader]. The return value n is the number of bytes read.
// Any error encountered during the read is also returned.
func (s *SegmentV1) ReadBody(r io.Reader) (int64, error) {
	return s.ReadFrom(r)
}

// ReadID reads [SegmentV1] ID from r [io.Reader]. The return value n is the number of bytes read.
// Any error encountered during the read is also returned. Implementation [Segment].
func (*SegmentV1) ReadID(io.Reader) (int64, error) { return 0, nil }

// Reset [SegmentV1] data.
func (s *SegmentV1) Reset() {
	s.id = math.MaxUint32
	s.Segment.Reset()
}

// SetSegmentID sets the segment ID value.
func (s *SegmentV1) SetSegmentID(sid uint32) {
	s.id = sid
}

//
// SegmentV2
//

// SegmentV2 encoded segment from wal, version 2.
type SegmentV2 struct {
	reader.SegmentV2
}

// EmptySegmentV2 init new empty [SegmentV2].
func EmptySegmentV2() *SegmentV2 {
	return &SegmentV2{SegmentV2: *reader.NewSegmentV2()}
}

// SetSegmentID sets the segment ID value, implementation [Segment].
func (*SegmentV2) SetSegmentID(uint32) {}
