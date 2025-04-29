package catalog

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/util/optional"
	"hash/crc32"
	"io"
)

const (
	RecordStructMaxSizeV2 = 50
	RecordFrameSizeV3     = 68
)

type EncoderV1 struct {
}

func (EncoderV1) Encode(writer io.Writer, r *Record) (err error) {
	if err = encodeString(writer, r.id.String()); err != nil {
		return fmt.Errorf("encode id: %w", err)
	}

	if err = encodeString(writer, r.id.String()); err != nil {
		return fmt.Errorf("encode dir: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &r.numberOfShards); err != nil {
		return fmt.Errorf("write number of shards: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &r.createdAt); err != nil {
		return fmt.Errorf("write created at: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &r.updatedAt); err != nil {
		return fmt.Errorf("write updated at: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &r.deletedAt); err != nil {
		return fmt.Errorf("write deleted at: %w", err)
	}

	if err = binary.Write(writer, binary.LittleEndian, &r.status); err != nil {
		return fmt.Errorf("write status: %w", err)
	}

	return nil
}

func encodeString(writer io.Writer, value string) (err error) {
	if err = binary.Write(writer, binary.LittleEndian, uint64(len(value))); err != nil {
		return fmt.Errorf("write string length: %w", err)
	}

	if _, err = writer.Write([]byte(value)); err != nil {
		return fmt.Errorf("write string: %w", err)
	}

	return nil
}

type EncoderV2 struct {
	buffer *bytes.Buffer
}

func NewEncoderV2() *EncoderV2 {
	return &EncoderV2{
		buffer: bytes.NewBuffer(make([]byte, 0, RecordStructMaxSizeV2)),
	}
}

func (e *EncoderV2) Encode(writer io.Writer, r *Record) (err error) {
	e.buffer.Reset()

	if err = binary.Write(e.buffer, binary.LittleEndian, uint8(0)); err != nil {
		return fmt.Errorf("encode size filler: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, r.id); err != nil {
		return fmt.Errorf("encode id: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.numberOfShards); err != nil {
		return fmt.Errorf("write number of shards: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.createdAt); err != nil {
		return fmt.Errorf("write created at: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.updatedAt); err != nil {
		return fmt.Errorf("write updated at: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.deletedAt); err != nil {
		return fmt.Errorf("write deleted at: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.corrupted); err != nil {
		return fmt.Errorf("write corrupted: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.status); err != nil {
		return fmt.Errorf("write status: %w", err)
	}

	if err = encodeOptionalValue(e.buffer, binary.LittleEndian, r.lastAppendedSegmentID); err != nil {
		return fmt.Errorf("write last written segment id: %w", err)
	}

	e.buffer.Bytes()[0] = uint8(len(e.buffer.Bytes()) - 1)

	if _, err = e.buffer.WriteTo(writer); err != nil {
		return fmt.Errorf("write record: %w", err)
	}

	return nil
}

func encodeOptionalValue[T any](writer io.Writer, byteOrder binary.ByteOrder, value optional.Optional[T]) (err error) {
	var nilIndicator uint8
	if value.IsNil() {
		return binary.Write(writer, byteOrder, nilIndicator)
	}
	nilIndicator = 1
	if err = binary.Write(writer, byteOrder, nilIndicator); err != nil {
		return err
	}
	if err = binary.Write(writer, byteOrder, value.Value()); err != nil {
		return err
	}

	return nil
}

// EncoderV3 encodes record.
type EncoderV3 struct {
	buffer *bytes.Buffer
}

// NewEncoderV3 creates EncoderV3.
func NewEncoderV3() *EncoderV3 {
	return &EncoderV3{
		buffer: bytes.NewBuffer(make([]byte, 0, RecordFrameSizeV3+1)), // +1 is for size byte
	}
}

// Encode is an Encoder interface implementation.
func (e *EncoderV3) Encode(writer io.Writer, r *Record) (err error) {
	e.buffer.Reset()

	if err = binary.Write(e.buffer, binary.LittleEndian, uint8(0)); err != nil {
		return fmt.Errorf("encode size filler: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, uint32(0)); err != nil {
		return fmt.Errorf("encode crc32 filler: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, r.id); err != nil {
		return fmt.Errorf("encode id: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.numberOfShards); err != nil {
		return fmt.Errorf("write number of shards: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.createdAt); err != nil {
		return fmt.Errorf("write created at: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.updatedAt); err != nil {
		return fmt.Errorf("write updated at: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.deletedAt); err != nil {
		return fmt.Errorf("write deleted at: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.corrupted); err != nil {
		return fmt.Errorf("write corrupted: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.status); err != nil {
		return fmt.Errorf("write status: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.numberOfSegments); err != nil {
		return fmt.Errorf("write number of segments: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.mint); err != nil {
		return fmt.Errorf("write min timestamp: %w", err)
	}

	if err = binary.Write(e.buffer, binary.LittleEndian, &r.maxt); err != nil {
		return fmt.Errorf("write max timestamp: %w", err)
	}

	e.buffer.Bytes()[0] = uint8(len(e.buffer.Bytes()) - 1)

	crc32Hasher := crc32.NewIEEE()
	_, err = crc32Hasher.Write(e.buffer.Bytes()[5:])
	if err != nil {
		return fmt.Errorf("write hash: %w", err)
	}

	var binaryCRC32 [4]byte
	binary.LittleEndian.PutUint32(binaryCRC32[:], crc32Hasher.Sum32())
	copy(e.buffer.Bytes()[1:5], binaryCRC32[:])

	if _, err = e.buffer.WriteTo(writer); err != nil {
		return fmt.Errorf("write record: %w", err)
	}

	return nil
}
