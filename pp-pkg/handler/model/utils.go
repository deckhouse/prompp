package model

// ResizeBuffer resize slice and fill zero value.
func ResizeBuffer(size int, buf *[]byte) {
	if cap(*buf) < size {
		*buf = append(*buf, make([]byte, size)...)
	}

	*buf = (*buf)[:size]
	(*buf)[0] = 0

	for i := 1; i < len(*buf); i *= 2 {
		copy((*buf)[i:], (*buf)[:i])
	}
}
