package catalog

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
)

const (
	defaultBufferSize = 4096

	LogFileVersionV1 uint64 = 1
	LogFileVersionV2 uint64 = 2
	LogFileVersionV3 uint64 = 3

	logFilePerm = 0o600
)

type Encoder interface {
	Encode(writer io.Writer, r *Record) error
}

type Decoder interface {
	Decode(reader io.Reader, r *Record) error
}

type FileLog struct {
	version  uint64
	file     *FileHandler
	filePath string
	encoder  Encoder
	decoder  Decoder
}

func NewFileLogV1(fileName string) (fl *FileLog, err error) {
	file, err := NewFileHandler(fileName)
	if err != nil {
		return nil, err
	}

	fl = &FileLog{
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

func NewFileLogV2(filePath string) (fl *FileLog, err error) {
	return NewFileLog(filePath, LogFileVersionV2)
}

func NewFileLogV3(filePath string) (fl *FileLog, err error) {
	return NewFileLog(filePath, LogFileVersionV3)
}

func NewFileLog(filePath string, targetVersion uint64) (fl *FileLog, err error) {
	sourceFilePath := filePath
	fl, err = openFileLog(filePath, sourceFilePath, targetVersion)
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

	return newFileLog(filePath, targetVersion)
}

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

func newFileLog(filePath string, version uint64) (*FileLog, error) {
	file, encoder, decoder, err := newFileHandlerByVersion(filePath, version)
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

func (fl *FileLog) Write(r *Record) error {
	return fl.encoder.Encode(fl.file, r)
}

func (fl *FileLog) ReWrite(records ...*Record) (err error) {
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

func writeSwapAndSwitchAtFilePath(targetFilePath, swapFilePath string, version uint64, encoder Encoder, records ...*Record) (*FileHandler, error) {
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

	offset, err := WriteLogFileVersion(swapFile, version)
	if err != nil {
		return nil, fmt.Errorf("write log file version: %w", err)
	}

	for _, record := range records {
		if err = encoder.Encode(swapFile, record); err != nil {
			return nil, fmt.Errorf("encode record: %w", err)
		}
	}

	if err = swapFile.Sync(); err != nil {
		return nil, fmt.Errorf("sync swap file: %w", err)
	}

	swapFile.SetReadOffset(int64(offset))

	return swapFile, nil
}

func (fl *FileLog) Read(r *Record) error {
	return fl.decoder.Decode(fl.file, r)
}

func (fl *FileLog) Size() int {
	return fl.file.Size()
}

func (fl *FileLog) Close() error {
	return fl.file.Close()
}

type FileHandler struct {
	file        *os.File
	size        int
	readOffset  int64
	writeOffset int64
}

func NewFileHandler(filePath string) (*FileHandler, error) {
	return newFileHandlerWithOpts(filePath, os.O_CREATE|os.O_RDWR, logFilePerm)
}

func NewFileHandlerWithOpts(filePath string, flag int, perm os.FileMode) (*FileHandler, error) {
	return newFileHandlerWithOpts(filePath, flag, perm)
}

func newFileHandlerWithOpts(filePath string, flag int, perm os.FileMode) (*FileHandler, error) {
	file, err := os.OpenFile(filePath, flag, perm)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, file.Close())
		}
	}()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("read file info: %w", err)
	}

	return &FileHandler{
		file:        file,
		size:        int(fileInfo.Size()),
		writeOffset: fileInfo.Size(),
	}, nil
}

func (fh *FileHandler) Write(p []byte) (n int, err error) {
	n, err = fh.file.WriteAt(p, fh.writeOffset)
	if err != nil {
		return 0, fmt.Errorf("write file: %w", err)
	}

	if err = fh.file.Sync(); err != nil {
		return 0, fmt.Errorf("sync file: %w", err)
	}

	fh.size += n
	fh.writeOffset += int64(n)
	return n, nil
}

func (fh *FileHandler) Read(p []byte) (n int, err error) {
	n, err = fh.file.ReadAt(p, fh.readOffset)
	if err != nil {
		if !errors.Is(err, io.EOF) {
			return 0, fmt.Errorf("read file: %w", err)
		}
	}
	fh.readOffset += int64(n)
	return n, err
}

func (fh *FileHandler) SetReadOffset(offset int64) {
	fh.readOffset = offset
}

func (fh *FileHandler) Sync() error {
	return fh.file.Sync()
}

func (fh *FileHandler) FileName() string {
	return fh.file.Name()
}

func (fh *FileHandler) Size() int {
	return fh.size
}

func (fh *FileHandler) Close() error {
	return fh.file.Close()
}
