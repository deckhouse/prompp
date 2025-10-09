package catalog

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash"
	"hash/crc32"
	"io"

	"github.com/prometheus/prometheus/pp/go/util/optional"
)

const (
	// RecordStructMaxSizeV2 max size of [Record] for [EncoderV2].
	RecordStructMaxSizeV2 = 50
	// RecordFrameSizeV3 size of frame [Record] for [EncoderV3].
	RecordFrameSizeV3 = 68
)

//
// EncoderV1
//

// EncoderV1 encodes [Record], version 1.
//
//	Deprecated.
type EncoderV1 struct{}

// EncodeTo encode [Record] to [io.Writer].
func (EncoderV1) EncodeTo(writer io.Writer, r *Record) (err error) {
	if err = encodeString(writer, r.id.String()); err != nil {
		return fmt.Errorf("v1: encode id: %w", err)
	}

	if err = encodeString(writer, r.id.String()); err != nil {
		return fmt.Errorf("v1: encode dir: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &r.numberOfShards); err != nil {
		return fmt.Errorf("v1: write number of shards: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &r.createdAt); err != nil {
		return fmt.Errorf("v1: write created at: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &r.updatedAt); err != nil {
		return fmt.Errorf("v1: write updated at: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &r.deletedAt); err != nil {
		return fmt.Errorf("v1: write deleted at: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &r.status); err != nil {
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

// EncoderV2 encodes [Record], version 2.
type EncoderV2 struct {
	buffer *bytes.Buffer
}

// NewEncoderV2 init new [EncoderV2].
func NewEncoderV2() *EncoderV2 {
	return &EncoderV2{
		buffer: bytes.NewBuffer(make([]byte, 0, RecordStructMaxSizeV2)),
	}
}

// EncodeTo encode [Record] to [io.Writer].
//
//revive:disable-next-line:cyclomatic this is encode.
//revive:disable-next-line:function-length long but this is encode.
func (e *EncoderV2) EncodeTo(writer io.Writer, r *Record) (err error) {
	e.buffer.Reset()

	if err = binary.Write(e.buffer, binary.LittleEndian, uint8(0)); err != nil {
		return fmt.Errorf("v2: encode size filler: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, r.id); err != nil {
		return fmt.Errorf("v2: encode id: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.numberOfShards); err != nil {
		return fmt.Errorf("v2: write number of shards: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.createdAt); err != nil {
		return fmt.Errorf("v2: write created at: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.updatedAt); err != nil {
		return fmt.Errorf("v2: write updated at: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.deletedAt); err != nil {
		return fmt.Errorf("v2: write deleted at: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.corrupted); err != nil {
		return fmt.Errorf("v2: write corrupted: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.status); err != nil {
		return fmt.Errorf("v2: write status: %w", err)
	}

	if err = encodeOptionalValue(e.buffer, binary.LittleEndian, r.lastAppendedSegmentID); err != nil {
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

// EncoderV3 encodes [Record], version 3.
type EncoderV3 struct {
	buffer      *bytes.Buffer
	crc32Hasher hash.Hash32
}

// NewEncoderV3 init new [EncoderV3].
func NewEncoderV3() *EncoderV3 {
	return &EncoderV3{
		buffer:      bytes.NewBuffer(make([]byte, 0, RecordFrameSizeV3+1)), // +1 is for size byte
		crc32Hasher: crc32.NewIEEE(),
	}
}

// EncodeTo encode [Record] to [io.Writer].
//
//revive:disable-next-line:cyclomatic this is encode.
//revive:disable-next-line:function-length long but this is encode.
func (e *EncoderV3) EncodeTo(writer io.Writer, r *Record) (err error) {
	e.buffer.Reset()

	if err = binary.Write(e.buffer, binary.LittleEndian, uint8(0)); err != nil {
		return fmt.Errorf("v3: encode size filler: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, uint32(0)); err != nil {
		return fmt.Errorf("v3: encode crc32 filler: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, r.id); err != nil {
		return fmt.Errorf("v3: encode id: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.numberOfShards); err != nil {
		return fmt.Errorf("v3: write number of shards: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.createdAt); err != nil {
		return fmt.Errorf("v3: write created at: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.updatedAt); err != nil {
		return fmt.Errorf("v3: write updated at: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.deletedAt); err != nil {
		return fmt.Errorf("v3: write deleted at: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.corrupted); err != nil {
		return fmt.Errorf("v3: write corrupted: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.status); err != nil {
		return fmt.Errorf("v3: write status: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.numberOfSegments); err != nil {
		return fmt.Errorf("v3: write number of segments: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.mint); err != nil {
		return fmt.Errorf("v3: write min timestamp: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.maxt); err != nil {
		return fmt.Errorf("v3: write max timestamp: %w", err)
	}

	e.buffer.Bytes()[0] = uint8(len(e.buffer.Bytes()) - 1) // #nosec G115 // no overflow

	e.crc32Hasher.Reset()
	_, err = e.crc32Hasher.Write(e.buffer.Bytes()[5:])
	if err != nil {
		return fmt.Errorf("v3: write hash: %w", err)
	}

	var binaryCRC32 [4]byte
	binary.LittleEndian.PutUint32(binaryCRC32[:], e.crc32Hasher.Sum32())
	copy(e.buffer.Bytes()[1:5], binaryCRC32[:])

	if _, err = e.buffer.WriteTo(writer); err != nil {
		return fmt.Errorf("v3: write record: %w", err)
	}

	return nil
}
