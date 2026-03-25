package reader

import "io"

// ByteReader reads from the reader 1 byte at a time.
type ByteReader struct {
	r   io.Reader
	buf []byte
	n   int
}

// NewByteReader init new [byteReader]
func NewByteReader(r io.Reader) *ByteReader {
	return &ByteReader{
		r:   r,
		buf: make([]byte, 1),
	}
}

// ReadByte reads from the reader 1 byte.
func (r *ByteReader) ReadByte() (byte, error) {
	n, err := io.ReadFull(r.r, r.buf)
	if err != nil {
		return 0, err
	}

	r.n += n

	return r.buf[0], nil
}
