package writer_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/writer"
)

func TestWriteHeader(t *testing.T) {
	buf := &bytes.Buffer{}
	fileFormatVersion := uint8(21)
	encoderVersion := uint8(42)
	expected := []byte{fileFormatVersion, encoderVersion}

	n, err := writer.WriteHeader(buf, fileFormatVersion, encoderVersion)
	require.NoError(t, err)

	require.Equal(t, len(expected), n)
	require.Equal(t, expected, buf.Bytes())
}
