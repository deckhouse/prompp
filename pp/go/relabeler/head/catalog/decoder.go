package catalog

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/prometheus/prometheus/pp/go/util/optional"
	"hash"
	"hash/crc32"
	"io"
)

// DecoderV1 decoder.
type DecoderV1 struct {
}

// Decode is an Decoder interface implementation.
func (DecoderV1) Decode(reader io.Reader, r *Record) (err error) {
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

// DecoderV2 decoder.
type DecoderV2 struct{}

type readerWithCounter struct {
	reader io.Reader
	n      int
}

func (r *readerWithCounter) Read(p []byte) (n int, err error) {
	n, err = r.reader.Read(p)
	r.n += n
	return n, err
}

func (r *readerWithCounter) BytesRead() int {
	return r.n
}

func newReaderWithCounter(reader io.Reader) *readerWithCounter {
	return &readerWithCounter{reader: reader}
}

// Decode is an Decoder interface implementation.
func (DecoderV2) Decode(reader io.Reader, r *Record) (err error) {
	var size uint8
	if err = binary.Read(reader, binary.LittleEndian, &size); err != nil {
		return fmt.Errorf("read record size: %w", err)
	}

	rReader := newReaderWithCounter(reader)

	defer func() {
		if err != nil && errors.Is(err, io.EOF) || size != uint8(rReader.BytesRead()) {
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

func decodeOptionalValue[T any](reader io.Reader, byteOrder binary.ByteOrder, valueRef *optional.Optional[T]) (err error) {
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

// DecoderV3 is the third decoder generation.
type DecoderV3 struct {
	offset int
	size   uint8
	buffer [RecordStructMaxSizeV3 - 1]byte
	hasher hash.Hash32
}

// NewDecoderV3 is DecoderV3 constructor.
func NewDecoderV3() *DecoderV3 {
	return &DecoderV3{
		hasher: crc32.NewIEEE(),
	}
}

// Decode is an Decoder interface implementation.
func (d *DecoderV3) Decode(reader io.Reader, r *Record) (err error) {
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

	targetOffset := d.offset + 16
	r.id = uuid.UUID(d.buffer[d.offset:targetOffset])
	d.offset = targetOffset

	r.numberOfShards = binary.LittleEndian.Uint16(d.buffer[d.offset:])
	d.offset += 2

	r.createdAt = int64(binary.LittleEndian.Uint64(d.buffer[d.offset:]))
	d.offset += 8

	r.updatedAt = int64(binary.LittleEndian.Uint64(d.buffer[d.offset:]))
	d.offset += 8

	r.deletedAt = int64(binary.LittleEndian.Uint64(d.buffer[d.offset:]))
	d.offset += 8

	r.corrupted = d.buffer[d.offset] > 0
	d.offset += 1

	r.status = Status(d.buffer[d.offset])
	d.offset += 1

	r.numberOfSegments = binary.LittleEndian.Uint32(d.buffer[d.offset:])
	d.offset += 4

	r.mint = int64(binary.LittleEndian.Uint64(d.buffer[d.offset:]))
	d.offset += 8

	r.maxt = int64(binary.LittleEndian.Uint64(d.buffer[d.offset:]))
	d.offset += 8

	return nil
}

func (d *DecoderV3) reset() {
	d.offset = 0
	d.size = 0
	d.hasher.Reset()
}

func (d *DecoderV3) readSize(reader io.Reader) error {
	var sizeBuff [1]byte
	if _, err := reader.Read(sizeBuff[:]); err != nil {
		return fmt.Errorf("read record size: %w", err)
	}
	d.size = sizeBuff[0]

	if int(d.size) > len(d.buffer) {
		return fmt.Errorf("invalid size: %d", d.size)
	}

	return nil
}

func (d *DecoderV3) readRecord(reader io.Reader) error {
	if _, err := reader.Read(d.buffer[:d.size]); err != nil {
		return fmt.Errorf("read whole record: %w", err)
	}
	return nil
}

func (d *DecoderV3) validateCRC32() (err error) {
	var expectedCRC32Hash uint32
	d.readUint32(&expectedCRC32Hash)

	if _, err = d.hasher.Write(d.buffer[4:d.size]); err != nil {
		return fmt.Errorf("write to crc32 hasher: %w", err)
	}

	actualCRC32Hash := d.hasher.Sum32()
	if expectedCRC32Hash != actualCRC32Hash {
		return fmt.Errorf("invalid crc32: expected: %d, actual: %d", expectedCRC32Hash, actualCRC32Hash)
	}

	return nil
}

func (d *DecoderV3) readUint32(value *uint32) {
	*value = binary.LittleEndian.Uint32(d.buffer[d.offset:])
	d.offset += 4
}
