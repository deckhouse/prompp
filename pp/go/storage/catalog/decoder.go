package catalog

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protowire"

	"github.com/prometheus/prometheus/pp/go/util/optional"
)

//
// DecoderV1
//

// DecoderV1 decodes [SerializedRecord], version 1.
//
//	Deprecated: For backward compatibility.
type DecoderV1 struct{}

// DecodeFrom decode [SerializedRecord] from [io.Reader].
//
//revive:disable-next-line:cyclomatic this is decode.
func (DecoderV1) DecodeFrom(reader io.Reader, sr *SerializedRecord) (err error) {
	var size uint64
	if err = binary.Read(reader, binary.LittleEndian, &size); err != nil {
		return fmt.Errorf("read id size: %w", err)
	}

	defer func() {
		if err != nil && errors.Is(err, io.EOF) {
			err = fmt.Errorf("%s: %w", err.Error(), io.ErrUnexpectedEOF)
		}
	}()

	buf := make([]byte, size)
	if _, err = reader.Read(buf); err != nil {
		return fmt.Errorf("read id: %w", err)
	}
	sr.id = uuid.MustParse(string(buf))

	if err = binary.Read(reader, binary.LittleEndian, &size); err != nil {
		return fmt.Errorf("read dir size: %w", err)
	}

	buf = make([]byte, size)
	if _, err = reader.Read(buf); err != nil {
		return fmt.Errorf("read dir: %w", err)
	}

	if err = binary.Read(reader, binary.LittleEndian, &sr.numberOfShards); err != nil {
		return fmt.Errorf("read number of shards: %w", err)
	}

	if err = binary.Read(reader, binary.LittleEndian, &sr.createdAt); err != nil {
		return fmt.Errorf("read created at: %w", err)
	}

	if err = binary.Read(reader, binary.LittleEndian, &sr.updatedAt); err != nil {
		return fmt.Errorf("read updated at: %w", err)
	}

	if err = binary.Read(reader, binary.LittleEndian, &sr.deletedAt); err != nil {
		return fmt.Errorf("read deleted at: %w", err)
	}

	if err = binary.Read(reader, binary.LittleEndian, &sr.status); err != nil {
		return fmt.Errorf("read status: %w", err)
	}
	sr.fields = fieldsSnapshotV2

	return nil
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

//
// DecoderV3
//

// DecoderV3 decodes [SerializedRecord] from the frame of the record version 3, sets only the present fields
// and their mask.
type DecoderV3 struct {
	buffer [maxRecordSizeV3]byte
}

// NewDecoderV3 init new [DecoderV3].
func NewDecoderV3() *DecoderV3 {
	return &DecoderV3{}
}

// DecodeFrom decode [SerializedRecord] from [io.Reader]. Returns [io.EOF] if the reader has no more records
// and [io.ErrUnexpectedEOF] if the frame is incomplete.
func (d *DecoderV3) DecodeFrom(reader io.Reader, sr *SerializedRecord) error {
	if _, err := io.ReadFull(reader, d.buffer[:1]); err != nil {
		return err
	}

	if d.buffer[0] != RecordVersionV3 {
		return fmt.Errorf("%w: %d", ErrUnsupportedRecordVersion, d.buffer[0])
	}

	if _, err := io.ReadFull(reader, d.buffer[1:recordHeaderSizeV3]); err != nil {
		return fmt.Errorf("v3: read record header: %w", unexpectedEOF(err))
	}

	size := recordHeaderSizeV3 + int(d.buffer[recordLengthOffsetV3])
	if _, err := io.ReadFull(reader, d.buffer[recordHeaderSizeV3:size]); err != nil {
		return fmt.Errorf("v3: read record payload: %w", unexpectedEOF(err))
	}

	_, err := decodeFrame(d.buffer[:size], sr)
	return err
}

// decodeFrame decodes the record frame of any known version from the beginning of buf.
// Returns the size of the frame, it is also returned with [ErrRecordChecksumMismatch] as the frame header states.
func decodeFrame(buf []byte, sr *SerializedRecord) (int, error) {
	if len(buf) == 0 {
		return 0, io.ErrUnexpectedEOF
	}

	switch buf[0] {
	case RecordVersionV3:
		return decodeFrameV3(buf, sr)
	default:
		return 0, fmt.Errorf("%w: %d", ErrUnsupportedRecordVersion, buf[0])
	}
}

// decodeFrameV3 decodes the frame of the record version 3 from the beginning of buf.
func decodeFrameV3(buf []byte, sr *SerializedRecord) (int, error) {
	if len(buf) < recordHeaderSizeV3 {
		return 0, io.ErrUnexpectedEOF
	}

	size := recordHeaderSizeV3 + int(buf[recordLengthOffsetV3])
	if len(buf) < size {
		return 0, io.ErrUnexpectedEOF
	}

	frame := buf[:size]
	if binary.LittleEndian.Uint32(frame[recordChecksumOffsetV3:]) != checksumV3(frame) {
		return size, ErrRecordChecksumMismatch
	}

	sr.id = uuid.UUID(frame[recordIDOffsetV3:recordLengthOffsetV3])
	if err := decodePayloadV3(frame[recordHeaderSizeV3:], sr); err != nil {
		return size, fmt.Errorf("v3: decode payload: %w", err)
	}

	return size, nil
}

// decodePayloadV3 decodes the protobuf payload of the record version 3, unknown fields are skipped.
func decodePayloadV3(payload []byte, sr *SerializedRecord) error {
	var fields fieldMask
	for len(payload) > 0 {
		number, wireType, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return protowire.ParseError(n)
		}
		payload = payload[n:]

		field := fieldByProtoNumber(number)
		if field == 0 {
			if n = protowire.ConsumeFieldValue(number, wireType, payload); n < 0 {
				return protowire.ParseError(n)
			}
			payload = payload[n:]
			continue
		}

		if wireType != protowire.VarintType {
			return fmt.Errorf("field %d: unexpected wire type %d", number, wireType)
		}

		value, n := protowire.ConsumeVarint(payload)
		if n < 0 {
			return protowire.ParseError(n)
		}
		payload = payload[n:]

		if err := setFieldV3(sr, field, value); err != nil {
			return fmt.Errorf("field %d: %w", number, err)
		}
		fields |= field
	}

	sr.fields = fields
	if fields.has(fieldSegmentsCount) {
		sr.lastAppendedSegmentID = lastAppendedSegmentIDByNumberOfSegments(sr.numberOfSegments)
	}

	return nil
}

// fieldByProtoNumber returns the field of the protobuf field number, 0 for an unknown number.
func fieldByProtoNumber(number protowire.Number) fieldMask {
	if number < protoNumberOfShards || number > protoMaxT {
		return 0
	}

	return fieldNumberOfShards << (number - protoNumberOfShards)
}

// setFieldV3 sets the value of the field to the record.
//
//revive:disable-next-line:cyclomatic // one branch per field.
func setFieldV3(sr *SerializedRecord, field fieldMask, value uint64) error {
	switch field {
	case fieldNumberOfShards:
		if value > math.MaxUint16 {
			return errValueOutOfRange(value)
		}
		sr.numberOfShards = uint16(value)
	case fieldCreatedAt:
		sr.createdAt = int64(value) // #nosec G115 // two's complement
	case fieldUpdatedAt:
		sr.updatedAt = int64(value) // #nosec G115 // two's complement
	case fieldDeletedAt:
		sr.deletedAt = int64(value) // #nosec G115 // two's complement
	case fieldCorrupted:
		sr.corrupted = protowire.DecodeBool(value)
	case fieldStatus:
		if value > math.MaxUint8 {
			return errValueOutOfRange(value)
		}
		sr.status = Status(value)
	case fieldSegmentsCount:
		if value > math.MaxUint32 {
			return errValueOutOfRange(value)
		}
		sr.numberOfSegments = uint32(value)
	case fieldMinT:
		sr.mint = int64(value) // #nosec G115 // two's complement
	case fieldMaxT:
		sr.maxt = int64(value) // #nosec G115 // two's complement
	}

	return nil
}

// errValueOutOfRange returns the error for the value that does not fit into the field.
func errValueOutOfRange(value uint64) error {
	return fmt.Errorf("value out of range: %d", value)
}

// unexpectedEOF converts [io.EOF] in the middle of the frame to [io.ErrUnexpectedEOF].
func unexpectedEOF(err error) error {
	if errors.Is(err, io.EOF) {
		return io.ErrUnexpectedEOF
	}

	return err
}
