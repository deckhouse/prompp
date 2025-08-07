package reader

import "io"

// byteReader reads from the reader 1 byte at a time.
type byteReader struct {
	r   io.Reader
	buf []byte
	n   int
}

// newByteReader init new [byteReader]
func newByteReader(r io.Reader) *byteReader {
	return &byteReader{
		r:   r,
		buf: make([]byte, 1),
	}
}

// ReadByte reads from the reader 1 byte.
func (r *byteReader) ReadByte() (byte, error) {
	n, err := io.ReadFull(r.r, r.buf)
	if err != nil {
		return 0, err
	}

	r.n += n

	return r.buf[0], nil
}
