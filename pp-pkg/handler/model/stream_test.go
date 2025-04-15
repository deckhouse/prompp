package model_test

import (
	"bytes"
	"hash/crc32"
	"testing"
	"time"

	"github.com/go-faker/faker/v4"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp-pkg/handler/model"
	"github.com/prometheus/prometheus/util/pool"
)

type StreamSuite struct {
	suite.Suite
}

func TestStreamSuite(t *testing.T) {
	suite.Run(t, new(StreamSuite))
}

func (s *StreamSuite) TestSegmentEncodeDecode() {
	body := faker.Paragraph()
	expectedSegment := model.Segment{
		Timestamp: time.Now().UnixMilli(),
		ID:        42,
		Size:      uint32(len(body)),
		CRC:       crc32.ChecksumIEEE([]byte(body)),
		Body:      []byte(body),
	}

	bb := &bytes.Buffer{}

	enc := model.NewSegmentEncoder(bb)
	err := enc.Encode(expectedSegment)
	s.Require().NoError(err)

	buffers := pool.New(8, 1e6, 2, func(sz int) interface{} { return make([]byte, 0, sz) })
	actualSegment := &model.Segment{}
	defer actualSegment.Destroy()

	dec := model.NewStreamSegmentDecoder(bb, buffers)
	err = dec.Decode(actualSegment)
	s.Require().NoError(err)

	s.Require().True(actualSegment.IsValid())
	s.Equal(expectedSegment.Timestamp, actualSegment.Timestamp)
	s.Equal(expectedSegment.ID, actualSegment.ID)
	s.Equal(expectedSegment.Size, actualSegment.Size)
	s.Equal(expectedSegment.CRC, actualSegment.CRC)
	s.Equal(body, string(actualSegment.Body))
}

func (s *StreamSuite) TestSegmentEncodeDecodeEmpty() {
	var body []byte
	expectedSegment := model.Segment{
		Timestamp: time.Now().UnixMilli(),
		ID:        42,
		Size:      uint32(len(body)),
		CRC:       crc32.ChecksumIEEE(body),
		Body:      body,
	}

	bb := &bytes.Buffer{}

	enc := model.NewSegmentEncoder(bb)
	err := enc.Encode(expectedSegment)
	s.Require().NoError(err)

	buffers := pool.New(8, 1e6, 2, func(sz int) interface{} { return make([]byte, 0, sz) })
	actualSegment := &model.Segment{}
	defer actualSegment.Destroy()

	dec := model.NewStreamSegmentDecoder(bb, buffers)
	err = dec.Decode(actualSegment)
	s.Require().NoError(err)

	s.Require().True(actualSegment.IsValid())
	s.Equal(expectedSegment.Timestamp, actualSegment.Timestamp)
	s.Equal(expectedSegment.ID, actualSegment.ID)
	s.Equal(expectedSegment.Size, actualSegment.Size)
	s.Equal(expectedSegment.CRC, actualSegment.CRC)
	s.Equal(body, actualSegment.Body)
}

func (s *StreamSuite) TestSegmentEncodeDecodeError() {
	body := faker.Paragraph()
	expectedSegment := model.Segment{
		Timestamp: time.Now().UnixMilli(),
		ID:        42,
		Size:      uint32(len(body)),
		CRC:       crc32.ChecksumIEEE([]byte(body)) - 1,
		Body:      []byte(body),
	}

	bb := &bytes.Buffer{}

	enc := model.NewSegmentEncoder(bb)
	err := enc.Encode(expectedSegment)
	s.Require().NoError(err)

	buffers := pool.New(8, 1e6, 2, func(sz int) interface{} { return make([]byte, 0, sz) })
	actualSegment := &model.Segment{}
	defer actualSegment.Destroy()

	dec := model.NewStreamSegmentDecoder(bb, buffers)
	err = dec.Decode(actualSegment)
	s.Require().ErrorIs(err, model.ErrCorruptedSegment)
}

func (s *StreamSuite) TestSegmentProcessingStatusEncodeDecode() {
	msg := faker.Paragraph()
	expectedSegmentStatus := model.StreamSegmentProcessingStatus{
		SegmentID: 42,
		Code:      200,
		Message:   msg,
		Timestamp: time.Now().UnixMilli(),
	}

	bb := &bytes.Buffer{}

	err := expectedSegmentStatus.Write(bb)
	s.Require().NoError(err)

	actualSegmentStatus := &model.StreamSegmentProcessingStatus{}
	err = actualSegmentStatus.DecodeFrom(bb)
	s.Require().NoError(err)

	s.Equal(expectedSegmentStatus.SegmentID, actualSegmentStatus.SegmentID)
	s.Equal(expectedSegmentStatus.Code, actualSegmentStatus.Code)
	s.Equal(expectedSegmentStatus.Message, actualSegmentStatus.Message)
	s.Equal(expectedSegmentStatus.Timestamp, actualSegmentStatus.Timestamp)
}
