package reader_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/reader"
)

func TestWriteHeader(t *testing.T) {
	buf := &bytes.Buffer{}
	expected := []byte{21, 42}

	_, err := buf.Write(expected)
	require.NoError(t, err)

	fileFormatVersion, encoderVersion, _, err := reader.ReadHeader(buf)
	require.NoError(t, err)

	require.Equal(t, expected[0], fileFormatVersion)
	require.Equal(t, expected[1], encoderVersion)
}
