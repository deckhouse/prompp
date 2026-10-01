package catalog

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"

	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protowire"

	"github.com/prometheus/prometheus/pp/go/util/optional"
)

const (
	// RecordStructMaxSizeV2 max size of [SerializedRecord] for [EncoderV2].
	RecordStructMaxSizeV2 = 50
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
	// ErrUnsupportedRecordVersion the record version is unknown.
	ErrUnsupportedRecordVersion = errors.New("unsupported record version")
)

//
// EncoderV1
//

// EncoderV1 encodes [SerializedRecord], version 1.
//
//	Deprecated.
type EncoderV1 struct{}

// EncodeTo encode [SerializedRecord] to [io.Writer].
func (EncoderV1) EncodeTo(writer io.Writer, sr *SerializedRecord) (err error) {
	if err = encodeString(writer, sr.id.String()); err != nil {
		return fmt.Errorf("v1: encode id: %w", err)
	}

	if err = encodeString(writer, sr.id.String()); err != nil {
		return fmt.Errorf("v1: encode dir: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &sr.numberOfShards); err != nil {
		return fmt.Errorf("v1: write number of shards: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &sr.createdAt); err != nil {
		return fmt.Errorf("v1: write created at: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &sr.updatedAt); err != nil {
		return fmt.Errorf("v1: write updated at: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &sr.deletedAt); err != nil {
		return fmt.Errorf("v1: write deleted at: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &sr.status); err != nil {
		return fmt.Errorf("v1: write status: %w", err)
	}

	return nil
}

// encodeString encode string to [io.Writer].
func encodeString(writer io.Writer, value string) (err error) {
	if err = binary.Write(writer, binary.LittleEndian, uint64(len(value))); err != nil {
		return fmt.Errorf("write string length: %w", err)
	}

	if _, err = writer.Write([]byte(value)); err != nil {
		return fmt.Errorf("write string: %w", err)
	}

	return nil
}

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
