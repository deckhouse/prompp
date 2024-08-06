// Copyright OpCore

package handler

import (
	"encoding/binary"
	"errors"
	"io"
	"sync"

	"github.com/golang/snappy"
)

const (
	minRead         = 512
	smallBufferSize = 64
	maxInt          = int(^uint(0) >> 1)
	minSliceSize    = 8192
)

var (
	errTooLarge     = errors.New("buffer: too large")
	errNegativeRead = errors.New("buffer: reader returned negative count from Read")
)

// bufferSnappy buffer []byte for decode snappy.
type bufferSnappy struct {
	b []byte
}

// newBufferSnappy init bufferSnappy.
func newBufferSnappy() *bufferSnappy {
	return &bufferSnappy{
		b: make([]byte, 0),
	}
}

// Bytes returns a slice bytes.
func (bs *bufferSnappy) Bytes() []byte {
	return bs.b
}

// Destroy destroy bufferSnappy, return to pool.
func (bs *bufferSnappy) Destroy() {
	releaseBufferSnappy(bs)
}

// decodeFrom read byte from reader and decode from snappy.
func (bs *bufferSnappy) decodeFrom(r io.ReadCloser) error {
	bsreader := acquireBufferSnappy()

	_, err := bsreader.ReadFrom(r)
	_ = r.Close()
	if err != nil {
		releaseBufferSnappy(bsreader)
		return err
	}

	dLen, _, err := bsreader.decodedLen()
	if err != nil {
		releaseBufferSnappy(bsreader)
		return err
	}
	bs.growIfNeed(dLen)

	bs.b, err = snappy.Decode(bs.b, bsreader.b)
	releaseBufferSnappy(bsreader)
	if err != nil {
		return err
	}

	return nil
}

// len is length of bufferSnappy.
func (bs *bufferSnappy) len() int {
	return len(bs.b)
}

// reset bufferSnappy.
func (bs *bufferSnappy) reset() {
	bs.b = bs.b[:0]
}

// decodedLen returns the length of the decoded block and the number of bytes
// that the length header occupied.
func (bs *bufferSnappy) decodedLen() (blockLen, headerLen int, err error) {
	v, n := binary.Uvarint(bs.b)
	//revive:disable-next-line:add-constant not need const
	if n <= 0 || v > 0xffffffff {
		return 0, 0, snappy.ErrCorrupt
	}
	//revive:disable-next-line:add-constant not need const
	const wordSize = 32 << (^uint(0) >> 32 & 1)
	if wordSize == 32 && v > 0x7fffffff {
		return 0, 0, errTooLarge
	}
	return int(v), n, nil
}

// growIfNeed resize slice if need.
func (bs *bufferSnappy) growIfNeed(n int) {
	switch {
	case n < cap(bs.b):
		bs.b = bs.b[:n]
	case n == cap(bs.b):
		bs.b = bs.b[:cap(bs.b)]
	case n > cap(bs.b):
		bs.b = append([]byte(nil), make([]byte, n)...)
	}
}

// ReadFrom implement bytes.Buffer.
func (bs *bufferSnappy) ReadFrom(r io.Reader) (n int64, err error) {
	for {
		i := bs.grow(minRead)
		bs.b = bs.b[:i]
		m, e := r.Read(bs.b[i:cap(bs.b)])
		if m < 0 {
			panic(errNegativeRead)
		}

		bs.b = bs.b[:i+m]
		n += int64(m)
		if e == io.EOF {
			return n, nil // e is EOF, so return nil explicitly
		}
		if e != nil {
			return n, e
		}
	}
}

// grow resize buffer.
func (bs *bufferSnappy) grow(n int) int {
	m := bs.len()
	// Try to grow by means of a reslice.
	if i, ok := bs.tryGrowByReslice(n); ok {
		return i
	}
	if bs.b == nil && n <= smallBufferSize {
		bs.b = make([]byte, n, smallBufferSize)
		return 0
	}
	c := cap(bs.b)
	if c > maxInt-c-n {
		panic(errTooLarge)
	}
	bs.b = growSlice(bs.b, n)

	bs.b = bs.b[:m+n]
	return m
}

func (bs *bufferSnappy) tryGrowByReslice(n int) (int, bool) {
	if l := len(bs.b); n <= cap(bs.b)-l {
		bs.b = bs.b[:l+n]
		return l, true
	}
	return 0, false
}

func growSlice(b []byte, n int) []byte {
	defer func() {
		if recover() != nil {
			panic(errTooLarge)
		}
	}()
	c := len(b) + n // ensure enough space for n elements
	switch {
	case c < minSliceSize:
		c = minSliceSize
	default:
		// The growth rate has historically always been 2x. In the future,
		// we could rely purely on append to determine the growth rate.
		c = int(1.5 * float64(cap(b)))
	}
	b2 := append([]byte(nil), make([]byte, c)...)
	copy(b2, b)
	return b2[:len(b)]
}

// bufferSnappyPool bufferSnappy to pool.
var bufferSnappyPool = &sync.Pool{
	New: func() interface{} {
		return newBufferSnappy()
	},
}

// acquireBufferSnappy get *bufferSnappy from pool.
func acquireBufferSnappy() *bufferSnappy {
	bs := bufferSnappyPool.Get().(*bufferSnappy)
	return bs
}

// releaseBufferSnappy return *bufferSnappy to pool.
func releaseBufferSnappy(bs *bufferSnappy) {
	// if bs.len() > defaultBufferSnappySize {
	// 	bs.reset()
	// 	return
	// }

	bs.reset()
	bufferSnappyPool.Put(bs)
}
