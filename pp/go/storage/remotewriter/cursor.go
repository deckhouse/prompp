package remotewriter

import (
	"errors"
	"fmt"
	"os"
	"unsafe"

	"github.com/edsrzf/mmap-go"

	"github.com/prometheus/prometheus/tsdb/fileutil"
)

//
// MMapFile
//

// MMapFile a wrapper over the mmap file.
type MMapFile struct {
	file *os.File
	mmap mmap.MMap
}

// NewMMapFile init new [MMapFile].
func NewMMapFile(fileName string, flag int, perm os.FileMode, targetSize int64) (*MMapFile, error) {
	file, err := os.OpenFile(fileName, flag, perm) // #nosec G304 // it's meant to be that way
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("stat file: %w", err), file.Close())
	}

	if fileInfo.Size() < targetSize {
		if err = fileutil.Preallocate(file, targetSize, true); err != nil {
			return nil, errors.Join(fmt.Errorf("preallocate file: %w", err), file.Close())
		}
		if err = file.Sync(); err != nil {
			return nil, errors.Join(fmt.Errorf("sync file: %w", err), file.Close())
		}
	}

	mapped, err := mmap.Map(file, mmap.RDWR, 0)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("map file: %w", err), file.Close())
	}

	if err = mapped.Lock(); err != nil {
		return nil, errors.Join(fmt.Errorf("lock mapped file: %w", err), mapped.Unmap(), file.Close())
	}

	return &MMapFile{
		file: file,
		mmap: mapped,
	}, nil
}

// Bytes returns mapped into memory data.
func (f *MMapFile) Bytes() []byte {
	return f.mmap
}

// Close closes the [os.File], rendering it unusable for I/O.
// Unmap deletes the memory mapped region, flushes any remaining changes, and sets m to nil.
func (f *MMapFile) Close() error {
	return errors.Join(f.mmap.Unlock(), f.mmap.Unmap(), f.file.Close())
}

// Sync synchronizes the mapping's contents to the file's contents on disk.
func (f *MMapFile) Sync() error {
	return f.mmap.Flush()
}

//
// Cursor
//

// Cursor to the required ID segment.
type Cursor struct {
	targetSegmentID uint32
	configCRC32     uint32
}

// CursorReadWriter reader and writer [Cursor]s from mmaped [MMapFile].
type CursorReadWriter struct {
	cursor       *Cursor
	failedShards []byte
	file         *MMapFile
}

// NewCursorReadWriter init new [CursorReadWriter].
func NewCursorReadWriter(fileName string, numberOfShards uint16) (*CursorReadWriter, error) {
	cursorSize := int64(unsafe.Sizeof(Cursor{}))
	fileSize := cursorSize + int64(numberOfShards)
	//revive:disable-next-line:add-constant file permissions simple readable as octa-number
	file, err := NewMMapFile(fileName, os.O_CREATE|os.O_RDWR, 0o600, fileSize)
	if err != nil {
		return nil, fmt.Errorf("create file: %w", err)
	}

	crw := &CursorReadWriter{
		cursor: (*Cursor)(unsafe.Pointer(&file.Bytes()[0])), // #nosec G103 // cast to Cursor
		failedShards: unsafe.Slice( // #nosec G103 // it's meant to be that way
			unsafe.SliceData(file.Bytes()[cursorSize:]), // #nosec G103 // it's meant to be that way
			numberOfShards,
		),
		file: file,
	}

	return crw, nil
}

// Close closes the [MMapFile].
func (crw *CursorReadWriter) Close() error {
	if crw.file != nil {
		err := crw.file.Close()
		if err == nil {
			crw.file = nil
		}
		return err
	}

	return nil
}

// GetConfigCRC32 returns CRC32 for config.
func (crw *CursorReadWriter) GetConfigCRC32() uint32 {
	return crw.cursor.configCRC32
}

// GetTargetSegmentID returns target segment ID.
func (crw *CursorReadWriter) GetTargetSegmentID() uint32 {
	return crw.cursor.targetSegmentID
}

// SetConfigCRC32 set CRC32 for config.
func (crw *CursorReadWriter) SetConfigCRC32(configCRC32 uint32) error {
	crw.cursor.configCRC32 = configCRC32
	return crw.file.Sync()
}

// SetShardCorrupted adds a flag that is a shard corrupted by shard ID.
func (crw *CursorReadWriter) SetShardCorrupted(shardID uint16) error {
	crw.failedShards[shardID] = 1
	return crw.file.Sync()
}

// SetTargetSegmentID set target segment ID.
func (crw *CursorReadWriter) SetTargetSegmentID(segmentID uint32) error {
	crw.cursor.targetSegmentID = segmentID
	return crw.file.Sync()
}

// ShardIsCorrupted returns a flag that is a shard corrupted by shard ID.
func (crw *CursorReadWriter) ShardIsCorrupted(shardID uint16) bool {
	return crw.failedShards[shardID] > 0
}
