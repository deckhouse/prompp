package reader

import (
	"encoding/binary"
	"fmt"
	"io"
)

// ReadHeader read header from reader.
//
//revive:disable-next-line:function-result-limit there is no point in packing it into a structure.
func ReadHeader(reader io.Reader) (fileFormatVersion, encoderVersion uint8, n int, err error) {
	br := NewByteReader(reader)
	fileFormatVersionU64, err := binary.ReadUvarint(br)
	if err != nil {
		return 0, 0, n, fmt.Errorf("failed to read file format version: %w", err)
	}
	fileFormatVersion = uint8(fileFormatVersionU64) // #nosec G115 // no overflow
	n = br.readBytes

	encoderVersionU64, err := binary.ReadUvarint(br)
	if err != nil {
		return 0, 0, n, fmt.Errorf("failed to read encoder version: %w", err)
	}
	encoderVersion = uint8(encoderVersionU64) // #nosec G115 // no overflow
	n = br.readBytes

	return fileFormatVersion, encoderVersion, n, nil
}
