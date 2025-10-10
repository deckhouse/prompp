package catalog

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"hash/crc32"
	"io"

	"github.com/google/uuid"

	"github.com/prometheus/prometheus/pp/go/util/optional"
)

const (
	// size of uint32.
	sizeOfUint32 = 4
	// size of int64 or uint64.
	sizeOf64 = 8
)

//
// DecoderV1
//

// DecoderV1 decodes [Record], version 1.
//
//	Deprecated: For backward compatibility.
type DecoderV1 struct{}

// DecodeFrom decode [Record] from [io.Reader].
//
//revive:disable-next-line:cyclomatic this is decode.
func (DecoderV1) DecodeFrom(reader io.Reader, r *Record) (err error) {
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
	r.id = uuid.MustParse(string(buf))

	if err = binary.Read(reader, binary.LittleEndian, &size); err != nil {
		return fmt.Errorf("read dir size: %w", err)
	}

	buf = make([]byte, size)
	if _, err = reader.Read(buf); err != nil {
		return fmt.Errorf("read dir: %w", err)
	}

	if err = binary.Read(reader, binary.LittleEndian, &r.numberOfShards); err != nil {
		return fmt.Errorf("read number of shards: %w", err)
	}

	if err = binary.Read(reader, binary.LittleEndian, &r.createdAt); err != nil {
		return fmt.Errorf("read created at: %w", err)
	}

	if err = binary.Read(reader, binary.LittleEndian, &r.updatedAt); err != nil {
		return fmt.Errorf("read updated at: %w", err)
	}

	if err = binary.Read(reader, binary.LittleEndian, &r.deletedAt); err != nil {
		return fmt.Errorf("read deleted at: %w", err)
	}

	if err = binary.Read(reader, binary.LittleEndian, &r.status); err != nil {
		return fmt.Errorf("read status: %w", err)
	}

	return nil
}

//
// DecoderV2
//

// DecoderV2 decodes [Record], version 2.
type DecoderV2 struct{}

// DecodeFrom decode [Record] from [io.Reader].
//
//revive:disable-next-line:cyclomatic this is decode.
//revive:disable-next-line:function-length long but this is decode.
func (DecoderV2) DecodeFrom(reader io.Reader, r *Record) (err error) {
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

	if err = binary.Read(rReader, binary.LittleEndian, &r.id); err != nil {
		return fmt.Errorf("read record id: %w", err)
	}

	if err = binary.Read(rReader, binary.LittleEndian, &r.numberOfShards); err != nil {
		return fmt.Errorf("read number of shards: %w", err)
	}

	if err = binary.Read(rReader, binary.LittleEndian, &r.createdAt); err != nil {
		return fmt.Errorf("read created at: %w", err)
	}

	if err = binary.Read(rReader, binary.LittleEndian, &r.updatedAt); err != nil {
		return fmt.Errorf("read updated at: %w", err)
	}

	if err = binary.Read(rReader, binary.LittleEndian, &r.deletedAt); err != nil {
		return fmt.Errorf("read deleted at: %w", err)
	}

	if err = binary.Read(rReader, binary.LittleEndian, &r.corrupted); err != nil {
		return fmt.Errorf("read currupted: %w", err)
	}

	if err = binary.Read(rReader, binary.LittleEndian, &r.status); err != nil {
		return fmt.Errorf("read status: %w", err)
	}

	if err = decodeOptionalValue(rReader, binary.LittleEndian, &r.lastAppendedSegmentID); err != nil {
		return fmt.Errorf("read last written segment id: %w", err)
	}

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

// DecoderV3 decodes [Record], version 3.
type DecoderV3 struct {
	offset int
	size   uint8
	buffer [RecordFrameSizeV3]byte
	hasher hash.Hash32
}

// NewDecoderV3 init new [DecoderV3].
func NewDecoderV3() *DecoderV3 {
	return &DecoderV3{
		hasher: crc32.NewIEEE(),
	}
}

// DecodeFrom decode [Record] from [io.Reader].
//
//revive:disable-next-line:cyclomatic this is decode.
//revive:disable-next-line:function-length long but this is decode.
func (d *DecoderV3) DecodeFrom(reader io.Reader, r *Record) (err error) {
	d.reset()

	if err = d.readSize(reader); err != nil {
		return err
	}

	defer func() {
		if err != nil && errors.Is(err, io.EOF) {
			err = fmt.Errorf("%s: %w", err.Error(), io.ErrUnexpectedEOF)
		}
	}()

	if err = d.readRecord(reader); err != nil {
		return err
	}

	if err = d.validateCRC32(); err != nil {
		return fmt.Errorf("read crc32: %w", err)
	}

	targetOffset := d.offset + 16 //revive:disable-line:add-constant it's size of UUID
	r.id = uuid.UUID(d.buffer[d.offset:targetOffset])
	d.offset = targetOffset

	r.numberOfShards = binary.LittleEndian.Uint16(d.buffer[d.offset:])
	d.offset += 2 //revive:disable-line:add-constant it's size of uint16

	r.createdAt = int64(binary.LittleEndian.Uint64(d.buffer[d.offset:])) // #nosec G115 // no overflow
	d.offset += sizeOf64

	r.updatedAt = int64(binary.LittleEndian.Uint64(d.buffer[d.offset:])) // #nosec G115 // no overflow
	d.offset += sizeOf64

	r.deletedAt = int64(binary.LittleEndian.Uint64(d.buffer[d.offset:])) // #nosec G115 // no overflow
	d.offset += sizeOf64

	r.corrupted = d.buffer[d.offset] > 0
	d.offset++

	r.status = Status(d.buffer[d.offset])
	d.offset++

	r.numberOfSegments = binary.LittleEndian.Uint32(d.buffer[d.offset:])
	d.offset += sizeOfUint32

	r.mint = int64(binary.LittleEndian.Uint64(d.buffer[d.offset:])) // #nosec G115 // no overflow
	d.offset += sizeOf64

	r.maxt = int64(binary.LittleEndian.Uint64(d.buffer[d.offset:])) // #nosec G115 // no overflow
	d.offset += sizeOf64

	return nil
}

// reset state of decoder.
func (d *DecoderV3) reset() {
	d.offset = 0
	d.size = 0
	d.hasher.Reset()
}

// readSize read size of buffer from [io.Reader].
func (d *DecoderV3) readSize(reader io.Reader) error {
	if _, err := reader.Read(d.buffer[:1]); err != nil {
		return fmt.Errorf("read record size: %w", err)
	}
	d.size = d.buffer[0]

	if int(d.size) != len(d.buffer) {
		return fmt.Errorf("invalid size: %d", d.size)
	}

	return nil
}

// readRecord read [Record] from [io.Reader].
func (d *DecoderV3) readRecord(reader io.Reader) error {
	if _, err := reader.Read(d.buffer[:d.size]); err != nil {
		return fmt.Errorf("read whole record: %w", err)
	}
	return nil
}

// validateCRC32 validate [Record] on CRC32.
func (d *DecoderV3) validateCRC32() (err error) {
	expectedCRC32Hash := binary.LittleEndian.Uint32(d.buffer[d.offset:])
	d.offset += sizeOfUint32

	if _, err = d.hasher.Write(d.buffer[d.offset:]); err != nil {
		return fmt.Errorf("write to crc32 hasher: %w", err)
	}

	actualCRC32Hash := d.hasher.Sum32()
	if expectedCRC32Hash != actualCRC32Hash {
		return fmt.Errorf("invalid crc32: expected: %d, actual: %d", expectedCRC32Hash, actualCRC32Hash)
	}

	return nil
}
