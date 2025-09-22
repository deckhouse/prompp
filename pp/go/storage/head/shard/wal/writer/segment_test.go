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
	segment := &testSegment{
		size:    int64(len(data)),
		samples: 42,
		data:    data,
	}
	buf := &bytes.Buffer{}
	expected := []byte{byte(len(data)), byte(segment.crc32), byte(segment.samples)}
	expected = append(expected, data...)

	_, err := writer.WriteSegment(buf, segment)
	require.NoError(t, err)

	require.Equal(t, expected, buf.Bytes())
}

//
// testSegment
//

// testSegment implementation [writer.EncodedSegment].
type testSegment struct {
	size    int64
	samples uint32
	crc32   uint32
	data    []byte
}

// CRC32 implementation [writer.EncodedSegment].
func (s *testSegment) CRC32() uint32 {
	return s.crc32
}

// Samples implementation [writer.EncodedSegment].
func (s *testSegment) Samples() uint32 {
	return s.samples
}

// Size implementation [writer.EncodedSegment].
func (s *testSegment) Size() int64 {
	return s.size
}

// WriteTo implementation [writer.EncodedSegment].
func (s *testSegment) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(s.data)
	return int64(n), err
}
