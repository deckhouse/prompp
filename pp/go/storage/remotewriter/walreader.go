package remotewriter

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/reader"
	"github.com/prometheus/prometheus/pp/go/util"
)

// walReader buffered reader [Segment]s from wal.
type walReader struct {
	file              *util.FileReader
	reader            io.Reader
	fsize             int64  // file wal size
	nextSegmentID     uint32 // next segment ID to read
	fileFormatVersion uint8  // file format version
	encoderVersion    uint8  // encoder version used to encode the wal file, required to initialize the decoder
}

// newWalReader creates a new [walReader].
// It returns the [walReader] and an error if any.
func newWalReader(fileName string) (*walReader, error) {
	file, err := util.OpenFileReader(fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to read wal file: %w", err)
	}

	fileFormatVersion, encoderVersion, n, err := reader.ReadHeader(file)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("failed to read header: %w", err), file.Close())
	}

	wr := &walReader{
		file:              file,
		reader:            bufio.NewReaderSize(file, 4096), //revive:disable-line:add-constant // 4kb
		fileFormatVersion: fileFormatVersion,
		encoderVersion:    encoderVersion,
	}
	wr.fsize -= int64(n)

	return wr, nil
}

// newWalReaderRotated creates a new rotated [walReader].
// It returns the [walReader] and an error if any.
func newWalReaderRotated(fileName string) (*walReader, error) {
	info, err := os.Stat(fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to stat wal file: %w", err)
	}

	fsize := info.Size()
	if fsize == 0 {
		return nil, fmt.Errorf("wal file is empty")
	}

	wr, err := newWalReader(fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to create wal file rotated reader: %w", err)
	}
	wr.fsize += fsize

	return wr, nil
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

// EncoderVersion returns the encoder version.
func (r *walReader) EncoderVersion() uint8 {
	return r.encoderVersion
}

// IsEOF returns true if the wal file is at the end.
func (r *walReader) IsEOF() bool {
	return r.fsize <= 0
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
	n, err := s.ReadBody(r.reader)
	r.fsize -= n
	if err != nil {
		return fmt.Errorf("failed to read segment body: %w", err)
	}

	r.nextSegmentID++

	return nil
}

// ReadSegmentID reads [Segment] ID from wal.
// It may return a non-nil error if some error condition is known, such as EOF.
func (r *walReader) ReadSegmentID(s Segment) error {
	n, err := s.ReadID(r.reader)
	r.fsize -= n
	if err != nil {
		return fmt.Errorf("failed to read segment id: %w", err)
	}

	// v1: set segment ID
	// v2: read segment ID from wal
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
		id:      reader.UnknownSegmentID,
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
	s.id = reader.UnknownSegmentID
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
