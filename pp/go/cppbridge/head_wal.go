package cppbridge

import (
	"hash/crc32"
	"io"
	"runtime"
)

//
// HeadEncodedSegment
//

// HeadEncodedSegment the encoded segment from the head wal.
type HeadEncodedSegment struct {
	buf     []byte
	samples uint32
}

// NewHeadEncodedSegment init new [HeadEncodedSegment].
func NewHeadEncodedSegment(b []byte, samples uint32) *HeadEncodedSegment {
	s := &HeadEncodedSegment{
		buf:     b,
		samples: samples,
	}

	runtime.SetFinalizer(s, func(s *HeadEncodedSegment) {
		freeBytes(s.buf)
	})

	return s
}

// Samples returns count of samples in segment.
func (s HeadEncodedSegment) Samples() uint32 {
	return s.samples
}

// Size returns len of bytes.
func (s *HeadEncodedSegment) Size() int64 {
	return int64(len(s.buf))
}

// CRC32 the hash amount according to the data.
func (s *HeadEncodedSegment) CRC32() uint32 {
	return crc32.ChecksumIEEE(s.buf)
}

// WriteTo implements io.WriterTo inerface.
func (s *HeadEncodedSegment) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(s.buf)
	return int64(n), err
}

//
// HeadWalEncoder
//

type HeadWalEncoder struct {
	lss     *LabelSetStorage
	encoder uintptr
}

func NewHeadWalEncoder(shardID uint16, logShards uint8, lss *LabelSetStorage) *HeadWalEncoder {
	e := &HeadWalEncoder{
		lss:     lss,
		encoder: headWalEncoderCtor(shardID, logShards, lss.Pointer()),
	}

	runtime.SetFinalizer(e, func(e *HeadWalEncoder) {
		headWalEncoderDtor(e.encoder)
	})

	return e
}

func (*HeadWalEncoder) Version() uint8 {
	return EncodersVersion()
}

func (e *HeadWalEncoder) Encode(innerSeriesSlice []*InnerSeries) (uint32, error) {
	samples, err := headWalEncoderAddInnerSeries(e.encoder, innerSeriesSlice)
	runtime.KeepAlive(e)
	return samples, err
}

func (e *HeadWalEncoder) Finalize() (*HeadEncodedSegment, error) {
	samples, segment, err := headWalEncoderFinalize(e.encoder)
	runtime.KeepAlive(e)
	return NewHeadEncodedSegment(segment, samples), err
}

//
// HeadWalDecoder
//

type HeadWalDecoder struct {
	lss     *LabelSetStorage
	decoder uintptr
}

func NewHeadWalDecoder(lss *LabelSetStorage, encoderVersion uint8) *HeadWalDecoder {
	d := &HeadWalDecoder{
		lss:     lss,
		decoder: headWalDecoderCtor(lss.Pointer(), encoderVersion),
	}

	runtime.SetFinalizer(d, func(d *HeadWalDecoder) {
		headWalDecoderDtor(d.decoder)
	})

	return d
}

func (d *HeadWalDecoder) Decode(segment []byte, innerSeries *InnerSeries) error {
	err := headWalDecoderDecode(d.decoder, segment, innerSeries)
	runtime.KeepAlive(d)
	return err
}

func (d *HeadWalDecoder) DecodeToDataStorage(segment []byte, headEncoder *HeadEncoder) (int64, int64, error) {
	createTimestamp, encodeTimestamp, err := headWalDecoderDecodeToDataStorage(d.decoder, segment, headEncoder.encoder)
	runtime.KeepAlive(d)
	runtime.KeepAlive(headEncoder)
	return createTimestamp, encodeTimestamp, err
}

func (d *HeadWalDecoder) CreateEncoder() *HeadWalEncoder {
	e := &HeadWalEncoder{
		lss:     d.lss,
		encoder: headWalDecoderCreateEncoder(d.decoder),
	}

	runtime.SetFinalizer(e, func(e *HeadWalEncoder) {
		headWalEncoderDtor(e.encoder)
	})

	return e
}
