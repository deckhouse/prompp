package reader

import (
	"bytes"
	"testing"
)

func TestXxx(t *testing.T) {
	bb := &bytes.Buffer{}
	br := newByteReader(bb)

	bb.Write([]byte{1, 2, 3, 0})

	t.Log(br.ReadByte())
	t.Log(br.ReadByte())
	t.Log(br.ReadByte())
	t.Log(br.ReadByte())
}

func BenchmarkBR1(b *testing.B) {
	bb := &bytes.Buffer{}
	br := newByteReader(bb)

	buf := []byte{1, 2, 3}

	for i := 0; i < b.N; i++ {
		bb.Write(buf)
		br.ReadByte()
		br.ReadByte()
		br.ReadByte()
	}
}
