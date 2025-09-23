package writer_test

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/go-faker/faker/v4"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/writer"
)

func TestWriteSegment(t *testing.T) {
	data := []byte(faker.Paragraph())
	segmentCrc32 := uint32(0)
	segmentSamples := uint32(42)

	segment := &EncodedSegmentMock{
		CRC32Func: func() uint32 {
			return segmentCrc32
		},
		SamplesFunc: func() uint32 {
			return segmentSamples
		},
		SizeFunc: func() int64 {
			return int64(len(data))
		},
		WriteToFunc: func(w io.Writer) (int64, error) {
			n, err := w.Write(data)
			return int64(n), err
		},
	}

	buf := &bytes.Buffer{}
	expected := []byte{}
	expected = append(expected, binary.AppendUvarint(nil, uint64(len(data)))...)
	expected = append(expected, byte(segmentCrc32), byte(segmentSamples))
	expected = append(expected, data...)

	_, err := writer.WriteSegment(buf, segment)
	require.NoError(t, err)

	require.Equal(t, expected, buf.Bytes())
}
