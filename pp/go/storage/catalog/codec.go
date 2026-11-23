package catalog

import (
	"errors"
	"fmt"
	"io"
)

//
// Encoder
//

// Encoder encodes [SerializedRecord].
type Encoder interface {
	// EncodeTo encode [SerializedRecord] to [io.Writer].
	EncodeTo(writer io.Writer, sr *SerializedRecord) error
}

//
// Decoder
//

// Decoder decodes [SerializedRecord].
type Decoder interface {
	// DecodeFrom decode [SerializedRecord] from [io.Reader].
	DecodeFrom(reader io.Reader, sr *SerializedRecord) error
}

// ErrUnsupportedRecordVersion the record version is unknown.
var ErrUnsupportedRecordVersion = errors.New("unsupported record version")

// codecsByVersion select codec by version.
func codecsByVersion(version uint64) (e Encoder, d Decoder, err error) {
	switch version {
	case LogFileVersionV1:
		return EncoderV1{}, DecoderV1{}, nil
	case LogFileVersionV2:
		return NewEncoderV2(), DecoderV2{}, nil
	case LogFileVersionV3:
		return NewEncoderV3(), NewDecoderV3(), nil
	default:
		return nil, nil, ErrUnsupportedVersion
	}
}

// decodeFrame decodes the record frame of any known version from the beginning of buf.
// Returns the size of the frame, it is also returned with [ErrRecordChecksumMismatch] as the frame header states.
func decodeFrame(buf []byte, sr *SerializedRecord) (int, error) {
	if len(buf) == 0 {
		return 0, io.ErrUnexpectedEOF
	}

	switch buf[0] {
	case RecordVersionV3:
		return decodeFrameV3(buf, sr)
	default:
		return 0, fmt.Errorf("%w: %d", ErrUnsupportedRecordVersion, buf[0])
	}
}
