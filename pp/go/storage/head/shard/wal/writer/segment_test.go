package writer_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/writer"
)

func TestWriteSegment(t *testing.T) {
	data := []byte{1, 2, 3, 2, 1, 0, 42}
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
	expected := []byte{byte(len(data)), byte(segmentCrc32), byte(segmentSamples)}
	expected = append(expected, data...)

	_, err := writer.WriteSegment(buf, segment)
	require.NoError(t, err)

	require.Equal(t, expected, buf.Bytes())
}
