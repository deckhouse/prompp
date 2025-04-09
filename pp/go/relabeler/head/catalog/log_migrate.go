package catalog

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
)

var (
	ErrUnsupportedVersion = errors.New("unsupported version")
	ErrUnreadableLogFile  = errors.New("unreadable log file")
)

const (
	headerSize = 8
)

func migrate(targetFilePath, sourceFilePath string, targetVersion uint64) (_ *FileHandler, _ Encoder, _ Decoder, _ error) {
	sourceFile, sourceVersion, sourceEncoder, sourceDecoder, err := loadFile(sourceFilePath)
	if err != nil {
		return nil, nil, nil, err
	}

	if sourceVersion == targetVersion {
		if sourceFilePath == targetFilePath {
			return sourceFile, sourceEncoder, sourceDecoder, nil
		}

		err = os.Rename(sourceFilePath, targetFilePath)
		if err != nil {
			return nil, nil, nil, errors.Join(err, sourceFile.Close())
		}

		return sourceFile, sourceEncoder, sourceDecoder, nil
	}

	targetEncoder, targetDecoder, err := codecByVersion(targetVersion)
	if err != nil {
		return nil, nil, nil, err
	}

	migration := getMigration(sourceVersion, targetVersion)

	records := make([]*Record, 0, 10)
	for {
		record := Record{}
		if err = sourceDecoder.Decode(sourceFile, &record); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			logger.Errorf("failed to decode record: %v", err)
			break
		}
		records = append(records, &record)
	}

	migratedRecords := make([]*Record, 0, len(records))
	for _, record := range records {
		migratedRecords = append(migratedRecords, migration.Migrate(record))
	}

	swapFilePath := fmt.Sprintf("%s.swap", sourceFilePath)
	targetFile, err := writeSwapAndSwitchAtFilePath(targetFilePath, swapFilePath, targetVersion, targetEncoder, migratedRecords...)
	if err != nil {
		return nil, nil, nil, err
	}

	if err = sourceFile.Close(); err != nil {
		logger.Errorf("failed to close file: %v", err)
	}

	return targetFile, targetEncoder, targetDecoder, nil
}

func loadFile(filePath string) (_ *FileHandler, _ uint64, _ Encoder, _ Decoder, err error) {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			err = ErrUnreadableLogFile
		}
		return nil, 0, nil, nil, err
	}

	if fileInfo.Size() == 0 {
		return nil, 0, nil, nil, ErrUnreadableLogFile
	}

	fh, err := NewFileHandlerWithOpts(filePath, os.O_RDWR, logFilePerm)
	if err != nil {
		return nil, 0, nil, nil, err
	}

	version, err := ReadLogFileVersion(fh)
	if err != nil {
		return nil, 0, nil, nil, errors.Join(fmt.Errorf("read log file version: %w", err), fh.Close())
	}

	e, d, err := codecByVersion(version)
	if err != nil {
		return nil, 0, nil, nil, errors.Join(ErrUnreadableLogFile, fh.Close())
	}

	return fh, version, e, d, nil
}

func newFileHandlerByVersion(filePath string, version uint64) (fh *FileHandler, e Encoder, d Decoder, err error) {
	e, d, err = codecByVersion(version)
	if err != nil {
		return nil, nil, nil, err
	}

	fh, err = newFileHandlerWithOpts(filePath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, logFilePerm)
	if err != nil {
		return nil, nil, nil, err
	}

	offset, err := WriteLogFileVersion(fh, version)
	if err != nil {
		return nil, nil, nil, errors.Join(err, fh.Close())
	}

	fh.SetReadOffset(int64(offset))

	return fh, e, d, nil
}

func codecByVersion(version uint64) (e Encoder, d Decoder, err error) {
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

type Migration interface {
	Migrate(record *Record) *Record
}

type MigrationFunc func(record *Record) *Record

func (fn MigrationFunc) Migrate(record *Record) *Record {
	return fn(record)
}

type MigrationV2 struct{}

func (MigrationV2) Up(record *Record) *Record {
	if record.status == StatusCorrupted {
		record.corrupted = true
		record.status = StatusRotated
	}
	return record
}

func (MigrationV2) Down(record *Record) *Record {
	if record.status == StatusRotated && record.corrupted {
		record.status = StatusCorrupted
	}
	return record
}

type MigrationV3 struct{}

func (MigrationV3) Up(record *Record) *Record {
	record.numberOfSegments = 0
	if !record.lastAppendedSegmentID.IsNil() {
		record.numberOfSegments = record.lastAppendedSegmentID.Value() + 1
	}
	return record
}

func (MigrationV3) Down(record *Record) *Record {
	if record.numberOfSegments > 0 {
		record.lastAppendedSegmentID.Set(record.numberOfSegments - 1)
	}
	return record
}

type ChainedMigration struct {
	migrations []Migration
}

func NewChainedMigration(migrations ...Migration) *ChainedMigration {
	return &ChainedMigration{migrations: migrations}
}

func (c *ChainedMigration) Migrate(record *Record) *Record {
	for _, migration := range c.migrations {
		record = migration.Migrate(record)
	}
	return record
}

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
