package model_test

import (
	"testing"

	"github.com/go-faker/faker/v4"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp-pkg/handler/model"
)

type RemoteWriteSuite struct {
	suite.Suite
}

func TestRemoteWriteSuite(t *testing.T) {
	suite.Run(t, new(RemoteWriteSuite))
}

func (s *RemoteWriteSuite) TestRemoteWriteProcessingStatus() {
	status := model.RemoteWriteProcessingStatus{
		Code:    200,
		Message: "ok",
	}

	rw := &responseWriter{}

	err := status.Write(rw)
	s.Require().NoError(err)

	s.Equal(status.Code, rw.statusCode)
	s.Equal(status.Message, rw.buf.String())
}

func (s *RemoteWriteSuite) TestRemoteWriteBuffer() {
	buf := []byte(faker.Paragraph())
	rwBuffer := model.NewRemoteWriteBuffer(
		&buf,
		func() {
			for i := range buf {
				buf[i] = 0
			}
		},
	)

	s.Equal(buf, rwBuffer.Bytes())

	rwBuffer.Destroy()

	rwb := rwBuffer.Bytes()
	for i := range rwb {
		s.Equal(byte(0), rwb[i])
	}
}
