package cppbridge

import (
	"errors"
	"fmt"
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

// HeadWalEncoder the encoder for the head wal.
type HeadWalEncoder struct {
	lss     *LabelSetStorage
	encoder uintptr
}

// NewHeadWalEncoder initializes a new [HeadWalEncoder].
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

// Version returns current encoder version.
func (*HeadWalEncoder) Version() uint8 {
	return EncodersVersion()
}

// Encode encodes inner series into a segment.
func (e *HeadWalEncoder) Encode(innerSeriesSlice []*InnerSeries) (uint32, error) {
	samples, err := headWalEncoderAddInnerSeries(e.encoder, innerSeriesSlice)
	runtime.KeepAlive(e)
	return samples, err
}

// Finalize finalizes the encoder and returns the encoded segment.
func (e *HeadWalEncoder) Finalize() (*HeadEncodedSegment, error) {
	samples, segment, err := headWalEncoderFinalize(e.encoder)
	runtime.KeepAlive(e)
	return NewHeadEncodedSegment(segment, samples), err
}

//
// HeadWalDecoder
//

// HeadWalDecoder the decoder for the head wal.
type HeadWalDecoder struct {
	lss     *LabelSetStorage
	decoder uintptr
}

// NewHeadWalDecoder initializes a new [HeadWalDecoder].
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

// Decode decodes a segment into an inner series.
func (d *HeadWalDecoder) Decode(segment []byte, innerSeries *InnerSeries) error {
	err := headWalDecoderDecode(d.decoder, segment, innerSeries)
	runtime.KeepAlive(d)
	return err
}

// DecodeToDataStorage decodes a segment into a data storage.
//
//revive:disable-next-line:confusing-results // returns createTimestamp, encodeTimestamp, error.
//nolint:gocritic // unnamedResult // returns createTimestamp, encodeTimestamp, error.
func (d *HeadWalDecoder) DecodeToDataStorage(segment []byte, headEncoder *HeadEncoder) (int64, int64, error) {
	createTimestamp, encodeTimestamp, err := headWalDecoderDecodeToDataStorage(d.decoder, segment, headEncoder.encoder)
	runtime.KeepAlive(d)
	runtime.KeepAlive(headEncoder)
	return createTimestamp, encodeTimestamp, err
}

// ErrInvalidEncoderVersion migration error.
var ErrInvalidEncoderVersion = errors.New("invalid encoder version")

// CreateEncoder creates a new [HeadWalEncoder] from the decoder.
func (d *HeadWalDecoder) CreateEncoder() (*HeadWalEncoder, error) {
	encoder, err := headWalDecoderCreateEncoder(d.decoder)
	// the only error for now is: invalid encoder version
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidEncoderVersion, err)
	}

	e := &HeadWalEncoder{
		lss:     d.lss,
		encoder: encoder,
	}

	runtime.SetFinalizer(e, func(e *HeadWalEncoder) {
		headWalEncoderDtor(e.encoder)
	})

	return e, nil
}
