package catalog

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/prometheus/prometheus/pp/go/util/optional"
)

const (
	// RecordStructMaxSizeV2 max size of [SerializedRecord] for [EncoderV2].
	RecordStructMaxSizeV2 = 50
)

//
// EncoderV2
//

// EncoderV2 encodes [SerializedRecord], version 2.
type EncoderV2 struct {
	buffer *bytes.Buffer
}

// NewEncoderV2 init new [EncoderV2].
func NewEncoderV2() *EncoderV2 {
	return &EncoderV2{
		buffer: bytes.NewBuffer(make([]byte, 0, RecordStructMaxSizeV2)),
	}
}

// EncodeTo encode [SerializedRecord] to [io.Writer].
//
//revive:disable-next-line:cyclomatic this is encode.
//revive:disable-next-line:function-length long but this is encode.
func (e *EncoderV2) EncodeTo(writer io.Writer, sr *SerializedRecord) (err error) {
	e.buffer.Reset()

	if err = binary.Write(e.buffer, binary.LittleEndian, uint8(0)); err != nil {
		return fmt.Errorf("v2: encode size filler: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, sr.id); err != nil {
		return fmt.Errorf("v2: encode id: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &sr.numberOfShards); err != nil {
		return fmt.Errorf("v2: write number of shards: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &sr.createdAt); err != nil {
		return fmt.Errorf("v2: write created at: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &sr.updatedAt); err != nil {
		return fmt.Errorf("v2: write updated at: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &sr.deletedAt); err != nil {
		return fmt.Errorf("v2: write deleted at: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &sr.corrupted); err != nil {
		return fmt.Errorf("v2: write corrupted: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &sr.status); err != nil {
		return fmt.Errorf("v2: write status: %w", err)
	}

	if err = encodeOptionalValue(e.buffer, binary.LittleEndian, sr.lastAppendedSegmentID); err != nil {
		return fmt.Errorf("v2: write last written segment id: %w", err)
	}

	e.buffer.Bytes()[0] = uint8(len(e.buffer.Bytes()) - 1) // #nosec G115 // no overflow

	if _, err = e.buffer.WriteTo(writer); err != nil {
		return fmt.Errorf("v2: write record: %w", err)
	}

	return nil
}

// encodeOptionalValue encode [optional.Optional[T]] to [io.Writer].
func encodeOptionalValue[T any](writer io.Writer, byteOrder binary.ByteOrder, value optional.Optional[T]) (err error) {
	var nilIndicator uint8
	if value.IsNil() {
		return binary.Write(writer, byteOrder, nilIndicator)
	}

	nilIndicator = 1
	if err = binary.Write(writer, byteOrder, nilIndicator); err != nil {
		return err
	}

	return binary.Write(writer, byteOrder, value.Value())
}

//
// DecoderV2
//

// DecoderV2 decodes [SerializedRecord], version 2.
type DecoderV2 struct{}

// DecodeFrom decode [SerializedRecord] from [io.Reader].
//
//revive:disable-next-line:cyclomatic this is decode.
//revive:disable-next-line:function-length long but this is decode.
func (DecoderV2) DecodeFrom(reader io.Reader, sr *SerializedRecord) (err error) {
	var size uint8
	if err = binary.Read(reader, binary.LittleEndian, &size); err != nil {
		return fmt.Errorf("read record size: %w", err)
	}

	rReader := newReaderWithCounter(reader)

	defer func() {
		if err != nil && errors.Is(err, io.EOF) || int(size) != rReader.BytesRead() {
			if err == nil {
				err = fmt.Errorf("bytes read: %d, bytes expected: %d", rReader.BytesRead(), size)
			}
			err = fmt.Errorf("%s: %w", err.Error(), io.ErrUnexpectedEOF)
		}
	}()

	if err = binary.Read(rReader, binary.LittleEndian, &sr.id); err != nil {
		return fmt.Errorf("read record id: %w", err)
	}

	if err = binary.Read(rReader, binary.LittleEndian, &sr.numberOfShards); err != nil {
		return fmt.Errorf("read number of shards: %w", err)
	}

	if err = binary.Read(rReader, binary.LittleEndian, &sr.createdAt); err != nil {
		return fmt.Errorf("read created at: %w", err)
	}

	if err = binary.Read(rReader, binary.LittleEndian, &sr.updatedAt); err != nil {
		return fmt.Errorf("read updated at: %w", err)
	}

	if err = binary.Read(rReader, binary.LittleEndian, &sr.deletedAt); err != nil {
		return fmt.Errorf("read deleted at: %w", err)
	}

	if err = binary.Read(rReader, binary.LittleEndian, &sr.corrupted); err != nil {
		return fmt.Errorf("read currupted: %w", err)
	}

	if err = binary.Read(rReader, binary.LittleEndian, &sr.status); err != nil {
		return fmt.Errorf("read status: %w", err)
	}

	if err = decodeOptionalValue(rReader, binary.LittleEndian, &sr.lastAppendedSegmentID); err != nil {
		return fmt.Errorf("read last written segment id: %w", err)
	}
	sr.numberOfSegments = numberOfSegmentsByLastAppendedSegmentID(sr.lastAppendedSegmentID.RawValue())
	sr.fields = fieldsSnapshotV2

	return nil
}

// readerWithCounter reader with a counter of read bytes.
type readerWithCounter struct {
	reader io.Reader
	n      int
}

// newReaderWithCounter init new [readerWithCounter].
func newReaderWithCounter(reader io.Reader) *readerWithCounter {
	return &readerWithCounter{reader: reader}
}

// Read reads up to len(p) bytes into p.
func (r *readerWithCounter) Read(p []byte) (n int, err error) {
	n, err = r.reader.Read(p)
	r.n += n
	return n, err
}

// BytesRead return a counter of read bytes.
func (r *readerWithCounter) BytesRead() int {
	return r.n
}

// decodeOptionalValue decode [optional.Optional[T]] from [io.Reader].
func decodeOptionalValue[T any](
	reader io.Reader,
	byteOrder binary.ByteOrder,
	valueRef *optional.Optional[T],
) (err error) {
	var nilIndicator uint8
	if err = binary.Read(reader, byteOrder, &nilIndicator); err != nil {
		return err
	}
	if nilIndicator == 0 {
		return nil
	}

	var value T
	if err = binary.Read(reader, byteOrder, &value); err != nil {
		return err
	}
	valueRef.Set(value)
	return nil
}
