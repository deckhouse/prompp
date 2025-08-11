package head

import (
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"slices"
	"sync"
	"unsafe"

	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
)

const (
	UnloadedDataStorageVersion  = 1
	QueriedSeriesStorageVersion = 1
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

type WriteTruncateCloser interface {
	io.WriteCloser
	io.ReadSeeker
	Truncate(size int64) error
}

type QueriedSeriesStorage struct {
	storages [2]WriteTruncateCloser
}

func NewQueriedSeriesStorage(storage1, storage2 WriteTruncateCloser) *QueriedSeriesStorage {
	return &QueriedSeriesStorage{
		storages: [2]WriteTruncateCloser{storage1, storage2},
	}
}

type queriedSeriesStorageHeader struct {
	timestamp int64
	crc32     uint32
	size      uint32
}

func (h *queriedSeriesStorageHeader) toSlice() []byte {
	return (*(*[unsafe.Sizeof(queriedSeriesStorageHeader{})]byte)(unsafe.Pointer(h)))[:]
}

func (h *queriedSeriesStorageHeader) CalculateCrc32(queriedSeriesBitset []byte) uint32 {
	h.crc32 = 0

	writer := crc32.NewIEEE()
	_, _ = writer.Write(h.toSlice())
	_, _ = writer.Write(queriedSeriesBitset)
	h.crc32 = writer.Sum32()

	return h.crc32
}

func (s *QueriedSeriesStorage) Write(queriedSeriesBitset []byte, timestamp int64) error {
	storage := s.storages[0]

	var headerBuffer [1 + unsafe.Sizeof(queriedSeriesStorageHeader{})]byte
	headerBuffer[0] = UnloadedDataStorageVersion

	header := (*queriedSeriesStorageHeader)(unsafe.Pointer(&headerBuffer[1]))
	header.timestamp = timestamp
	header.size = uint32(len(queriedSeriesBitset))
	header.CalculateCrc32(queriedSeriesBitset)

	if _, err := storage.Seek(0, io.SeekStart); err != nil {
		return err
	}

	if _, err := storage.Write(headerBuffer[:]); err != nil {
		return err
	}

	if _, err := storage.Write(queriedSeriesBitset); err != nil {
		return err
	}

	if err := storage.Truncate(int64(len(headerBuffer) + len(queriedSeriesBitset))); err != nil {
		return err
	}

	s.storages[0], s.storages[1] = s.storages[1], s.storages[0]
	return nil
}

func (s *QueriedSeriesStorage) Read() (data []byte, err error) {
	storages := s.readStorageHeaders()

	for i := range storages {
		if growTo := int(storages[i].size) - len(data); growTo > 0 {
			data = slices.Grow(data, growTo)
		}
		data = data[:storages[i].size]

		if len(data) > 0 {
			if _, err = io.ReadFull(storages[i].reader, data); err != nil {
				logger.Warnf("failed to read data from queried series storage: %v", err)
				continue
			}
		}

		if storageCrc32 := storages[i].crc32; storageCrc32 != storages[i].CalculateCrc32(data) {
			logger.Warnf("invalid queried series storage crc32: %d != %d", storageCrc32, storages[i].crc32)
			continue
		}

		return data, nil
	}

	return nil, errors.New("no valid queried series storage")
}

func (s *QueriedSeriesStorage) readStorageHeaders() (result []storageHeader) {
	for _, storage := range []storageHeader{{reader: s.storages[0]}, {reader: s.storages[1]}} {
		if err := storage.read(); err == nil {
			result = append(result, storage)
		} else {
			logger.Warnf("failed to read header: %v", err)
		}
	}

	if len(result) == 2 && result[0].timestamp < result[1].timestamp {
		result[0], result[1] = result[1], result[0]
	}

	return result
}

func (s *QueriedSeriesStorage) Close() error {
	return errors.Join(s.storages[0].Close(), s.storages[1].Close())
}

type storageHeader struct {
	queriedSeriesStorageHeader
	reader io.ReadSeeker
}

func (s *storageHeader) read() error {
	if err := s.readAndValidateFormatVersion(); err != nil {
		return err
	}

	_, err := io.ReadFull(s.reader, s.toSlice())
	return err
}

func (s *storageHeader) readAndValidateFormatVersion() error {
	if _, err := s.reader.Seek(0, io.SeekStart); err != nil {
		return err
	}

	version := []byte{0}
	if _, err := s.reader.Read(version); err != nil {
		return err
	}

	if version[0] != QueriedSeriesStorageVersion {
		return fmt.Errorf("QueriedSeriesStorage invalid version %d", version[0])
	}

	return nil
}
