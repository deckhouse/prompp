package model_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/go-faker/faker/v4"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp-pkg/handler/model"
)

type StreamSuite struct {
	suite.Suite
}

func TestStreamSuite(t *testing.T) {
	suite.Run(t, new(StreamSuite))
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

	err := expectedSegmentStatus.EncodeTo(bb)
	s.Require().NoError(err)

	actualSegmentStatus := &model.StreamSegmentProcessingStatus{}
	err = actualSegmentStatus.DecodeFrom(bb)
	s.Require().NoError(err)

	s.Equal(expectedSegmentStatus.SegmentID, actualSegmentStatus.SegmentID)
	s.Equal(expectedSegmentStatus.Code, actualSegmentStatus.Code)
	s.Equal(expectedSegmentStatus.Message, actualSegmentStatus.Message)
	s.Equal(expectedSegmentStatus.Timestamp, actualSegmentStatus.Timestamp)
}
