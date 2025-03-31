package catalog

import (
	"encoding/binary"
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
		return sourceFile, sourceEncoder, sourceDecoder, nil
	}

	targetEncoder, targetDecoder, err := codecByVersion(targetVersion)
	if err != nil {
		return nil, nil, nil, err
	}

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

	// we assume migration from v1 to v2 here
	for _, record := range records {
		if record.status == StatusCorrupted {
			record.corrupted = true
			record.status = StatusRotated
		}
	}

	swapFilePath := fmt.Sprintf("%s.swap", sourceFilePath)
	targetFile, err := writeSwapAndSwitchAtFilePath(targetFilePath, swapFilePath, targetVersion, targetEncoder, records...)
	if err != nil {
		return nil, nil, nil, err
	}

	if err = sourceFile.Close(); err != nil {
		logger.Errorf("failed to close file: %v", err)
	}

	targetFile.SetReadOffset(headerSize)

	return targetFile, targetEncoder, targetDecoder, nil
}

func loadFile(filePath string) (_ *FileHandler, version uint64, _ Encoder, _ Decoder, err error) {
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

	fh, err := NewFileHandlerWithOpts(filePath, os.O_CREATE|os.O_RDWR, logFilePerm)
	if err != nil {
		return nil, 0, nil, nil, err
	}

	if err = binary.Read(fh, binary.LittleEndian, &version); err != nil {
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

	if err = binary.Write(fh, binary.LittleEndian, version); err != nil {
		return nil, nil, nil, errors.Join(err, fh.Close())
	}

	fh.SetReadOffset(headerSize)

	return fh, e, d, nil
}

func codecByVersion(version uint64) (e Encoder, d Decoder, err error) {
	switch version {
	case LogFileVersionV1:
		return EncoderV1{}, DecoderV1{}, nil
	case LogFileVersionV2:
		return NewEncoderV2(), DecoderV2{}, nil
	default:
		return nil, nil, ErrUnsupportedVersion
	}
}
