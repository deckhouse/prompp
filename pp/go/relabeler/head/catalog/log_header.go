package catalog

import (
	"encoding/binary"
	"io"
)

// ReadLogFileVersion reads log file version respecting on disk version size.
func ReadLogFileVersion(reader io.Reader) (version uint64, err error) {
	var v [8]byte
	_, err = reader.Read(v[:1])
	if err != nil {
		return 0, err
	}

	version = binary.LittleEndian.Uint64(v[:8])
	if version <= LogFileVersionV2 {
		// skip next 7 bytes
		_, err = reader.Read(v[1:8])
		return version, err
	}

	return version, nil
}

// WriteLogFileVersion writes log file version respecting on disk version size.
func WriteLogFileVersion(writer io.Writer, version uint64) (int, error) {
	var v [8]byte
	binary.LittleEndian.PutUint64(v[:8], version)
	numberOfBytesToWrite := len(v)
	if version >= LogFileVersionV3 {
		numberOfBytesToWrite = 1
	}
	bytesWritten, err := writer.Write(v[:numberOfBytesToWrite])
	return bytesWritten, err
}
