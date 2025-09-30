package cppbridge

import "runtime"

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

func (e *HeadWalEncoder) Encode(innerSeriesSlice []*InnerSeries) (WALEncoderStats, error) {
	res, err := headWalEncoderAddInnerSeries(e.encoder, innerSeriesSlice)
	runtime.KeepAlive(e)
	return res, err
}

func (e *HeadWalEncoder) Finalize() (*EncodedSegment, error) {
	stats, segment, err := headWalEncoderFinalize(e.encoder)
	runtime.KeepAlive(e)
	return NewEncodedSegment(segment, stats), err
}

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
