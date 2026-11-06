package catalog

import (
	"fmt"
	"io"

	"github.com/google/uuid"
)

// CorruptedRecordError identifies a damaged frame and its absolute file offset.
// The next Read resumes at the next validated frame, or at the journal end.
type CorruptedRecordError struct {
	ID     uuid.UUID
	Offset int64
	Err    error
}

func (e *CorruptedRecordError) Error() string {
	return fmt.Sprintf("corrupted record %s at %d: %v", e.ID, e.Offset, e.Err)
}

func (e *CorruptedRecordError) Unwrap() error {
	return e.Err
}

// recordReader reads bounded v3 frames and searches past corruption using validated checksums.
type recordReader struct {
	file          *FileHandler
	minUsed       int64
	buffer        [maxRecordSizeV3]byte
	lastReadValid bool
}

// Read decodes the next frame in the active journal into [*SerializedRecord] and advances the read offset.
// On corruption it returns [CorruptedRecordError] and positions the next read at a valid frame or the journal end.
// A clean terminator or an exhausted journal returns [io.EOF]; [*SerializedRecord] is unchanged on error.
func (r *recordReader) Read(sr *SerializedRecord) error {
	fh := r.file
	offset := fh.readOffset
	if offset >= fh.journalEnd {
		if r.lastReadValid {
			r.lastReadValid = false

			return &CorruptedRecordError{Offset: offset, Err: io.ErrUnexpectedEOF}
		}

		return io.EOF
	}

	n, err := fh.file.ReadAt(r.buffer[:min(int64(len(r.buffer)), fh.journalEnd-offset)], offset)
	if err != nil {
		return unexpectedEOF(err)
	}

	buf := r.buffer[:n]
	if len(buf) > 0 && buf[0] == RecordVersionV3 {
		var decoded SerializedRecord
		size, decodeErr := decodeFrameV3(buf, &decoded)
		if decodeErr == nil {
			*sr = decoded
			fh.readOffset += int64(size)
			r.lastReadValid = true

			return nil
		}

		err = decodeErr
	} else {
		err = ErrUnsupportedRecordVersion
	}

	return r.resynchronize(offset, err)
}

// resynchronize distinguishes a clean terminator from corruption and resumes at the next valid frame.
func (r *recordReader) resynchronize(offset int64, cause error) error {
	fh := r.file
	r.lastReadValid = false
	rest := make([]byte, fh.journalEnd-offset)
	if _, readErr := fh.file.ReadAt(rest, offset); readErr != nil {
		return unexpectedEOF(readErr)
	}

	if rest[0] == recordVersionEndV3 && offset-fh.journalStart >= r.minUsed &&
		allZeroV3(rest) {
		return io.EOF
	}

	corrupted := &CorruptedRecordError{Offset: offset, Err: cause}
	if len(rest) >= recordLengthOffsetV3 {
		copy(corrupted.ID[:], rest[recordIDOffsetV3:recordLengthOffsetV3])
	}

	fh.readOffset = offset + int64(findNextFrameV3(rest))

	return corrupted
}

// allZeroV3 checks the unused tail without allocating another journal buffer.
func allZeroV3(buf []byte) bool {
	for _, b := range buf {
		if b != 0 {
			return false
		}
	}

	return true
}

// findNextFrameV3 returns the next valid frame offset, or len(buf) if no frame remains.
// It tries the advertised boundary first, then searches byte by byte when the length is damaged.
func findNextFrameV3(buf []byte) int {
	if len(buf) >= recordHeaderSizeV3 {
		next := recordHeaderSizeV3 + int(buf[recordLengthOffsetV3])
		if next < len(buf) && validFrameV3(buf[next:]) {
			return next
		}
	}

	for next := 1; next+recordHeaderSizeV3 <= len(buf); next++ {
		if validFrameV3(buf[next:]) {
			return next
		}
	}

	return len(buf)
}

// validFrameV3 checks both the version and the complete frame, including its payload.
func validFrameV3(buf []byte) bool {
	if len(buf) == 0 || buf[0] != RecordVersionV3 {
		return false
	}

	var sr SerializedRecord
	_, err := decodeFrameV3(buf, &sr)

	return err == nil
}
