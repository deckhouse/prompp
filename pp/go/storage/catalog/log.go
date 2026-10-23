package catalog

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/prometheus/prometheus/pp/go/logger"
)

const (
	// LogFileVersionV1 version 1 of log-file.
	LogFileVersionV1 uint64 = 1
	// LogFileVersionV2 version 2 of log-file.
	LogFileVersionV2 uint64 = 2
	// LogFileVersionV3 version 3 of log-file.
	LogFileVersionV3 uint64 = 3

	// logFilePerm log-file permissions.
	logFilePerm = 0o600
)

//
// FileLog
//

// FileLog head-log file, contains [SerializedRecord]s of heads.
type FileLog struct {
	version  uint64
	file     *FileHandler
	filePath string
	encoder  Encoder
	decoder  Decoder
}

// NewFileLogV1 init new [FileLog] with [EncoderV1], [DecoderV1], version 1.
//
//	Deprecated.
func NewFileLogV1(fileName string) (*FileLog, error) {
	file, err := NewFileHandler(fileName)
	if err != nil {
		return nil, err
	}

	fl := &FileLog{
		version: LogFileVersionV1,
		file:    file,
		encoder: EncoderV1{},
		decoder: DecoderV1{},
	}

	defer func() {
		if err != nil {
			_ = fl.Close()
		}
	}()

	if file.Size() == 0 {
		if err = binary.Write(file, binary.LittleEndian, fl.version); err != nil {
			return nil, errors.Join(fmt.Errorf("failed to write log file version: %w", err), fl.Close())
		}
	} else {
		var version uint64
		if err = binary.Read(file, binary.LittleEndian, &version); err != nil {
			return nil, errors.Join(fmt.Errorf("failed to read log file version: %w", err), fl.Close())
		}
		if version != fl.version {
			return nil, errors.Join(fmt.Errorf("invalid log file version: %d", version), fl.Close())
		}
	}

	return fl, nil
}

// NewFileLogV2 init new [FileLog] with [EncoderV2], [DecoderV2], version 2.
func NewFileLogV2(filePath string) (*FileLog, error) {
	return NewFileLog(filePath, LogFileVersionV2)
}

// NewFileLogV3 init new [FileLog] with [EncoderV3], [DecoderV3], version 3.
func NewFileLogV3(filePath string) (*FileLog, error) {
	return NewFileLog(filePath, LogFileVersionV3)
}

// NewFileLog init new [FileLog] with migrate to target version encoder and decoder.
func NewFileLog(filePath string, targetVersion uint64) (*FileLog, error) {
	sourceFilePath := filePath
	fl, err := openFileLog(filePath, sourceFilePath, targetVersion)
	if err == nil {
		return fl, nil
	}

	if !errors.Is(err, ErrUnreadableLogFile) {
		return nil, err
	}

	logger.Errorf("unreadable log file: filepath: %s, error: %v", sourceFilePath, err)

	sourceFilePath = fmt.Sprintf("%s.compacted", filePath)
	fl, err = openFileLog(filePath, sourceFilePath, targetVersion)
	if err == nil {
		return fl, nil
	}

	if !errors.Is(err, ErrUnreadableLogFile) {
		return nil, err
	}

	logger.Errorf("unreadable log file: filepath: %s, error: %v", sourceFilePath, err)

	return newFileLogByVersion(filePath, targetVersion)
}

// openFileLog open [FileLog] with migrate to version.
func openFileLog(filePath, sourceFilePath string, version uint64) (*FileLog, error) {
	file, encoder, decoder, err := migrate(filePath, sourceFilePath, version)
	if err != nil {
		return nil, err
	}

	return &FileLog{
		version:  version,
		file:     file,
		filePath: filePath,
		encoder:  encoder,
		decoder:  decoder,
	}, nil
}

// newFileLogByVersion init new [FileLog] by version.
func newFileLogByVersion(filePath string, version uint64) (*FileLog, error) {
	encoder, decoder, err := codecsByVersion(version)
	if err != nil {
		return nil, fmt.Errorf("create encoder/decoder: %w", err)
	}

	file, err := createFileHandlerByVersion(filePath, version)
	if err != nil {
		return nil, fmt.Errorf("create file handler: %w", err)
	}

	return &FileLog{
		version:  version,
		file:     file,
		filePath: filePath,
		encoder:  encoder,
		decoder:  decoder,
	}, nil
}

// Close closes the [FileHandler], rendering it unusable for I/O.
func (fl *FileLog) Close() error {
	if fl.isFixedV3() {
		return errors.Join(updateV3ActiveUsed(fl.file), fl.file.Close())
	}

	return fl.file.Close()
}

// ReWrite rewrite [FileLog] with [SerializedRecord]s.
func (fl *FileLog) ReWrite(srecords ...*SerializedRecord) error {
	oldFile := fl.file
	swapFilePath := fmt.Sprintf("%s.compacted", strings.TrimSuffix(fl.filePath, ".compacted"))
	newFile, err := writeSwapAndSwitchAtFilePath(fl.filePath, swapFilePath, fl.version, fl.encoder, srecords...)
	if err != nil {
		return fmt.Errorf("write log file: %w", err)
	}

	fl.file = newFile
	if err = oldFile.Close(); err != nil {
		logger.Errorf("failed to close old file: %v", err)
	}

	return nil
}

// Read [SerializedRecord] from [FileLog].
func (fl *FileLog) Read(sr *SerializedRecord) error {
	return fl.decoder.DecodeFrom(fl.file, sr)
}

// Size return current size of [FileHandler].
func (fl *FileLog) Size() int {
	return fl.file.Size()
}

// Write [SerializedRecord] to [FileLog].
func (fl *FileLog) Write(sr *SerializedRecord) error {
	if fl.isFixedV3() {
		return fl.appendV3(sr)
	}

	return fl.encoder.EncodeTo(fl.file, sr)
}

// appendV3 appends the frame of the record version 3 with the journal terminator in a single
// fixed-size write: the frame overwrites the terminator at the write offset, the new terminator
// is written in the same call, and the write offset stops at the new terminator, so the next
// append overwrites it. The file size does not change.
func (fl *FileLog) appendV3(sr *SerializedRecord) error {
	encoder, ok := fl.encoder.(*EncoderV3)
	if !ok {
		return fmt.Errorf("v3: unexpected encoder %T", fl.encoder)
	}

	frame, err := appendFrameV3(encoder.buffer[:0], sr)
	if err != nil {
		return fmt.Errorf("v3: encode record: %w", err)
	}

	frameLen := len(frame)
	writeBuf := append(frame, recordVersionEndV3)

	if fl.file.appendErr != nil {
		return fl.file.appendErr
	}

	offset := fl.file.writeOffset
	if offset < fl.file.journalStart || offset >= fl.file.journalEnd ||
		int64(len(writeBuf)) > fl.file.journalEnd-offset {
		return ErrJournalFull
	}

	if err = fl.file.WriteFixed(writeBuf); err != nil {
		return fmt.Errorf("v3: write record: %w", err)
	}

	encoder.buffer = writeBuf[:frameLen]
	fl.file.writeOffset += int64(frameLen)

	return nil
}

// isFixedV3 reports whether the log is the fixed-size layout of the version 3: migrated and
// rewritten version 3 files keep the growing legacy layout and the v1/v2 write path.
func (fl *FileLog) isFixedV3() bool {
	return fl.version == LogFileVersionV3 && fl.file.fixedSize
}

// needsCompaction counts used bytes for fixed-size journals instead of their allocated file size.
func (fl *FileLog) needsCompaction(maxSize int) bool {
	if fl.isFixedV3() {
		return fl.file.writeOffset-fl.file.journalStart >= int64(maxSize) ||
			fl.file.journalEnd-fl.file.writeOffset <= int64(maxRecordSizeV3)
	}

	return fl.Size() >= maxSize
}

func writeSwapAndSwitchAtFilePath(
	targetFilePath, swapFilePath string,
	version uint64,
	encoder Encoder,
	srecords ...*SerializedRecord,
) (*FileHandler, error) {
	swapFile, err := createSwapFile(swapFilePath, version, encoder, srecords...)
	if err != nil {
		return nil, fmt.Errorf("create swap file: %w", err)
	}

	defer func() {
		if err != nil {
			err = errors.Join(err, swapFile.Close(), os.RemoveAll(swapFilePath))
		}
	}()

	if err = os.Rename(swapFilePath, targetFilePath); err != nil {
		return nil, fmt.Errorf("rename swap file: %w", err)
	}

	return swapFile, nil
}

// creates swap file, writes records & sets read offset at first record.
func createSwapFile(fileName string, version uint64, encoder Encoder, srs ...*SerializedRecord) (*FileHandler, error) {
	swapFile, err := NewFileHandlerWithOpts(fileName, os.O_CREATE|os.O_RDWR|os.O_TRUNC, logFilePerm)
	if err != nil {
		return nil, fmt.Errorf("new file handler: %w", err)
	}

	defer func() {
		if err != nil {
			err = errors.Join(err, swapFile.Close(), os.RemoveAll(fileName))
		}
	}()

	offset, err := writeLogFileVersion(swapFile, version)
	if err != nil {
		return nil, fmt.Errorf("write log file version: %w", err)
	}

	for _, srecord := range srs {
		if err = encoder.EncodeTo(swapFile, srecord); err != nil {
			return nil, fmt.Errorf("encode record: %w", err)
		}
	}

	if err = swapFile.Sync(); err != nil {
		return nil, fmt.Errorf("sync swap file: %w", err)
	}

	swapFile.SetReadOffset(int64(offset))

	return swapFile, nil
}

// readLogFileVersion reads log file version respecting on disk version size.
func readLogFileVersion(reader io.Reader) (version uint64, err error) {
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

// writeLogFileVersion writes log file version respecting on disk version size.
func writeLogFileVersion(writer io.Writer, version uint64) (int, error) {
	var v [8]byte
	binary.LittleEndian.PutUint64(v[:8], version)
	numberOfBytesToWrite := len(v)
	if version >= LogFileVersionV3 {
		numberOfBytesToWrite = 1
	}
	bytesWritten, err := writer.Write(v[:numberOfBytesToWrite])
	return bytesWritten, err
}
