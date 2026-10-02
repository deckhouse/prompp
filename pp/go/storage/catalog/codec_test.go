package catalog

import (
	"encoding/hex"
	"io"
	"math"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
)

type CodecSuite struct {
	suite.Suite
}

func TestCodecSuite(t *testing.T) {
	suite.Run(t, new(CodecSuite))
}

// newCodecRecord returns a record with all its known fields present.
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

func (s *CodecSuite) TestDecodeFrameEmptyBuffer() {
	_, err := decodeFrame(nil, &SerializedRecord{})

	s.ErrorIs(err, io.ErrUnexpectedEOF)
}

func (s *CodecSuite) TestDecodeFrameUnsupportedVersion() {
	n, err := decodeFrame([]byte{0, 1, 2}, &SerializedRecord{})

	s.Require().ErrorIs(err, ErrUnsupportedRecordVersion)
	s.Zero(n)
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
