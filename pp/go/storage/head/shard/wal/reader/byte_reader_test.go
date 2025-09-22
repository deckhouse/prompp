package reader_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/reader"
)

func TestRead(t *testing.T) {
	bb := &bytes.Buffer{}
	br := reader.NewByteReader(bb)
	data := []byte{1, 42, 3, 0}

	_, err := bb.Write(data)
	require.NoError(t, err)

	for _, expectedV := range data {
		actualV, errRead := br.ReadByte()
		require.NoError(t, errRead)
		require.Equal(t, expectedV, actualV)
	}

	_, err = br.ReadByte()
	require.ErrorIs(t, err, io.EOF)
}

func BenchmarkBR1(b *testing.B) {
	bb := &bytes.Buffer{}
	br := reader.NewByteReader(bb)

	buf := []byte{1, 2, 3}

	for i := 0; i < b.N; i++ {
		_, _ = bb.Write(buf)
		_, _ = br.ReadByte()
		_, _ = br.ReadByte()
		_, _ = br.ReadByte()
	}
}
