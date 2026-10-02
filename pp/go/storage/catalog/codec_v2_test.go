package catalog

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/suite"
)

type CodecV2Suite struct {
	suite.Suite
}

func TestCodecV2Suite(t *testing.T) {
	suite.Run(t, new(CodecV2Suite))
}

func (s *CodecV2Suite) TestDecoderV2SetsSnapshotFields() {
	buf := &bytes.Buffer{}
	s.Require().NoError(NewEncoderV2().EncodeTo(buf, newCodecRecord()))
	sr := &SerializedRecord{}

	err := DecoderV2{}.DecodeFrom(buf, sr)

	s.Require().NoError(err)
	s.Equal(fieldsSnapshotV2, sr.fields)
}
