package catalog

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"
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
