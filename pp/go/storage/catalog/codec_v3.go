package catalog

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"

	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protowire"
)

// Frame of the record version 3: version | id | payload length | crc32 | payload.
const (
	// RecordVersionV3 version 3 of the record.
	RecordVersionV3 uint8 = 3

	recordIDOffsetV3       = 1
	recordLengthOffsetV3   = recordIDOffsetV3 + len(uuid.UUID{})
	recordChecksumOffsetV3 = recordLengthOffsetV3 + 1
	// recordHeaderSizeV3 size of the frame header of the record version 3.
	recordHeaderSizeV3 = recordChecksumOffsetV3 + 4
	// maxRecordPayloadSizeV3 max size of the payload of the record version 3.
	maxRecordPayloadSizeV3 = math.MaxUint8
	// maxRecordSizeV3 max size of the frame of the record version 3.
	maxRecordSizeV3 = recordHeaderSizeV3 + maxRecordPayloadSizeV3
)

// Protobuf field numbers of the payload of the record version 3.
const (
	protoNumberOfShards protowire.Number = iota + 1
	protoCreatedAt
	protoUpdatedAt
	protoDeletedAt
	protoCorrupted
	protoStatus
	protoSegmentsCount
	protoMinT
	protoMaxT
)

// castagnoliTable is the CRC32 table of the record version 3.
var castagnoliTable = crc32.MakeTable(crc32.Castagnoli)

var (
	// ErrRecordTooLarge the record payload exceeds the max size.
	ErrRecordTooLarge = errors.New("record too large")
	// ErrRecordChecksumMismatch the record checksum does not match the record content.
	ErrRecordChecksumMismatch = errors.New("record checksum mismatch")
)

//
// EncoderV3
//

// EncoderV3 encodes [SerializedRecord] to the frame of the record version 3, only the present fields are encoded.
type EncoderV3 struct {
	buffer []byte
}

// NewEncoderV3 init new [EncoderV3].
func NewEncoderV3() *EncoderV3 {
	return &EncoderV3{
		buffer: make([]byte, 0, maxRecordSizeV3),
	}
}

// EncodeTo encode [SerializedRecord] to [io.Writer] with a single write.
func (e *EncoderV3) EncodeTo(writer io.Writer, sr *SerializedRecord) (err error) {
	if e.buffer, err = appendFrameV3(e.buffer[:0], sr); err != nil {
		return fmt.Errorf("v3: encode record: %w", err)
	}

	if _, err = writer.Write(e.buffer); err != nil {
		return fmt.Errorf("v3: write record: %w", err)
	}

	return nil
}

// appendFrameV3 appends the frame of the record version 3 to dst.
func appendFrameV3(dst []byte, sr *SerializedRecord) ([]byte, error) {
	start := len(dst)
	dst = append(dst, RecordVersionV3)
	dst = append(dst, sr.id[:]...)
	dst = append(dst, 0, 0, 0, 0, 0) // payload length and crc32 are filled in after the payload
	payloadStart := len(dst)
	dst = appendPayloadV3(dst, sr)

	payloadSize := len(dst) - payloadStart
	if payloadSize > maxRecordPayloadSizeV3 {
		return dst[:start], fmt.Errorf("%w: %d bytes", ErrRecordTooLarge, payloadSize)
	}

	frame := dst[start:]
	frame[recordLengthOffsetV3] = uint8(payloadSize)
	binary.LittleEndian.PutUint32(frame[recordChecksumOffsetV3:], checksumV3(frame))

	return dst, nil
}

// appendPayloadV3 appends the present fields of the record as protobuf to dst.
//
//revive:disable-next-line:cyclomatic // one branch per field.
func appendPayloadV3(dst []byte, sr *SerializedRecord) []byte {
	if sr.fields.has(fieldNumberOfShards) {
		dst = appendVarintField(dst, protoNumberOfShards, uint64(sr.numberOfShards))
	}

	if sr.fields.has(fieldCreatedAt) {
		dst = appendVarintField(dst, protoCreatedAt, uint64(sr.createdAt)) // #nosec G115 // two's complement
	}

	if sr.fields.has(fieldUpdatedAt) {
		dst = appendVarintField(dst, protoUpdatedAt, uint64(sr.updatedAt)) // #nosec G115 // two's complement
	}

	if sr.fields.has(fieldDeletedAt) {
		dst = appendVarintField(dst, protoDeletedAt, uint64(sr.deletedAt)) // #nosec G115 // two's complement
	}

	if sr.fields.has(fieldCorrupted) {
		dst = appendVarintField(dst, protoCorrupted, protowire.EncodeBool(sr.corrupted))
	}

	if sr.fields.has(fieldStatus) {
		dst = appendVarintField(dst, protoStatus, uint64(sr.status))
	}

	if sr.fields.has(fieldSegmentsCount) {
		dst = appendVarintField(dst, protoSegmentsCount, uint64(sr.numberOfSegments))
	}

	if sr.fields.has(fieldMinT) {
		dst = appendVarintField(dst, protoMinT, uint64(sr.mint)) // #nosec G115 // two's complement
	}

	if sr.fields.has(fieldMaxT) {
		dst = appendVarintField(dst, protoMaxT, uint64(sr.maxt)) // #nosec G115 // two's complement
	}

	return dst
}

// appendVarintField appends the protobuf varint field to dst.
func appendVarintField(dst []byte, number protowire.Number, value uint64) []byte {
	dst = protowire.AppendTag(dst, number, protowire.VarintType)
	return protowire.AppendVarint(dst, value)
}

// checksumV3 returns the crc32 of the frame of the record version 3 without the checksum field.
func checksumV3(frame []byte) uint32 {
	checksum := crc32.Update(0, castagnoliTable, frame[:recordChecksumOffsetV3])
	return crc32.Update(checksum, castagnoliTable, frame[recordHeaderSizeV3:])
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
