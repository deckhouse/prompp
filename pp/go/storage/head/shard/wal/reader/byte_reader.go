package reader

import "io"

// ByteReader reads from the reader 1 byte at a time.
type ByteReader struct {
	r         io.Reader
	buf       [1]byte
	readBytes int // bytes read via ReadByte
}

// NewByteReader init new [byteReader]
func NewByteReader(r io.Reader) *ByteReader {
	return &ByteReader{
		r:   r,
		buf: [1]byte{},
	}
}

// Read reads from the reader into p.
func (r *ByteReader) Read(p []byte) (int, error) {
	return r.r.Read(p)
}

// ReadByte reads from the reader 1 byte.
func (r *ByteReader) ReadByte() (byte, error) {
	n, err := io.ReadFull(r.r, r.buf[:])
	if err != nil {
		return 0, err
	}

	r.readBytes += n

	return r.buf[0], nil
}
