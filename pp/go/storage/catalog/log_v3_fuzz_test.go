package catalog

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func FuzzFileHeaderSlotsV3(f *testing.F) {
	header := fileHeaderV3{journalSize: journalSizeV3}
	first := encodeFileHeaderV3(header)
	header.active = 1
	header.generation = 1
	header.activeUsed = journalSizeV3 - 1
	second := encodeFileHeaderV3(header)
	slots := append(first[:], second[:]...)
	f.Add(slots, false)
	f.Add(slots, true)
	f.Add([]byte{}, false)
	f.Add([]byte{RecordVersionV3}, true)

	f.Fuzz(func(t *testing.T, data []byte, repairCRC bool) {
		// File readers always supply two complete slots, including after a short read.
		var buf [fileHeaderSlotsCount * fileHeaderSlotSizeV3]byte
		copy(buf[:], data)

		if repairCRC {
			for i := range fileHeaderSlotsCount {
				slot := buf[i*fileHeaderSlotSizeV3 : (i+1)*fileHeaderSlotSizeV3]
				binary.LittleEndian.PutUint32(slot[fileHeaderChecksumOffsetV3:],
					crc32.Checksum(slot[:fileHeaderChecksumOffsetV3], castagnoliTable))
			}
		}

		first, firstErr := decodeFileHeaderV3(buf[:fileHeaderSlotSizeV3])
		second, secondErr := decodeFileHeaderV3(buf[fileHeaderSlotSizeV3:])
		header, slot, err := decodeFileHeaderSlotsV3(buf[:])

		if firstErr != nil && secondErr != nil {
			require.ErrorIs(t, err, ErrInvalidV3Header)

			return
		}

		require.NoError(t, err)
		require.Less(t, header.active, uint8(fileHeaderSlotsCount))
		require.Equal(t, uint32(journalSizeV3), header.journalSize)
		require.Less(t, header.activeUsed, uint32(journalSizeV3))

		if firstErr == nil && (secondErr != nil || first.generation >= second.generation) {
			require.Zero(t, slot)
			require.Equal(t, first, header)
		} else {
			require.Equal(t, uint8(1), slot)
			require.Equal(t, second, header)
		}

		canonical := encodeFileHeaderV3(header)
		decoded, err := decodeFileHeaderV3(canonical[:])
		require.NoError(t, err)
		require.Equal(t, header, decoded)
	})
}

//revive:disable-next-line:cyclomatic // this is test.
//revive:disable-next-line:cognitive-complexity // this is test.
func FuzzFileLogV3(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0, 1, 2, 3, 0, 2, 7, 0, 2, 3})
	f.Add([]byte{0, 0xff, 0, 0x12, 0, 0x16, 0, 2, 7})

	f.Fuzz(func(t *testing.T, operations []byte) {
		if len(operations) > 24 {
			t.Skip()
		}

		path := filepath.Join(t.TempDir(), "head.log")
		fl, err := NewFileLogV3(path)
		require.NoError(t, err)
		t.Cleanup(func() {
			if fl != nil {
				require.NoError(t, fl.Close())
			}
		})
		var expected []SerializedRecord

		for i, operation := range operations {
			switch operation & 3 {
			case 0, 1:
				sr := *newFuzzRecordV3()
				sr.id[15] = byte(i)
				sr.status = Status(operation)
				sr.updatedAt = int64(i)
				sr.numberOfShards = uint16(operation)
				require.NoError(t, fl.Write(&sr))
				expected = append(expected, sr)
			case 2:
				if operation&0x10 != 0 && len(expected) > 0 {
					expected = expected[len(expected)/2:]
				}

				snapshots := make([]*SerializedRecord, len(expected))
				for j := range expected {
					snapshots[j] = &expected[j]
				}

				require.NoError(t, fl.ReWrite(snapshots...))
			case 3:
				if operation&4 != 0 {
					// Close only the descriptor to simulate losing the header checkpoint.
					require.NoError(t, fl.file.Close())
				} else {
					require.NoError(t, fl.Close())
				}

				fl, err = NewFileLogV3(path)
				require.NoError(t, err)
			}
		}

		check := func() {
			t.Helper()
			require.Equal(t, fileSizeV3, fl.Size())
			fl.file.readOffset = fl.file.journalStart
			fl.reader = recordReader{}

			for _, want := range expected {
				var got SerializedRecord
				require.NoError(t, fl.Read(&got))
				require.Equal(t, want, got)
			}

			var got SerializedRecord
			require.ErrorIs(t, fl.Read(&got), io.EOF)
			require.ErrorIs(t, fl.Read(&got), io.EOF)
		}
		check()
		require.NoError(t, fl.Close())
		fl, err = NewFileLogV3(path)
		require.NoError(t, err)
		check()
	})
}

//revive:disable-next-line:cognitive-complexity // this is test.
func FuzzRecordReaderV3(f *testing.F) {
	frame, err := appendFrameV3(nil, newFuzzRecordV3())
	if err != nil {
		f.Fatal(err)
	}

	badChecksum := append([]byte(nil), frame...)
	badChecksum[recordChecksumOffsetV3] ^= 0xff
	badLength := append([]byte(nil), frame...)
	badLength[recordLengthOffsetV3] = maxRecordPayloadSizeV3

	for _, prefix := range [][]byte{
		nil,
		{0},
		{0xff},
		frame,
		frame[:len(frame)-1],
		badChecksum,
		badLength,
	} {
		f.Add(prefix, uint16(0), true)
		f.Add(prefix, uint16(1), false)
	}

	f.Fuzz(func(t *testing.T, prefix []byte, checkpoint uint16, terminated bool) {
		if len(prefix) > 4096 {
			t.Skip()
		}

		// A full frame of zero padding prevents a prefix frame from consuming the sentinel.
		journal := append([]byte(nil), prefix...)
		journal = append(journal, make([]byte, maxRecordSizeV3)...)
		sentinelOffset := len(journal)
		sentinel := *newFuzzRecordV3()
		sentinel.id[15] = 0xff
		journal, err := appendFrameV3(journal, &sentinel)
		require.NoError(t, err)

		if terminated {
			journal = append(journal, make([]byte, maxRecordSizeV3)...)
		}

		path := filepath.Join(t.TempDir(), "journal")
		file, err := os.Create(path)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, file.Close()) })

		// Valid frames outside the journal bounds must never be returned.
		image := append(make([]byte, fileHeaderAreaSizeV3), journal...)
		image = append(image, frame...)
		_, err = file.Write(image)
		require.NoError(t, err)
		fh := &FileHandler{
			file:         file,
			fixedSize:    true,
			journalStart: fileHeaderAreaSizeV3,
			journalEnd:   int64(fileHeaderAreaSizeV3 + len(journal)),
			readOffset:   fileHeaderAreaSizeV3,
		}
		reader := recordReader{file: fh, minUsed: int64(checkpoint) % int64(len(journal)+1)}
		foundSentinel := false
		missingTerminatorReported := false
		sr := *newFuzzRecordV3()

		for range len(journal) + 2 {
			before, previous := fh.readOffset, sr
			err := reader.Read(&sr)
			require.GreaterOrEqual(t, fh.readOffset, before)
			require.LessOrEqual(t, fh.readOffset, fh.journalEnd)

			if err == nil {
				offset := int(before - fh.journalStart)
				var expected SerializedRecord
				n, decodeErr := decodeFrame(journal[offset:], &expected)
				require.NoError(t, decodeErr)
				require.Equal(t, expected, sr)
				require.Equal(t, before+int64(n), fh.readOffset)

				if offset == sentinelOffset {
					require.Equal(t, sentinel, sr)
					foundSentinel = true
				}

				continue
			}

			require.Equal(t, previous, sr)

			if errors.Is(err, io.EOF) {
				require.True(t, foundSentinel, "reader lost the valid frame after the damaged prefix")
				require.Equal(t, !terminated, missingTerminatorReported)
				require.ErrorIs(t, reader.Read(&sr), io.EOF)
				require.Equal(t, previous, sr)

				return
			}

			var corrupted *CorruptedRecordError
			require.ErrorAs(t, err, &corrupted)
			require.Equal(t, before, corrupted.Offset)

			if before < fh.journalEnd {
				require.Greater(t, fh.readOffset, before, "corruption must advance the reader")
			} else {
				require.ErrorIs(t, err, io.ErrUnexpectedEOF)
				require.False(t, missingTerminatorReported)
				missingTerminatorReported = true
			}
		}

		t.Fatal("reader did not terminate within the journal bounds")
	})
}

// newFuzzRecordV3 returns a deterministic full snapshot with valid time bounds.
func newFuzzRecordV3() *SerializedRecord {
	sr := newCodecRecord()
	sr.mint, sr.maxt = -10, 20
	sr.fields = fullFields(sr)

	return sr
}
