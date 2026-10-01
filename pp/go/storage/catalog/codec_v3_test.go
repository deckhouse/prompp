package catalog

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"io"
	"math"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/encoding/protowire"
)

const (
	// goldenFullFrameV3 is the frame of the record from newCodecRecord with all its known fields.
	goldenFullFrameV3 = "03" + "01929a6c7b1e70008000000000000001" + "0d" + "3e2f8a16" +
		"0804" + "1064" + "18c801" + "2800" + "3004" + "380a"
	// goldenStatusFrameV3 is the frame of the status change of the record from newCodecRecord.
	goldenStatusFrameV3 = "03" + "01929a6c7b1e70008000000000000001" + "07" + "09505421" +
		"18ac02" + "3001" + "380a"
)

type CodecV3Suite struct {
	suite.Suite
}

func TestCodecV3Suite(t *testing.T) {
	suite.Run(t, new(CodecV3Suite))
}

func newCodecRecord() *SerializedRecord {
	sr := &SerializedRecord{
		id:               uuid.MustParse("01929a6c-7b1e-7000-8000-000000000001"),
		numberOfShards:   4,
		createdAt:        100,
		updatedAt:        200,
		status:           StatusActive,
		numberOfSegments: 10,
		mint:             math.MaxInt64,
		maxt:             math.MinInt64,
	}
	sr.lastAppendedSegmentID = lastAppendedSegmentIDByNumberOfSegments(sr.numberOfSegments)
	sr.fields = fullFields(sr)

	return sr
}

func (s *CodecV3Suite) encode(sr *SerializedRecord) []byte {
	buf := &bytes.Buffer{}
	s.Require().NoError(NewEncoderV3().EncodeTo(buf, sr))

	return buf.Bytes()
}

func (s *CodecV3Suite) decode(frame []byte) (*SerializedRecord, error) {
	sr := &SerializedRecord{mint: math.MaxInt64, maxt: math.MinInt64}
	err := NewDecoderV3().DecodeFrom(bytes.NewReader(frame), sr)

	return sr, err
}

// frameWithPayload builds a valid frame of the record version 3 with an arbitrary payload.
func (*CodecV3Suite) frameWithPayload(payload []byte) []byte {
	frame := make([]byte, recordHeaderSizeV3, recordHeaderSizeV3+len(payload))
	frame[0] = RecordVersionV3
	frame[recordLengthOffsetV3] = uint8(len(payload))
	frame = append(frame, payload...)
	binary.LittleEndian.PutUint32(frame[recordChecksumOffsetV3:], checksumV3(frame))

	return frame
}

func (s *CodecV3Suite) TestEncodeFullRecord() {
	frame := s.encode(newCodecRecord())

	s.Equal(goldenFullFrameV3, hex.EncodeToString(frame))
}

func (s *CodecV3Suite) TestEncodeStatusChange() {
	sr := newCodecRecord()
	sr.status = StatusRotated
	sr.updatedAt = 300
	sr.fields = fieldStatus | fieldUpdatedAt | fieldSegmentsCount

	frame := s.encode(sr)

	s.Equal(goldenStatusFrameV3, hex.EncodeToString(frame))
}

func (s *CodecV3Suite) TestDecodeFullRecord() {
	expected := newCodecRecord()
	expected.deletedAt = 300
	expected.corrupted = true
	expected.mint = 10
	expected.maxt = 20
	expected.fields = fullFields(expected)

	decoded, err := s.decode(s.encode(expected))

	s.Require().NoError(err)
	s.Equal(expected, decoded)
}

func (s *CodecV3Suite) TestDecodeSingleField() {
	testCases := []struct {
		name  string
		field fieldMask
	}{
		{"number of shards", fieldNumberOfShards},
		{"created at", fieldCreatedAt},
		{"updated at", fieldUpdatedAt},
		{"deleted at", fieldDeletedAt},
		{"corrupted", fieldCorrupted},
		{"status", fieldStatus},
		{"segments count", fieldSegmentsCount},
		{"mint", fieldMinT},
		{"maxt", fieldMaxT},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			sr := &SerializedRecord{
				id:               uuid.MustParse("01929a6c-7b1e-7000-8000-000000000002"),
				numberOfShards:   8,
				createdAt:        1,
				updatedAt:        2,
				deletedAt:        3,
				corrupted:        true,
				status:           StatusPersisted,
				numberOfSegments: 7,
				mint:             -5,
				maxt:             5,
				fields:           tc.field,
			}
			expected := &SerializedRecord{id: sr.id, mint: math.MaxInt64, maxt: math.MinInt64}
			applyRecordChanges(expected, sr, tc.field)
			expected.fields = tc.field

			decoded, err := s.decode(s.encode(sr))

			s.Require().NoError(err)
			s.Equal(expected, decoded)
		})
	}
}

func (s *CodecV3Suite) TestDecodeZeroValuesArePresent() {
	sr := newCodecRecord()
	sr.status = StatusNew
	sr.numberOfSegments = 0
	sr.fields = fieldStatus | fieldCorrupted | fieldSegmentsCount

	decoded, err := s.decode(s.encode(sr))

	s.Require().NoError(err)
	s.Equal(fieldStatus|fieldCorrupted|fieldSegmentsCount, decoded.fields)
	s.Equal(StatusNew, decoded.status)
	s.False(decoded.corrupted)
	s.Equal(uint32(0), decoded.numberOfSegments)
	s.True(decoded.lastAppendedSegmentID.IsNil())
}

func (s *CodecV3Suite) TestDecodeChecksumMismatch() {
	testCases := []struct {
		name   string
		offset int
		value  byte
	}{
		{"id", recordIDOffsetV3 + 4, 0xff},
		{"payload length", recordLengthOffsetV3, 0x0c},
		{"payload", recordHeaderSizeV3 + 1, 0x05},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			frame := s.encode(newCodecRecord())
			frame[tc.offset] = tc.value

			_, err := s.decode(frame)

			s.ErrorIs(err, ErrRecordChecksumMismatch)
		})
	}
}

func (s *CodecV3Suite) TestDecodeFrameReturnsSizeOnChecksumMismatch() {
	frame := s.encode(newCodecRecord())
	frame[recordHeaderSizeV3] ^= 0xff

	n, err := decodeFrame(append(frame, RecordVersionV3), &SerializedRecord{})

	s.Require().ErrorIs(err, ErrRecordChecksumMismatch)
	s.Equal(len(frame), n)
}

func (s *CodecV3Suite) TestDecodeTruncatedFrame() {
	testCases := []struct {
		name string
		size int
	}{
		{"header", recordHeaderSizeV3 - 1},
		{"payload", len(goldenFullFrameV3)/2 - 1},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			frame := s.encode(newCodecRecord())

			_, err := s.decode(frame[:tc.size])

			s.ErrorIs(err, io.ErrUnexpectedEOF)
		})
	}
}

func (s *CodecV3Suite) TestDecodeEmptyStream() {
	_, err := s.decode(nil)

	s.ErrorIs(err, io.EOF)
}

func (s *CodecV3Suite) TestDecodeUnsupportedVersion() {
	frame := s.encode(newCodecRecord())
	frame[0] = RecordVersionV3 + 1

	_, err := s.decode(frame)

	s.ErrorIs(err, ErrUnsupportedRecordVersion)
}

func (s *CodecV3Suite) TestDecodeSkipsUnknownFields() {
	payload := protowire.AppendTag(nil, protoStatus, protowire.VarintType)
	payload = protowire.AppendVarint(payload, uint64(StatusRotated))
	payload = protowire.AppendTag(payload, 15, protowire.BytesType)
	payload = protowire.AppendBytes(payload, []byte("future"))
	payload = protowire.AppendTag(payload, 16, protowire.VarintType)
	payload = protowire.AppendVarint(payload, 42)

	decoded, err := s.decode(s.frameWithPayload(payload))

	s.Require().NoError(err)
	s.Equal(fieldStatus, decoded.fields)
	s.Equal(StatusRotated, decoded.status)
}

func (s *CodecV3Suite) TestDecodeInvalidPayload() {
	testCases := []struct {
		name    string
		payload []byte
	}{
		{"unexpected wire type", protowire.AppendFixed32(protowire.AppendTag(nil, protoStatus, protowire.Fixed32Type), 1)},
		{"truncated varint", protowire.AppendTag(nil, protoCreatedAt, protowire.VarintType)},
		{"number of shards out of range", appendVarintField(nil, protoNumberOfShards, math.MaxUint16+1)},
		{"status out of range", appendVarintField(nil, protoStatus, math.MaxUint8+1)},
		{"segments count out of range", appendVarintField(nil, protoSegmentsCount, math.MaxUint32+1)},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			_, err := s.decode(s.frameWithPayload(tc.payload))

			s.Error(err)
		})
	}
}

func (s *CodecV3Suite) TestEncodeDecodeWithoutAllocations() {
	sr := newCodecRecord()
	encoder := NewEncoderV3()
	decoder := NewDecoderV3()
	frame := s.encode(sr)
	reader := bytes.NewReader(frame)
	decoded := &SerializedRecord{}

	allocs := testing.AllocsPerRun(100, func() {
		_ = encoder.EncodeTo(io.Discard, sr)
		reader.Reset(frame)
		_ = decoder.DecodeFrom(reader, decoded)
	})

	s.Zero(allocs)
}

func FuzzDecodeFrame(f *testing.F) {
	for _, golden := range []string{goldenFullFrameV3, goldenStatusFrameV3} {
		frame, err := hex.DecodeString(golden)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(frame)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		n, err := decodeFrame(data, &SerializedRecord{})
		if n > len(data) {
			t.Fatalf("frame size %d exceeds the buffer size %d", n, len(data))
		}

		if err == nil && n < recordHeaderSizeV3 {
			t.Fatalf("decoded frame size %d is less than the header size", n)
		}
	})
}

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

func BenchmarkEncoderV3(b *testing.B) {
	sr := newCodecRecord()
	encoder := NewEncoderV3()
	b.ReportAllocs()

	for b.Loop() {
		if err := encoder.EncodeTo(io.Discard, sr); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecoderV3(b *testing.B) {
	frame, err := appendFrameV3(nil, newCodecRecord())
	if err != nil {
		b.Fatal(err)
	}
	reader := bytes.NewReader(frame)
	decoder := NewDecoderV3()
	decoded := &SerializedRecord{}
	b.ReportAllocs()

	for b.Loop() {
		reader.Reset(frame)
		if err = decoder.DecodeFrom(reader, decoded); err != nil {
			b.Fatal(err)
		}
	}
}
