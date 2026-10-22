package catalog

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

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

// ErrLogNeedsRewrite prevents mixing source and target formats before [Catalog] compacts the log.
var ErrLogNeedsRewrite = errors.New("log requires rewrite before appending")

//
// FileLog
//

// FileLog reads snapshots in the source file format and writes the target format after compaction.
// [Catalog] merges and repairs [SerializedRecord] deltas before requesting a rewrite.
type FileLog struct {
	version       uint64
	targetVersion uint64
	migration     Migration
	file          *FileHandler
	filePath      string
	encoder       Encoder
	decoder       Decoder
	reader        recordReader
}

// NewFileLog opens the source format for reading and defers migration to [Catalog] compaction.
// A damaged fixed-v3 source may be recovered from a fallback, but is never replaced with an empty log.
func NewFileLog(filePath string, targetVersion uint64) (*FileLog, error) {
	sourceFilePath := filePath
	fl, err := openFileLog(filePath, sourceFilePath, targetVersion, syncDir)
	if err == nil {
		return fl, nil
	}

	if !errors.Is(err, ErrUnreadableLogFile) &&
		!errors.Is(err, ErrInvalidV3Header) && !errors.Is(err, ErrInvalidV3FileSize) {
		return nil, err
	}

	sourceErr := err
	sourceFilePath = fmt.Sprintf("%s.compacted", filePath)
	fl, err = openFileLog(filePath, sourceFilePath, targetVersion, syncDir)
	if err == nil {
		return fl, nil
	}

	if !errors.Is(sourceErr, ErrUnreadableLogFile) || !errors.Is(err, ErrUnreadableLogFile) {
		return nil, errors.Join(sourceErr, err)
	}

	return newFileLogByVersion(filePath, targetVersion)
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
		version:       LogFileVersionV1,
		targetVersion: LogFileVersionV1,
		filePath:      fileName,
		file:          file,
		encoder:       EncoderV1{},
		decoder:       DecoderV1{},
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

// Close closes the [FileHandler], rendering it unusable for I/O.
func (fl *FileLog) Close() error {
	if fl.isFixedV3() {
		return errors.Join(updateV3ActiveUsed(fl.file), fl.file.Close())
	}

	return fl.file.Close()
}

// NeedsRewrite reports whether the source must be compacted before writing the target format.
func (fl *FileLog) NeedsRewrite() bool {
	return fl.version != fl.targetVersion
}

// Read [SerializedRecord] from [FileLog].
func (fl *FileLog) Read(sr *SerializedRecord) error {
	var err error
	if fl.version == LogFileVersionV3 {
		if fl.reader.file != fl.file {
			fl.reader = recordReader{file: fl.file}
		}

		fl.reader.minUsed = fl.file.activeUsed
		err = fl.reader.Read(sr)
	} else {
		err = fl.decoder.DecodeFrom(fl.file, sr)
	}

	if err == nil && fl.migration != nil {
		fl.migration.Migrate(sr)
	}

	return err
}

// ReWrite rewrite [FileLog] with [SerializedRecord]s.
func (fl *FileLog) ReWrite(srecords ...*SerializedRecord) error {
	if fl.version == LogFileVersionV3 && !fl.NeedsRewrite() {
		return fl.rewriteV3(srecords)
	}

	return fl.rewriteV2(srecords, syncDir)
}

// Size return current size of [FileHandler].
func (fl *FileLog) Size() int {
	return fl.file.Size()
}

// Write [SerializedRecord] to [FileLog].
func (fl *FileLog) Write(sr *SerializedRecord) error {
	if fl.NeedsRewrite() {
		return ErrLogNeedsRewrite
	}

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
	frame = append(frame, recordVersionEndV3)

	if fl.file.appendErr != nil {
		return fl.file.appendErr
	}

	offset := fl.file.writeOffset
	if offset < fl.file.journalStart || offset >= fl.file.journalEnd ||
		int64(len(frame)) > fl.file.journalEnd-offset {
		return ErrJournalFull
	}

	if err = fl.file.WriteFixed(frame); err != nil {
		return fmt.Errorf("v3: write record: %w", err)
	}

	encoder.buffer = frame[:frameLen]
	fl.file.writeOffset += int64(frameLen)
	fl.file.activeUsed = fl.file.writeOffset - fl.file.journalStart

	return nil
}

// isFixedV3 reports whether the open file uses the preallocated v3 layout.
func (fl *FileLog) isFixedV3() bool {
	return fl.version == LogFileVersionV3
}

// needsCompaction counts used bytes for fixed-size journals instead of their allocated file size.
func (fl *FileLog) needsCompaction(maxSize int) bool {
	if fl.isFixedV3() {
		return fl.file.writeOffset-fl.file.journalStart >= int64(maxSize) ||
			fl.file.journalEnd-fl.file.writeOffset <= int64(maxRecordSizeV3)
	}

	return fl.Size() >= maxSize
}

// rewriteV2 adopts the replacement after rename and closes both files if syncing the directory fails.
func (fl *FileLog) rewriteV2(srecords []*SerializedRecord, syncDirectory func(string) error) error {
	oldFile := fl.file
	swapFilePath := fl.filePath + ".compacted"
	newFile, err := writeSwapAndSwitchAtFilePath(fl.filePath, swapFilePath, fl.targetVersion, fl.encoder, srecords...)
	if err != nil {
		return fmt.Errorf("write log file: %w", err)
	}

	fl.file = newFile
	fl.version = fl.targetVersion
	_, fl.decoder, _ = codecsByVersion(fl.version)
	fl.migration = getMigration(fl.version, fl.version)
	if err = oldFile.Close(); err != nil {
		logger.Errorf("failed to close old file: %v", err)
	}

	if err = syncDirectory(filepath.Dir(fl.filePath)); err != nil {
		return errors.Join(fmt.Errorf("sync log directory: %w", err), newFile.Close())
	}

	return nil
}

// createSwapFile builds a complete snapshot in the target format, leaving the source untouched on failure.
func createSwapFile(fileName string, version uint64, encoder Encoder, srs ...*SerializedRecord) (*FileHandler, error) {
	swapFile, err := createFileHandlerByVersion(fileName, version)
	if err != nil {
		return nil, err
	}

	fl := &FileLog{version: version, targetVersion: version, file: swapFile, encoder: encoder}
	if version == LogFileVersionV3 {
		err = fl.rewriteV3(srs)
	} else {
		for _, sr := range srs {
			if err = encoder.EncodeTo(swapFile, sr); err != nil {
				break
			}
		}
	}

	if err == nil {
		err = swapFile.Sync()
	}

	if err != nil {
		return nil, errors.Join(err, swapFile.Close(), os.Remove(fileName))
	}

	return swapFile, nil
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
		version:       version,
		targetVersion: version,
		migration:     getMigration(version, version),
		file:          file,
		filePath:      filePath,
		encoder:       encoder,
		decoder:       decoder,
	}, nil
}

// openFileLog loads the source, promoting a fallback file by rename before syncing its directory.
// A directory sync failure closes the file and prevents returning a writable log.
func openFileLog(
	filePath, sourceFilePath string,
	targetVersion uint64,
	syncDirectory func(string) error,
) (*FileLog, error) {
	encoder, _, err := codecsByVersion(targetVersion)
	if err != nil {
		return nil, err
	}

	file, version, err := loadFile(sourceFilePath)
	if err != nil {
		return nil, err
	}

	_, decoder, err := codecsByVersion(version)
	if err != nil {
		return nil, errors.Join(ErrUnreadableLogFile, err, file.Close())
	}

	if sourceFilePath != filePath {
		if err = os.Rename(sourceFilePath, filePath); err != nil {
			return nil, errors.Join(err, file.Close())
		}

		if err = syncDirectory(filepath.Dir(filePath)); err != nil {
			return nil, errors.Join(fmt.Errorf("sync log directory: %w", err), file.Close())
		}
	}

	return &FileLog{
		version:       version,
		targetVersion: targetVersion,
		migration:     getMigration(version, targetVersion),
		file:          file,
		filePath:      filePath,
		encoder:       encoder,
		decoder:       decoder,
	}, nil
}

// readLogFileVersion reads the version byte and consumes the remaining legacy v1/v2 header bytes.
// Legacy headers occupy eight bytes; the v3 file header stores its version in a single byte.
func readLogFileVersion(reader io.Reader) (version uint64, err error) {
	var v [8]byte
	_, err = reader.Read(v[:1])
	if err != nil {
		return 0, err
	}

	version = uint64(v[0])

	if version <= LogFileVersionV2 {
		// Skip the remaining bytes of the legacy uint64 header.
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

// writeSwapAndSwitchAtFilePath renames a synced target file over the source.
// The caller must adopt the returned file and sync the parent directory after a successful rename.
func writeSwapAndSwitchAtFilePath(
	targetFilePath, swapFilePath string,
	version uint64,
	encoder Encoder,
	srecords ...*SerializedRecord,
) (*FileHandler, error) {
	swapFile, err := createSwapFile(swapFilePath, version, encoder, srecords...)
	if err != nil {
		return nil, err
	}

	if err = os.Rename(swapFilePath, targetFilePath); err != nil {
		return nil, errors.Join(err, swapFile.Close())
	}

	return swapFile, nil
}
