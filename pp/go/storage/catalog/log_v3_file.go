package catalog

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
)

// Layout of the log file version 3 (constant after creation):
//
//	offset 0: header slot 0 (64 bytes), header slot 1 (64 bytes), padding up to 4096
//	offset 4096: journal 0 (journalSizeV3 bytes), journal 1 (journalSizeV3 bytes)
const (
	// fileHeaderSlotSizeV3 size of one header slot of the log file version 3.
	fileHeaderSlotSizeV3 = 64
	// fileHeaderSlotsCount number of header slots of the log file version 3.
	fileHeaderSlotsCount = 2
	// fileHeaderAreaSizeV3 size of the header area with padding of the log file version 3.
	fileHeaderAreaSizeV3 = 4096
	// journalSizeV3 size of one journal of the log file version 3.
	journalSizeV3 = 2 << 20
	// fileSizeV3 size of the log file version 3: 4096 + 2 * journalSizeV3.
	fileSizeV3 = fileHeaderAreaSizeV3 + fileHeaderSlotsCount*journalSizeV3
)

// Offsets of the fields inside a header slot of the log file version 3.
const (
	fileHeaderVersionOffsetV3     = 0
	fileHeaderMagicOffsetV3       = 1
	fileHeaderActiveOffsetV3      = 4
	fileHeaderGenerationOffsetV3  = 5
	fileHeaderJournalSizeOffsetV3 = 13
	fileHeaderActiveUsedOffsetV3  = 17
	fileHeaderChecksumOffsetV3    = 60
)

// fileMagicV3 magic of the header slot of the log file version 3.
const fileMagicV3 = "HLG"

var (
	// ErrInvalidV3FileSize means the fixed-size log file version 3 has an unexpected length.
	ErrInvalidV3FileSize = errors.New("invalid v3 file size")
	// ErrInvalidV3Header the header slots of the log file version 3 are invalid.
	ErrInvalidV3Header = errors.New("invalid v3 file header")
	// ErrJournalFull means the next frame and its terminator do not fit in the active journal.
	ErrJournalFull = errors.New("catalog journal full")
)

// fileHeaderV3 is the decoded header slot of the log file version 3.
type fileHeaderV3 struct {
	active      uint8
	generation  uint64
	journalSize uint32
	activeUsed  uint32
}

// rewriteV3 writes a zero-padded snapshot before committing the alternate header slot.
// Until the header is durable, the previous active journal remains authoritative.
func (fl *FileLog) rewriteV3(records []*SerializedRecord) error {
	header, headerSlot, err := readFileHeaderV3(fl.file.file)
	if err != nil {
		return err
	}

	image, used, err := encodeJournalV3(records)
	if err != nil {
		return err
	}

	next := header.active ^ 1
	if _, err = fl.file.writeAt(image, journalOffsetV3(next)); err != nil {
		return fmt.Errorf("write inactive v3 journal: %w", err)
	}

	if err = fl.file.Sync(); err != nil {
		return fmt.Errorf("sync inactive v3 journal: %w", err)
	}

	header.active = next
	header.activeUsed = uint32(used) // #nosec G115 // bounded by journalSizeV3
	header.generation++
	slot := encodeFileHeaderV3(header)
	if _, err = fl.file.writeAt(slot[:], int64(headerSlot^1)*fileHeaderSlotSizeV3); err != nil {
		fl.file.appendErr = err
		return fmt.Errorf("switch v3 journal: %w", err)
	}

	if err = fl.file.Sync(); err != nil {
		fl.file.appendErr = err
		return fmt.Errorf("sync v3 journal switch: %w", err)
	}

	fl.file.journalStart = journalOffsetV3(next)
	fl.file.journalEnd = fl.file.journalStart + journalSizeV3
	fl.file.readOffset = fl.file.journalStart
	fl.file.writeOffset = fl.file.journalStart + int64(used)
	fl.file.appendErr = nil
	fl.file.activeUsed = int64(used)

	return nil
}

// createV3FileHandler creates the fixed-size log file version 3 and returns a [FileHandler]
// positioned at the beginning of the active journal.
// Initialization failures close and remove the file after it has been opened successfully.
func createV3FileHandler(
	filePath string,
	preallocate func(*os.File) error,
	syncDirectory func(string) error,
) (_ *FileHandler, err error) {
	fh, err := NewFileHandlerWithOpts(filePath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, logFilePerm)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			err = errors.Join(err, fh.Close(), os.Remove(filePath))
		}
	}()

	if err = preallocate(fh.file); err != nil {
		return nil, err
	}

	header := fileHeaderV3{generation: 1, journalSize: uint32(journalSizeV3)}
	if err = initializeV3File(fh, header); err != nil {
		return nil, err
	}

	if err = syncDirectory(filepath.Dir(filePath)); err != nil {
		return nil, err
	}

	fh.size = fileSizeV3
	fh.fixedSize = true
	fh.activeUsed = int64(header.activeUsed)
	fh.journalStart = journalOffsetV3(header.active)
	fh.journalEnd = fh.journalStart + journalSizeV3
	fh.readOffset = fh.journalStart
	fh.writeOffset = fh.readOffset + int64(header.activeUsed)

	return fh, nil
}

// decodeFileHeaderSlotsV3 selects the valid header with the greatest generation and its slot index.
func decodeFileHeaderSlotsV3(buf []byte) (fileHeaderV3, uint8, error) {
	header0, err0 := decodeFileHeaderV3(buf[:fileHeaderSlotSizeV3])
	header1, err1 := decodeFileHeaderV3(buf[fileHeaderSlotSizeV3:])

	switch {
	case err0 == nil && err1 == nil:
		if header1.generation > header0.generation {
			return header1, 1, nil
		}

		return header0, 0, nil
	case err0 == nil:
		return header0, 0, nil
	case err1 == nil:
		return header1, 1, nil
	default:
		return fileHeaderV3{}, 0, fmt.Errorf("%w: slot 0: %w; slot 1: %w", ErrInvalidV3Header, err0, err1)
	}
}

// decodeFileHeaderV3 decodes and validates one header slot of the log file version 3.
func decodeFileHeaderV3(src []byte) (fileHeaderV3, error) {
	if len(src) < fileHeaderSlotSizeV3 {
		return fileHeaderV3{}, fmt.Errorf("%w: slot is %d bytes", ErrInvalidV3Header, len(src))
	}

	slot := src[:fileHeaderSlotSizeV3]

	if slot[fileHeaderVersionOffsetV3] != byte(LogFileVersionV3) {
		return fileHeaderV3{}, fmt.Errorf("%w: version %d", ErrInvalidV3Header, slot[fileHeaderVersionOffsetV3])
	}

	if string(slot[fileHeaderMagicOffsetV3:fileHeaderMagicOffsetV3+len(fileMagicV3)]) != fileMagicV3 {
		return fileHeaderV3{}, fmt.Errorf("%w: bad magic", ErrInvalidV3Header)
	}

	if crc32.Checksum(slot[:fileHeaderChecksumOffsetV3], castagnoliTable) !=
		binary.LittleEndian.Uint32(slot[fileHeaderChecksumOffsetV3:]) {
		return fileHeaderV3{}, fmt.Errorf("%w: checksum mismatch", ErrInvalidV3Header)
	}

	header := fileHeaderV3{
		active:      slot[fileHeaderActiveOffsetV3],
		generation:  binary.LittleEndian.Uint64(slot[fileHeaderGenerationOffsetV3:]),
		journalSize: binary.LittleEndian.Uint32(slot[fileHeaderJournalSizeOffsetV3:]),
		activeUsed:  binary.LittleEndian.Uint32(slot[fileHeaderActiveUsedOffsetV3:]),
	}

	if header.active >= fileHeaderSlotsCount {
		return fileHeaderV3{}, fmt.Errorf("%w: active journal %d", ErrInvalidV3Header, header.active)
	}

	if header.journalSize != journalSizeV3 {
		return fileHeaderV3{}, fmt.Errorf("%w: journal size %d", ErrInvalidV3Header, header.journalSize)
	}

	if header.activeUsed >= journalSizeV3 {
		return fileHeaderV3{}, fmt.Errorf("%w: active used %d", ErrInvalidV3Header, header.activeUsed)
	}

	return header, nil
}

// encodeFileHeaderV3 encodes the header slot of the log file version 3 with the CRC32C over
// all the fields except the checksum itself; the reserved bytes are zeroed.
func encodeFileHeaderV3(header fileHeaderV3) [fileHeaderSlotSizeV3]byte {
	var slot [fileHeaderSlotSizeV3]byte
	slot[fileHeaderVersionOffsetV3] = byte(LogFileVersionV3)
	copy(slot[fileHeaderMagicOffsetV3:], fileMagicV3)
	slot[fileHeaderActiveOffsetV3] = header.active
	binary.LittleEndian.PutUint64(slot[fileHeaderGenerationOffsetV3:], header.generation)
	binary.LittleEndian.PutUint32(slot[fileHeaderJournalSizeOffsetV3:], header.journalSize)
	binary.LittleEndian.PutUint32(slot[fileHeaderActiveUsedOffsetV3:], header.activeUsed)
	binary.LittleEndian.PutUint32(
		slot[fileHeaderChecksumOffsetV3:],
		crc32.Checksum(slot[:fileHeaderChecksumOffsetV3], castagnoliTable),
	)

	return slot
}

// encodeJournalV3 builds a zero-padded journal image and returns its used length without the terminator.
func encodeJournalV3(records []*SerializedRecord) (image []byte, used int, err error) {
	image = make([]byte, 0, journalSizeV3)
	for _, record := range records {
		snapshot := *record
		snapshot.fields = fullFields(&snapshot)
		image, err = appendFrameV3(image, &snapshot)
		if err != nil {
			return nil, 0, err
		}

		if len(image) >= journalSizeV3 {
			return nil, 0, ErrJournalFull
		}
	}

	used = len(image)
	image = image[:journalSizeV3]
	clear(image[used:])

	return image, used, nil
}

// initializeV3File writes both header slots and empty journal terminators, then syncs the file.
func initializeV3File(fh *FileHandler, header fileHeaderV3) error {
	slot := encodeFileHeaderV3(header)
	for i := range uint8(fileHeaderSlotsCount) {
		if _, err := fh.file.WriteAt(slot[:], int64(i)*fileHeaderSlotSizeV3); err != nil {
			return fmt.Errorf("write v3 file header: %w", err)
		}
	}

	for i := range uint8(fileHeaderSlotsCount) {
		if _, err := fh.file.WriteAt([]byte{recordVersionEndV3}, journalOffsetV3(i)); err != nil {
			return fmt.Errorf("write v3 journal terminator: %w", err)
		}
	}

	if err := fh.Sync(); err != nil {
		return fmt.Errorf("sync v3 file: %w", err)
	}

	return nil
}

// journalOffsetV3 returns the offset of the journal with the given index in the log file version 3.
func journalOffsetV3(index uint8) int64 {
	return fileHeaderAreaSizeV3 + int64(index)*journalSizeV3
}

// openV3FileHandler restores the append position by validating the active journal up to its terminator.
// A corrupt journal remains readable for Catalog.sync to rewrite, but cannot be appended to.
func openV3FileHandler(fh *FileHandler, header fileHeaderV3) error {
	if err := verifyPreallocatedSize(fh.file); err != nil {
		return err
	}

	fh.fixedSize = true
	fh.activeUsed = int64(header.activeUsed)
	fh.journalStart = journalOffsetV3(header.active)
	fh.journalEnd = fh.journalStart + journalSizeV3
	fh.readOffset = fh.journalStart
	fh.writeOffset = fh.journalStart

	reader := recordReader{file: fh, minUsed: int64(header.activeUsed)}
	for {
		var record SerializedRecord
		if err := reader.Read(&record); err != nil {
			fh.readOffset = fh.journalStart
			if errors.Is(err, io.EOF) {
				fh.activeUsed = fh.writeOffset - fh.journalStart

				return nil
			}

			fh.appendErr = fmt.Errorf("v3: journal requires rewrite at %d: %w", fh.writeOffset, err)

			return nil
		}

		fh.writeOffset = fh.readOffset
	}
}

// preallocateV3File allocates the fixed-size file, falls back to zero writes, and verifies its size.
func preallocateV3File(f *os.File) error {
	if err := preallocateFile(f, fileSizeV3); err != nil {
		if !errors.Is(err, errPreallocUnsupported) {
			return fmt.Errorf("preallocate v3 file: %w", err)
		}

		if err = writeZeros(f, fileSizeV3); err != nil {
			return fmt.Errorf("preallocate v3 file: %w", err)
		}
	}

	return verifyPreallocatedSize(f)
}

// readFileHeaderV3 reads both header slots of the log file version 3 and returns the valid slot
// with the maximum generation and its slot index: one corrupt slot loses to the other one, two corrupt slots fail
// with [ErrInvalidV3Header].
func readFileHeaderV3(f *os.File) (fileHeaderV3, uint8, error) {
	buf := make([]byte, fileHeaderSlotsCount*fileHeaderSlotSizeV3)
	if _, err := f.ReadAt(buf, 0); err != nil {
		return fileHeaderV3{}, 0, fmt.Errorf("read v3 file header: %w", err)
	}

	return decodeFileHeaderSlotsV3(buf)
}

// syncDir fsyncs the directory to make file creation durable.
func syncDir(dirPath string) error {
	dir, err := os.Open(dirPath) //nolint:gosec // fsync of the directory requires opening it
	if err != nil {
		return fmt.Errorf("open dir: %w", err)
	}

	if err = dir.Sync(); err != nil {
		return errors.Join(fmt.Errorf("sync dir: %w", err), dir.Close())
	}

	return dir.Close()
}

// tryOpenV3FileHandler recognizes fixed v3 before falling back to a legacy version header.
// A complete v1/v2 version header takes precedence over magic bytes in record data.
func tryOpenV3FileHandler(fh *FileHandler) (bool, error) {
	var slots [fileHeaderSlotsCount * fileHeaderSlotSizeV3]byte
	if _, err := fh.file.ReadAt(slots[:], 0); err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("read file header: %w", err)
	}

	version := binary.LittleEndian.Uint64(slots[:8])
	if version == LogFileVersionV1 || version == LogFileVersionV2 {
		return false, nil
	}

	header, _, err := decodeFileHeaderSlotsV3(slots[:])
	if err == nil {
		return true, openV3FileHandler(fh, header)
	}

	slot0 := slots[:fileHeaderSlotSizeV3]
	slot1 := slots[fileHeaderSlotSizeV3:]
	magic0 := string(slot0[fileHeaderMagicOffsetV3 : fileHeaderMagicOffsetV3+len(fileMagicV3)])
	magic1 := string(slot1[fileHeaderMagicOffsetV3 : fileHeaderMagicOffsetV3+len(fileMagicV3)])
	if magic0 == fileMagicV3 || magic1 == fileMagicV3 ||
		slots[0] == byte(LogFileVersionV3) {
		return true, err
	}

	return false, nil
}

// updateV3ActiveUsed persists the used length of the active journal of a fixed-size log file
// version 3 into the other header slot: the same active journal, generation + 1, a fresh CRC,
// one WriteAt + fsync. A close without new records leaves the header untouched.
func updateV3ActiveUsed(fh *FileHandler) error {
	if fh.appendErr != nil {
		return nil
	}

	header, headerSlot, err := readFileHeaderV3(fh.file)
	if err != nil {
		return err
	}

	used := fh.writeOffset - journalOffsetV3(header.active)
	if used == int64(header.activeUsed) {
		return nil
	}

	if used < 0 || used >= journalSizeV3 {
		return fmt.Errorf("%w: active used %d", ErrInvalidV3Header, used)
	}

	header.generation++
	header.activeUsed = uint32(used) // #nosec G115 // bounded above by journalSizeV3

	slot := encodeFileHeaderV3(header)
	if _, err = fh.writeAt(slot[:], int64(headerSlot^1)*fileHeaderSlotSizeV3); err != nil {
		return fmt.Errorf("write v3 file header: %w", err)
	}

	return fh.Sync()
}

// verifyPreallocatedSize checks that the file is allocated up to the expected size.
func verifyPreallocatedSize(f *os.File) error {
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat v3 file: %w", err)
	}

	if info.Size() != fileSizeV3 {
		return fmt.Errorf("%w: preallocated file size: got %d, want %d", ErrInvalidV3FileSize, info.Size(), fileSizeV3)
	}

	return nil
}
