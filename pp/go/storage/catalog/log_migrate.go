package catalog

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/util/optional"
)

var (
	// ErrUnsupportedVersion unsupported version error.
	ErrUnsupportedVersion = errors.New("unsupported version")
	// ErrUnreadableLogFile unreadable log file error.
	ErrUnreadableLogFile = errors.New("unreadable log file")
)

// migrate source file to target file on target version.
//
//revive:disable-next-line:function-result-limit there is no point in packing it into a structure.
func migrate(
	targetFilePath, sourceFilePath string,
	targetVersion uint64,
) (_ *FileHandler, _ Encoder, _ Decoder, _ error) {
	sourceFile, sourceVersion, err := loadFile(sourceFilePath)
	if err != nil {
		return nil, nil, nil, err
	}

	sourceEncoder, sourceDecoder, err := codecsByVersion(sourceVersion)
	if err != nil {
		return nil, nil, nil, errors.Join(ErrUnreadableLogFile, sourceFile.Close())
	}

	if sourceVersion != targetVersion {
		return migrateTo(sourceFile, sourceDecoder, targetFilePath, sourceFilePath, targetVersion, sourceVersion)
	}

	if sourceFilePath == targetFilePath {
		return sourceFile, sourceEncoder, sourceDecoder, nil
	}

	err = os.Rename(sourceFilePath, targetFilePath)
	if err != nil {
		return nil, nil, nil, errors.Join(err, sourceFile.Close())
	}

	return sourceFile, sourceEncoder, sourceDecoder, nil
}

// migrateTo source file to target file on target version.
//
//revive:disable-next-line:function-result-limit there is no point in packing it into a structure.
func migrateTo(
	sourceFile *FileHandler,
	sourceDecoder Decoder,
	targetFilePath, sourceFilePath string,
	targetVersion, sourceVersion uint64,
) (_ *FileHandler, _ Encoder, _ Decoder, _ error) {
	targetEncoder, targetDecoder, err := codecsByVersion(targetVersion)
	if err != nil {
		return nil, nil, nil, errors.Join(err, sourceFile.Close())
	}

	records := make([]*Record, 0, 10) //revive:disable-line:add-constant it's average value of records
	for {
		record := &Record{}
		if err = sourceDecoder.DecodeFrom(sourceFile, record); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			logger.Errorf("failed to decode record: %v", err)
			break
		}
		records = append(records, record)
	}

	migration := getMigration(sourceVersion, targetVersion)
	migratedRecords := make([]*Record, 0, len(records))
	for _, record := range records {
		migratedRecords = append(migratedRecords, migration.Migrate(record))
	}

	swapFilePath := fmt.Sprintf("%s.swap", sourceFilePath)
	targetFile, err := writeSwapAndSwitchAtFilePath(
		targetFilePath,
		swapFilePath,
		targetVersion,
		targetEncoder,
		migratedRecords...,
	)
	if err != nil {
		return nil, nil, nil, errors.Join(err, sourceFile.Close())
	}

	if err = sourceFile.Close(); err != nil {
		logger.Errorf("failed to close file: %v", err)
	}

	return targetFile, targetEncoder, targetDecoder, nil
}

// loadFile load [FileHandler] from file.
func loadFile(filePath string) (_ *FileHandler, _ uint64, err error) {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			err = ErrUnreadableLogFile
		}
		return nil, 0, err
	}

	if fileInfo.Size() == 0 {
		return nil, 0, ErrUnreadableLogFile
	}

	fh, err := NewFileHandlerWithOpts(filePath, os.O_RDWR, logFilePerm)
	if err != nil {
		return nil, 0, err
	}

	version, err := readLogFileVersion(fh)
	if err != nil {
		return nil, 0, errors.Join(fmt.Errorf("read log file version: %w", err), fh.Close())
	}

	return fh, version, nil
}

// createFileHandlerByVersion create [FileHandler] by version.
func createFileHandlerByVersion(filePath string, version uint64) (*FileHandler, error) {
	fh, err := NewFileHandlerWithOpts(filePath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, logFilePerm)
	if err != nil {
		return nil, err
	}

	offset, err := writeLogFileVersion(fh, version)
	if err != nil {
		return nil, errors.Join(err, fh.Close())
	}

	fh.SetReadOffset(int64(offset))

	return fh, nil
}

// codecsByVersion select codec by version.
func codecsByVersion(version uint64) (e Encoder, d Decoder, err error) {
	switch version {
	case LogFileVersionV1:
		return EncoderV1{}, DecoderV1{}, nil
	case LogFileVersionV2:
		return NewEncoderV2(), DecoderV2{}, nil
	case LogFileVersionV3:
		return NewEncoderV3(), NewDecoderV3(), nil
	default:
		return nil, nil, ErrUnsupportedVersion
	}
}

//
// Migration
//

// Migration migrates record interface.
type Migration interface {
	Migrate(record *Record) *Record
}

// MigrationFunc is Migration interface function wrapper.
type MigrationFunc func(record *Record) *Record

// Migrate reacord version.
func (fn MigrationFunc) Migrate(record *Record) *Record {
	return fn(record)
}

//
// MigrationV2
//

// MigrationV2 migrates record from v1 to v2 and vice versa.
type MigrationV2 struct{}

// Up migrates from v1 to v2.
func (MigrationV2) Up(record *Record) *Record {
	if record.status == StatusCorrupted {
		record.corrupted = true
		record.status = StatusRotated
	}
	return record
}

// Down migrates from v2 to v1.
func (MigrationV2) Down(record *Record) *Record {
	if record.status == StatusRotated && record.corrupted {
		record.status = StatusCorrupted
	}
	return record
}

//
// MigrationV3
//

// MigrationV3 migrates record from v2 to v3 and vice versa.
type MigrationV3 struct{}

// Up migrates from v2 to v3.
func (MigrationV3) Up(record *Record) *Record {
	record.numberOfSegments = 0
	if !record.lastAppendedSegmentID.IsNil() {
		record.numberOfSegments = record.lastAppendedSegmentID.Value() + 1
	}
	return record
}

// Down migrates from v3 to v2.
func (MigrationV3) Down(record *Record) *Record {
	if record.numberOfSegments > 0 {
		record.lastAppendedSegmentID.Set(record.numberOfSegments - 1)
	} else {
		record.lastAppendedSegmentID = optional.WithRawValue[uint32](nil)
	}
	return record
}

//
// ChainedMigration
//

// ChainedMigration combines migrations to provide multiple migrations.
type ChainedMigration struct {
	migrations []Migration
}

// NewChainedMigration constructor.
func NewChainedMigration(migrations ...Migration) *ChainedMigration {
	return &ChainedMigration{migrations: migrations}
}

// Migrate is an Migration interface implementation.
func (c *ChainedMigration) Migrate(record *Record) *Record {
	for _, migration := range c.migrations {
		record = migration.Migrate(record)
	}
	return record
}

// getMigration create [Migration] from version to version.
func getMigration(from, to uint64) Migration {
	up := true
	if from > to {
		up = false
		from, to = to, from
	}

	var migrations []Migration
	for i := from + 1; i <= to; i++ {
		migrations = append(migrations, migrationByVersion(i, up))
	}

	return NewChainedMigration(migrations...)
}

// migrationByVersion create [Migration] by version.
//
//revive:disable-next-line:flag-parameter this is a flag, but it's more convenient this way
func migrationByVersion(version uint64, up bool) Migration {
	switch version {
	case LogFileVersionV2:
		if up {
			return MigrationFunc(MigrationV2{}.Up)
		}
		return MigrationFunc(MigrationV2{}.Down)
	case LogFileVersionV3:
		if up {
			return MigrationFunc(MigrationV3{}.Up)
		}
		return MigrationFunc(MigrationV3{}.Down)
	default:
		panic(fmt.Sprintf("invalid version: %d", version))
	}
}
