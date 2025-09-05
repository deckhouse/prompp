package head

import (
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"unsafe"

	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
)

const (
	UnloadedDataStorageVersion  = 1
	QueriedSeriesStorageVersion = 1
)

type StorageFile interface {
	Open(flags int) error
	io.WriteCloser
	io.ReadSeeker
	io.ReaderAt
	Sync() error
	Truncate(size int64) error
	IsEmpty() bool
}

type UnloadedDataStorage struct {
	storage         StorageFile
	snapshots       []relabeler.UnloadedDataSnapshotHeader
	maxSnapshotSize uint32
}

func NewUnloadedDataStorage(storage StorageFile) *UnloadedDataStorage {
	return &UnloadedDataStorage{
		storage: storage,
	}
}

func (s *UnloadedDataStorage) WriteSnapshot(snapshot []byte) (relabeler.UnloadedDataSnapshotHeader, error) {
	if len(snapshot) == 0 {
		return relabeler.UnloadedDataSnapshotHeader{}, nil
	}

	if err := s.storage.Open(os.O_RDWR | os.O_CREATE | os.O_TRUNC); err != nil {
		return relabeler.UnloadedDataSnapshotHeader{}, err
	}

	if len(s.snapshots) == 0 {
		if err := s.WriteFormatVersion(); err != nil {
			return relabeler.UnloadedDataSnapshotHeader{}, err
		}
	}

	_, err := s.storage.Write(snapshot)
	if err == nil {
		err = s.storage.Sync()
	}
	return relabeler.NewUnloadedDataSnapshotHeader(snapshot), err
}

func (s *UnloadedDataStorage) WriteIndex(header relabeler.UnloadedDataSnapshotHeader) {
	s.snapshots = append(s.snapshots, header)
	s.maxSnapshotSize = max(header.SnapshotSize, s.maxSnapshotSize)
}

func (s *UnloadedDataStorage) WriteFormatVersion() error {
	_, err := s.storage.Write([]byte{UnloadedDataStorageVersion})
	return err
}

func (s *UnloadedDataStorage) ForEachSnapshot(f func(snapshot []byte, isLast bool)) error {
	if len(s.snapshots) == 0 {
		return nil
	}

	offset, err := s.validateFormatVersion()
	if err != nil {
		return err
	}

	snapshot := make([]byte, 0, s.maxSnapshotSize)
	for index, header := range s.snapshots {
		snapshot = snapshot[:header.SnapshotSize]
		size, err := s.storage.ReadAt(snapshot, offset)
		if uint32(size) != header.SnapshotSize {
			return err
		}
		offset += int64(size)

		if !header.IsValid(snapshot) {
			return fmt.Errorf("invalid snapshot at index %d", index)
		}

		f(snapshot, index == len(s.snapshots)-1)
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

func (s *UnloadedDataStorage) Close() (err error) {
	if s.storage != nil {
		err = s.storage.Close()
		s.storage = nil
	}

	return
}

type QueriedSeriesStorage struct {
	storages     [2]StorageFile
	validStorage StorageFile
}

func NewQueriedSeriesStorage(storage1, storage2 StorageFile) *QueriedSeriesStorage {
	return &QueriedSeriesStorage{
		storages: [2]StorageFile{storage1, storage2},
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
	if err := storage.Open(os.O_RDWR | os.O_CREATE | os.O_TRUNC); err != nil {
		s.changeActiveStorageIfNoValidStorage()
		return err
	}

	var headerBuffer [1 + unsafe.Sizeof(queriedSeriesStorageHeader{})]byte
	headerBuffer[0] = UnloadedDataStorageVersion

	header := (*queriedSeriesStorageHeader)(unsafe.Pointer(&headerBuffer[1]))
	header.timestamp = timestamp
	header.size = uint32(len(queriedSeriesBitset))
	header.CalculateCrc32(queriedSeriesBitset)

	if err := s.writeToStorage(storage, headerBuffer[:], queriedSeriesBitset); err != nil {
		s.changeActiveStorageIfNoValidStorage()
		return err
	}

	s.validStorage = s.storages[0]
	s.changeActiveStorage()
	return nil
}

func (s *QueriedSeriesStorage) writeToStorage(storage StorageFile, headerBuffer, queriedSeriesBitset []byte) error {
	if _, err := storage.Seek(0, io.SeekStart); err != nil {
		return err
	}

	if _, err := storage.Write(headerBuffer); err != nil {
		return err
	}

	if _, err := storage.Write(queriedSeriesBitset); err != nil {
		return err
	}

	if err := storage.Sync(); err != nil {
		return err
	}

	return storage.Truncate(int64(len(headerBuffer) + len(queriedSeriesBitset)))
}

func (s *QueriedSeriesStorage) changeActiveStorage() {
	s.storages[0], s.storages[1] = s.storages[1], s.storages[0]
}

func (s *QueriedSeriesStorage) changeActiveStorageIfNoValidStorage() {
	if s.validStorage == nil {
		s.changeActiveStorage()
	}
}

func (s *QueriedSeriesStorage) Read() (data []byte, err error) {
	readers, maxSize := s.readStorageHeaders()
	data = make([]byte, 0, maxSize)

	for i := range readers {
		data = data[:readers[i].size]

		if len(data) > 0 {
			if _, err = io.ReadFull(readers[i].storage, data); err != nil {
				logger.Warnf("failed to read data from queried series storage: %v", err)
				continue
			}
		}

		if storageCrc32 := readers[i].crc32; storageCrc32 != readers[i].CalculateCrc32(data) {
			logger.Warnf("invalid queried series storage crc32: %d != %d", storageCrc32, readers[i].crc32)
			continue
		}

		s.validStorage = readers[i].storage
		if readers[i].storage == s.storages[0] {
			s.changeActiveStorage()
		}

		return data, nil
	}

	return nil, errors.New("no valid queried series storage")
}

func (s *QueriedSeriesStorage) readStorageHeaders() (result []storageHeaderReader, maxSize uint32) {
	for _, storage := range s.storages {
		reader := storageHeaderReader{storage: storage}

		if err := reader.read(); err == nil {
			result = append(result, reader)
			maxSize = max(maxSize, reader.size)
		} else {
			if !os.IsNotExist(err) && !errors.Is(err, io.EOF) {
				logger.Warnf("failed to read header: %v", err)
			}
		}
	}

	if len(result) == 2 && result[0].timestamp < result[1].timestamp {
		result[0], result[1] = result[1], result[0]
	}

	return
}

func (s *QueriedSeriesStorage) Close() error {
	return errors.Join(s.storages[0].Close(), s.storages[1].Close())
}

type storageHeaderReader struct {
	queriedSeriesStorageHeader
	storage StorageFile
}

func (s *storageHeaderReader) read() error {
	if err := s.storage.Open(os.O_RDWR); err != nil {
		return err
	}

	if err := s.readAndValidateFormatVersion(); err != nil {
		return err
	}

	_, err := io.ReadFull(s.storage, s.toSlice())
	return err
}

func (s *storageHeaderReader) readAndValidateFormatVersion() error {
	if _, err := s.storage.Seek(0, io.SeekStart); err != nil {
		return err
	}

	version := []byte{0}
	if _, err := s.storage.Read(version); err != nil {
		return err
	}

	if version[0] != QueriedSeriesStorageVersion {
		return fmt.Errorf("QueriedSeriesStorage invalid version %d", version[0])
	}

	return nil
}
