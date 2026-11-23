package catalog

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/suite"
)

type CodecV1Suite struct {
	suite.Suite
}

func TestCodecV1Suite(t *testing.T) {
	suite.Run(t, new(CodecV1Suite))
}

func (s *CodecV1Suite) TestDecoderV1SetsSnapshotFields() {
	buf := &bytes.Buffer{}
	s.Require().NoError(EncoderV1{}.EncodeTo(buf, newCodecRecord()))
	sr := &SerializedRecord{}

	err := DecoderV1{}.DecodeFrom(buf, sr)

	s.Require().NoError(err)
	s.Equal(fieldsSnapshotV2, sr.fields)
}
