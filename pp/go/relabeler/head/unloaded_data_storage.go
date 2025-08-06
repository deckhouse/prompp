package head

import (
	"fmt"
	"hash/crc32"
	"io"
	"sync"
)

type ReaderAtWriterCloser interface {
	io.ReaderAt
	io.Writer
	io.Closer
}

type UnloadedDataSnapshotHeader struct {
	crc32        uint32
	snapshotSize uint32
}

func newUnloadedDataSnapshotHeader(snapshot []byte) UnloadedDataSnapshotHeader {
	return UnloadedDataSnapshotHeader{crc32: crc32.ChecksumIEEE(snapshot), snapshotSize: uint32(len(snapshot))}
}

func (h UnloadedDataSnapshotHeader) isValid(snapshot []byte) bool {
	return h.crc32 == crc32.ChecksumIEEE(snapshot)
}

const UnloadedDataStorageVersion = 1

type UnloadedDataStorage struct {
	storage         ReaderAtWriterCloser
	snapshots       []UnloadedDataSnapshotHeader
	lock            sync.RWMutex
	maxSnapshotSize uint32
}

func NewUnloadedDataStorage(storage ReaderAtWriterCloser) *UnloadedDataStorage {
	return &UnloadedDataStorage{storage: storage}
}

func (s *UnloadedDataStorage) Write(snapshot []byte) error {
	if len(snapshot) == 0 {
		return nil
	}

	if len(s.snapshots) == 0 {
		if err := s.writeFormatVersion(); err != nil {
			return err
		}
	}

	header := newUnloadedDataSnapshotHeader(snapshot)
	if _, err := s.storage.Write(snapshot); err != nil {
		return err
	}

	s.lock.Lock()
	s.snapshots = append(s.snapshots, header)
	s.maxSnapshotSize = max(header.snapshotSize, s.maxSnapshotSize)
	s.lock.Unlock()

	return nil
}

func (s *UnloadedDataStorage) writeFormatVersion() error {
	_, err := s.storage.Write([]byte{UnloadedDataStorageVersion})
	return err
}

func (s *UnloadedDataStorage) ForEachSnapshot(f func(snapshot []byte, isLast bool)) error {
	offset, err := s.validateFormatVersion()
	if err != nil {
		if err == io.EOF {
			return nil
		}
		return err
	}

	s.lock.RLock()
	snapshots := s.snapshots
	maxSnapshotSize := s.maxSnapshotSize
	s.lock.RUnlock()

	snapshot := make([]byte, 0, maxSnapshotSize)
	for index := range snapshots {
		header := snapshots[index]

		snapshot = snapshot[:header.snapshotSize]
		size, err := s.storage.ReadAt(snapshot, offset)
		if uint32(size) != header.snapshotSize {
			return err
		}
		offset += int64(size)

		if !header.isValid(snapshot) {
			return fmt.Errorf("invalid snapshot at index %d", index)
		}

		f(snapshot, index == len(snapshots)-1)
	}

	return nil
}

func (s *UnloadedDataStorage) validateFormatVersion() (offset int64, err error) {
	version := []byte{0}
	if _, err = s.storage.ReadAt(version, 0); err != nil {
		return 0, err
	}

	if version[0] != UnloadedDataStorageVersion {
		return 0, fmt.Errorf("UnloadedDataStorage invalid version %d", version[0])
	}

	return int64(len(version)), nil
}

func (s *UnloadedDataStorage) Close() error {
	return s.storage.Close()
}
