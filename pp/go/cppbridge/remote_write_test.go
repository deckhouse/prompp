package cppbridge

import (
	"runtime"
	"testing"

	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/stretchr/testify/suite"
)

type MessageEncodersSuite struct {
	suite.Suite
}

func TestMessageEncodersSuite(t *testing.T) {
	suite.Run(t, new(MessageEncodersSuite))
}

func (s *MessageEncodersSuite) TestEncode() {
	// Arrange
	lss := NewLssStorage()
	lss.FindOrEmplace(model.NewLabelSetBuilder().Set("__name__", "name1").Set("job", "doing1").Build())

	messages := NewRWMessageList(1, 0)
	encoders := NewMessageEncoders(1, []*LabelSetStorage{lss})
	sampleStorages := NewSegmentSamplesStorage(1)

	walSegmentSamplesStorageAdd(sampleStorages.Get(0), 0, 1000, 1.1)
	walSegmentSamplesStorageAdd(sampleStorages.Get(0), 0, 2000, 1.1)
	walSegmentSamplesStorageAdd(sampleStorages.Get(0), 0, 3000, 1.1)

	sampleStorages.SplitMessages(3)

	// Act
	encoders.Encode(
		0,
		sampleStorages,
		0,
		1,
		messages.Messages,
	)

	// Assert
	s.Equal(int64(3000), messages.Messages[0].MaxTimestamp)
	s.Equal(uint64(3), messages.Messages[0].SampleCount)
	s.Equal([]byte{
		0x4e, 0x3c, 0x0a, 0x4c, 0x0a, 0x11, 0x0a, 0x08, 0x5f, 0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x5f, 0x5f,
		0x12, 0x05, 0x01, 0x08, 0x50, 0x31, 0x0a, 0x0d, 0x0a, 0x03, 0x6a, 0x6f, 0x62, 0x12, 0x06, 0x64,
		0x6f, 0x69, 0x6e, 0x67, 0x31, 0x12, 0x0c, 0x09, 0x9a, 0x99, 0x01, 0x01, 0x10, 0xf1, 0x3f, 0x10,
		0xe8, 0x07, 0x2e, 0x0e, 0x00, 0x3c, 0xd0, 0x0f, 0x12, 0x0c, 0x09, 0x9a, 0x99, 0x99, 0x99, 0x99,
		0x99, 0xf1, 0x3f, 0x10, 0xb8, 0x17,
	}, messages.Messages[0].Buffer)

	runtime.KeepAlive(lss)
	runtime.KeepAlive(messages)
	runtime.KeepAlive(encoders)
	runtime.KeepAlive(sampleStorages)
}
