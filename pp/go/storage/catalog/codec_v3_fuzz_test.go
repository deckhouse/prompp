package catalog

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func FuzzEncodeDecodeV3(f *testing.F) {
	f.Add(uint16(4), int64(100), int64(200), int64(0), false, uint8(StatusActive), uint32(10), int64(1), int64(2))

	f.Fuzz(func(
		t *testing.T,
		numberOfShards uint16,
		createdAt, updatedAt, deletedAt int64,
		corrupted bool,
		status uint8,
		segmentsCount uint32,
		mint, maxt int64,
	) {
		sr := &SerializedRecord{
			id:               uuid.MustParse("01929a6c-7b1e-7000-8000-000000000003"),
			numberOfShards:   numberOfShards,
			createdAt:        createdAt,
			updatedAt:        updatedAt,
			deletedAt:        deletedAt,
			corrupted:        corrupted,
			status:           Status(status),
			numberOfSegments: segmentsCount,
			mint:             mint,
			maxt:             maxt,
			fields:           fieldsAll,
		}
		sr.lastAppendedSegmentID = lastAppendedSegmentIDByNumberOfSegments(segmentsCount)

		frame, err := appendFrameV3(nil, sr)
		if err != nil {
			t.Fatal(err)
		}

		decoded := &SerializedRecord{}
		if _, err = decodeFrame(frame, decoded); err != nil {
			t.Fatal(err)
		}

		if *decoded != *sr {
			t.Fatalf("decoded record %+v differs from %+v", decoded, sr)
		}
	})
}

func FuzzDecodePayloadV3(f *testing.F) {
	for _, golden := range []string{goldenFullFrameV3, goldenStatusFrameV3} {
		frame, err := hex.DecodeString(golden)
		if err != nil {
			f.Fatal(err)
		}

		f.Add(frame[recordHeaderSizeV3:])
	}

	for _, payload := range [][]byte{
		{},
		{0},
		{0x08, 0},
		{0x08, 0xff, 0xff, 3},
		{0x08, 0x80, 0x80, 4},
		{0x28, 2},
		{0x30, 0x80, 2},
		{0x38, 0xff, 0xff, 0xff, 0xff, 0x0f},
		{0x08, 1, 0x08, 2},
		{0x52, 2, 0, 0},
		{0x53, 0x54},
		{0x53},
	} {
		f.Add(payload)
	}

	f.Fuzz(func(t *testing.T, payload []byte) {
		if len(payload) > maxRecordPayloadSizeV3 {
			t.Skip()
		}

		// Recompute CRC so malformed protobuf reaches the payload decoder.
		frame := make([]byte, recordHeaderSizeV3, recordHeaderSizeV3+len(payload))
		frame[0] = RecordVersionV3
		frame[recordLengthOffsetV3] = byte(len(payload))
		frame = append(frame, payload...)
		binary.LittleEndian.PutUint32(frame[recordChecksumOffsetV3:], checksumV3(frame))
		original := bytes.Clone(frame)

		var decoded SerializedRecord
		n, err := decodeFrame(frame, &decoded)
		require.Equal(t, len(frame), n)
		require.Equal(t, original, frame)
		require.NotErrorIs(t, err, ErrRecordChecksumMismatch)

		if err != nil {
			return
		}

		canonical, err := appendFrameV3(nil, &decoded)
		require.NoError(t, err)

		var roundTrip, streamed SerializedRecord
		_, err = decodeFrame(canonical, &roundTrip)
		require.NoError(t, err)
		require.Equal(t, decoded, roundTrip)
		require.NoError(t, NewDecoderV3().DecodeFrom(bytes.NewReader(frame), &streamed))
		require.Equal(t, decoded, streamed)
	})
}
