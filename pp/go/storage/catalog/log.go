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
// Encoder
//

// Encoder encodes [Record].
type Encoder interface {
	// EncodeTo encode [Record] to [io.Writer].
	EncodeTo(writer io.Writer, r *Record) error
}

//
// Decoder
//

// Decoder decodes [Record].
type Decoder interface {
	// DecodeFrom decode [Record] from [io.Reader].
	DecodeFrom(reader io.Reader, r *Record) error
}

//
// FileLog
//

// FileLog head-log file, contains [Record]s of heads.
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
	return fl.file.Close()
}

// ReWrite rewrite [FileLog] with [Record]s.
func (fl *FileLog) ReWrite(records ...*Record) error {
	oldFile := fl.file
	swapFilePath := fmt.Sprintf("%s.compacted", strings.TrimSuffix(fl.filePath, ".compacted"))
	newFile, err := writeSwapAndSwitchAtFilePath(fl.filePath, swapFilePath, fl.version, fl.encoder, records...)
	if err != nil {
		return fmt.Errorf("write log file: %w", err)
	}

	fl.file = newFile
	if err = oldFile.Close(); err != nil {
		logger.Errorf("failed to close old file: %v", err)
	}

	return nil
}

// Read [Record] from [FileLog].
func (fl *FileLog) Read(r *Record) error {
	return fl.decoder.DecodeFrom(fl.file, r)
}

// Size return current size of [FileHandler].
func (fl *FileLog) Size() int {
	return fl.file.Size()
}

// Write [Record] to [FileLog].
func (fl *FileLog) Write(r *Record) error {
	return fl.encoder.EncodeTo(fl.file, r)
}

func writeSwapAndSwitchAtFilePath(
	targetFilePath, swapFilePath string,
	version uint64,
	encoder Encoder,
	records ...*Record,
) (*FileHandler, error) {
	swapFile, err := createSwapFile(swapFilePath, version, encoder, records...)
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
func createSwapFile(fileName string, version uint64, encoder Encoder, records ...*Record) (*FileHandler, error) {
	swapFile, err := NewFileHandlerWithOpts(fileName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, logFilePerm)
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

	for _, record := range records {
		if err = encoder.EncodeTo(swapFile, record); err != nil {
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
