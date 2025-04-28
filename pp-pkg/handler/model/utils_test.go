package model_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp-pkg/handler/model"
)

type UtilsSuite struct {
	suite.Suite
}

func TestUtilsSuite(t *testing.T) {
	suite.Run(t, new(UtilsSuite))
}

func (s *UtilsSuite) TestResizeBuffer() {
	buf := make([]byte, 10)
	for i := range buf {
		buf[i] = 42
	}

	model.ResizeBuffer(10, &buf)

	s.Len(buf, 10)

	for i := range buf {
		s.Require().Equal(byte(0), buf[i])
	}
}

func (s *UtilsSuite) TestResizeBufferLess() {
	buf := make([]byte, 10)
	for i := range buf {
		buf[i] = 42
	}

	model.ResizeBuffer(5, &buf)

	s.Len(buf, 5)

	for i := range buf {
		s.Require().Equal(byte(0), buf[i])
	}
}

func (s *UtilsSuite) TestResizeBufferMore() {
	buf := make([]byte, 10)
	for i := range buf {
		buf[i] = 42
	}

	model.ResizeBuffer(15, &buf)

	s.Len(buf, 15)

	for i := range buf {
		s.Require().Equal(byte(0), buf[i])
	}
}
